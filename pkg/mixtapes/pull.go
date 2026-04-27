// Package mixtapes -- OCI pull implementation using google/go-containerregistry.
//
// StereOS mixtapes are distributed as OCI artifacts behind an OCI index
// manifest. The index contains two format-specific manifests (raw and qcow2),
// each with 6 layers:
//
//   - Layer 0: zstd-compressed disk image (stereos.img.zst or stereos.qcow2.zst)
//   - Layer 1: bzImage (kernel)
//   - Layer 2: initrd
//   - Layer 3: cmdline
//   - Layer 4: init
//   - Layer 5: mixtape.toml
//
// Disk images are zstd-compressed in the registry and decompressed on pull.
// All layers are written flat into ~/.config/mb/mixtapes/<name>/<tag>/.
package mixtapes

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/klauspost/compress/zstd"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/papercomputeco/masterblaster/pkg/ui"
)

// DefaultDownloadRegistry is the Paper Compute OCI registry for StereOS mixtapes.
const DefaultDownloadRegistry = "download.stereos.ai"

// defaultRepoPrefix is the repository path prefix in the registry.
// Mixtape repos live at download.stereos.ai/mixtapes/<name>.
const defaultRepoPrefix = "mixtapes"

// --- Media types for StereOS layers ---
const (
	mediaTypeDiskRawZstd   = "application/vnd.papercompute.stereos.disk.v1+raw+zstd"
	mediaTypeDiskQcow2Zstd = "application/vnd.papercompute.stereos.disk.v1+qcow2+zstd"
	mediaTypeDiskRaw       = "application/vnd.papercompute.stereos.disk.v1+raw"
	mediaTypeDiskQcow2     = "application/vnd.papercompute.stereos.disk.v1+qcow2"
	mediaTypeKernel        = "application/vnd.papercompute.stereos.kernel.v1"
	mediaTypeInitrd        = "application/vnd.papercompute.stereos.initrd.v1"
	mediaTypeCmdline       = "application/vnd.papercompute.stereos.cmdline.v1"
	mediaTypeInit          = "application/vnd.papercompute.stereos.init.v1"
	mediaTypeMixtapeToml   = "application/vnd.papercompute.stereos.mixtape-manifest.v1+toml"
)

// ParseReference resolves a user-provided name[:tag] string into a full OCI
// reference. Short names (no "/" separator) are prefixed with the default
// registry and /mixtapes/ repo path. If no tag is supplied, "latest" is
// assumed by the name package.
//
// Examples:
//
//	"opencode-mixtape"                -> "download.stereos.ai/mixtapes/opencode-mixtape:latest"
//	"opencode-mixtape:0.1.0"          -> "download.stereos.ai/mixtapes/opencode-mixtape:0.1.0"
//	"myregistry.io/my/repo:v1"        -> "myregistry.io/my/repo:v1"
func ParseReference(rawRef string) (name.Reference, error) {
	ref := rawRef
	if !strings.Contains(ref, "/") {
		ref = DefaultDownloadRegistry + "/" + defaultRepoPrefix + "/" + ref
	}
	return name.ParseReference(ref)
}

// PullOCI downloads a mixtape OCI artifact from a registry and extracts
// each layer to baseDir/mixtapes/<mixtapeName>/<tag>/. The registry serves
// an OCI index manifest; we select the raw-format manifest (preferred for
// Apple Virtualization.framework) and fall back to qcow2.
//
// Zstd-compressed disk layers are decompressed during extraction. All
// files are written flat into the tag directory.
func PullOCI(baseDir, mixtapeName, tag string, ref name.Reference) error {
	// Fetch the remote descriptor first, before creating any directories.
	// This avoids leaving empty directories on the filesystem if the
	// registry returns an error (e.g. manifest unknown).
	desc, err := remote.Get(ref)
	if err != nil {
		return fmt.Errorf("fetching manifest for %s: %w", ref.String(), err)
	}

	img, err := resolveImage(desc)
	if err != nil {
		return fmt.Errorf("resolving image for %s: %w", ref.String(), err)
	}

	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("reading layers: %w", err)
	}

	if len(layers) == 0 {
		return fmt.Errorf("manifest for %s contains no layers", ref.String())
	}

	// Only create the directory once we know the manifest is valid.
	mixtapeDir := filepath.Join(baseDir, "mixtapes", mixtapeName, tag)
	if err := os.MkdirAll(mixtapeDir, 0755); err != nil {
		return fmt.Errorf("creating mixtape directory: %w", err)
	}

	for i, layer := range layers {
		if err := extractLayer(mixtapeDir, layer, i); err != nil {
			// Clean up on failure -- don't leave partial downloads.
			_ = os.RemoveAll(mixtapeDir)
			return err
		}
	}

	return nil
}

// resolveImage selects the appropriate platform image from an OCI index,
// or returns the image directly if the descriptor is already a single
// manifest.
//
// For index manifests, we prefer the raw disk format manifest (listed
// first by build convention). We identify it by inspecting each child
// manifest's layers for the raw disk media type.
func resolveImage(desc *remote.Descriptor) (v1.Image, error) {
	switch desc.MediaType {
	case types.OCIImageIndex, types.DockerManifestList:
		return resolveFromIndex(desc)
	default:
		// Single manifest -- use directly.
		return desc.Image()
	}
}

// resolveFromIndex parses an OCI index and selects the best manifest for the
// running host, preferring raw disk format.
//
// Selection priority (see selectIndexManifest for the pure policy):
//  1. Platform matches host GOOS/GOARCH AND has a raw disk layer
//  2. Platform matches host GOOS/GOARCH (any format)
//  3. Any manifest with a raw disk layer (index didn't set Platform)
//  4. First manifest (last-resort fallback)
func resolveFromIndex(desc *remote.Descriptor) (v1.Image, error) {
	idx, err := desc.ImageIndex()
	if err != nil {
		return nil, fmt.Errorf("parsing image index: %w", err)
	}

	indexManifest, err := idx.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("reading index manifest: %w", err)
	}

	hasRaw := func(h v1.Hash) (bool, error) {
		img, err := idx.Image(h)
		if err != nil {
			return false, err
		}
		return hasRawDiskLayer(img), nil
	}

	digest, err := selectIndexManifest(indexManifest.Manifests, runtime.GOOS, runtime.GOARCH, hasRaw)
	if err != nil {
		return nil, err
	}
	return idx.Image(digest)
}

// hasRawDiskLayerFunc reports whether the manifest addressed by a given digest
// contains a raw disk layer. Parameterised so selectIndexManifest stays
// testable without a real OCI client.
type hasRawDiskLayerFunc func(v1.Hash) (bool, error)

// selectIndexManifest picks the best manifest digest from an OCI index's
// manifest descriptors for the given host GOOS/GOARCH. See resolveFromIndex
// for priority order.
func selectIndexManifest(
	manifests []v1.Descriptor,
	goos, goarch string,
	hasRaw hasRawDiskLayerFunc,
) (v1.Hash, error) {
	if len(manifests) == 0 {
		return v1.Hash{}, fmt.Errorf("index manifest contains no entries")
	}

	platformMatch := func(p *v1.Platform) bool {
		return p != nil && p.OS == goos && p.Architecture == goarch
	}

	var (
		platformRaw, platformAny, anyRaw v1.Hash
		hasT1, hasT2, hasT3              bool
	)

	for _, m := range manifests {
		// Cache per-manifest raw-probe so we only load each manifest once.
		var (
			rawChecked, rawOK bool
		)
		checkRaw := func() bool {
			if rawChecked {
				return rawOK
			}
			rawChecked = true
			ok, err := hasRaw(m.Digest)
			rawOK = err == nil && ok
			return rawOK
		}

		if platformMatch(m.Platform) {
			if !hasT2 {
				platformAny = m.Digest
				hasT2 = true
			}
			if !hasT1 && checkRaw() {
				platformRaw = m.Digest
				hasT1 = true
			}
		}
		if !hasT3 && checkRaw() {
			anyRaw = m.Digest
			hasT3 = true
		}

		// Early exit: we have the top-priority match.
		if hasT1 {
			break
		}
	}

	switch {
	case hasT1:
		ui.Info("Selected raw disk manifest for %s/%s from index", goos, goarch)
		return platformRaw, nil
	case hasT2:
		ui.Warn("No raw format for %s/%s in index; using platform-matched manifest", goos, goarch)
		return platformAny, nil
	case hasT3:
		ui.Warn("No %s/%s manifest in index; using any raw-format manifest (may not run on this host)", goos, goarch)
		return anyRaw, nil
	default:
		ui.Warn("No raw format in index, using first manifest")
		return manifests[0].Digest, nil
	}
}

// hasRawDiskLayer checks whether an image contains a raw disk layer.
func hasRawDiskLayer(img v1.Image) bool {
	layers, err := img.Layers()
	if err != nil {
		return false
	}
	for _, l := range layers {
		mt, err := l.MediaType()
		if err != nil {
			continue
		}
		if mt == types.MediaType(mediaTypeDiskRawZstd) || mt == types.MediaType(mediaTypeDiskRaw) {
			return true
		}
	}
	return false
}

// extractLayer writes a single OCI layer to the mixtape directory.
// All files are written flat. Zstd-compressed disk layers are
// decompressed on the fly.
func extractLayer(mixtapeDir string, layer v1.Layer, index int) error {
	digest, err := layer.Digest()
	if err != nil {
		return fmt.Errorf("reading digest for layer %d: %w", index, err)
	}

	mediaType, _ := layer.MediaType()
	mt := string(mediaType)

	// Determine the output filename and whether zstd decompression is needed.
	filename, needsZstdDecompress := resolveLayerOutput(mt, layer, digest.Hex)

	// Sanitize: strip any path separators to prevent directory traversal.
	filename = filepath.Base(filename)

	size, _ := layer.Size()
	sizeStr := ""
	if size > 0 {
		sizeStr = fmt.Sprintf(" (%s)", formatSize(size))
	}

	if needsZstdDecompress {
		decompressedName := decompressedFilename(filename)
		ui.Info("Downloading %s%s -> decompressing to %s", filename, sizeStr, decompressedName)
	} else {
		ui.Info("Downloading %s%s", filename, sizeStr)
	}

	// Open the layer stream. For OCI artifacts with custom media types,
	// layers are not Docker-style gzip compressed -- they're stored as-is
	// (possibly zstd-compressed at the application level). Use Compressed()
	// to get the raw blob bytes.
	rc, err := layer.Compressed()
	if err != nil {
		return fmt.Errorf("opening layer %s: %w", digest.String(), err)
	}
	defer func() { _ = rc.Close() }()

	outPath := filepath.Join(mixtapeDir, filename)

	if needsZstdDecompress {
		// Stream-decompress the zstd layer directly to disk.
		decompressedName := decompressedFilename(filename)
		decompressedPath := filepath.Join(mixtapeDir, decompressedName)
		if err := writeZstdDecompressed(rc, decompressedPath); err != nil {
			return fmt.Errorf("decompressing %s: %w", filename, err)
		}
	} else {
		// Write the layer directly.
		if err := writeFile(rc, outPath); err != nil {
			return err
		}
	}

	return nil
}

// resolveLayerOutput determines the filename and whether zstd decompression
// is needed for a layer based on its media type and annotations.
func resolveLayerOutput(mediaType string, layer v1.Layer, digestHex string) (filename string, needsZstdDecompress bool) {
	// Try to get filename from annotation first.
	desc, err := layerDescriptor(layer)
	if err == nil {
		if title, ok := desc.Annotations["org.opencontainers.image.title"]; ok && title != "" {
			filename = title
		}
	}

	switch mediaType {
	case mediaTypeDiskRawZstd:
		if filename == "" {
			filename = "stereos.img.zst"
		}
		return filename, true

	case mediaTypeDiskQcow2Zstd:
		if filename == "" {
			filename = "stereos.qcow2.zst"
		}
		return filename, true

	case mediaTypeDiskRaw:
		if filename == "" {
			filename = "stereos.img"
		}
		return filename, false

	case mediaTypeDiskQcow2:
		if filename == "" {
			filename = "stereos.qcow2"
		}
		return filename, false

	case mediaTypeKernel:
		if filename == "" {
			filename = "bzImage"
		}
		return filename, false

	case mediaTypeInitrd:
		if filename == "" {
			filename = "initrd"
		}
		return filename, false

	case mediaTypeCmdline:
		if filename == "" {
			filename = "cmdline"
		}
		return filename, false

	case mediaTypeInit:
		if filename == "" {
			filename = "init"
		}
		return filename, false

	case mediaTypeMixtapeToml:
		if filename == "" {
			filename = "mixtape.toml"
		}
		return filename, false

	default:
		if filename == "" {
			filename = digestHex
		}
		return filename, false
	}
}

// decompressedFilename strips the .zst suffix from a filename.
// "stereos.img.zst" -> "stereos.img", "stereos.qcow2.zst" -> "stereos.qcow2".
func decompressedFilename(name string) string {
	return strings.TrimSuffix(name, ".zst")
}

// writeZstdDecompressed reads a zstd-compressed stream and writes the
// decompressed output to dstPath.
func writeZstdDecompressed(r io.Reader, dstPath string) error {
	decoder, err := zstd.NewReader(r)
	if err != nil {
		return fmt.Errorf("creating zstd decoder: %w", err)
	}
	defer decoder.Close()

	f, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dstPath, err)
	}

	if _, err := io.Copy(f, decoder); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing %s: %w", dstPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", dstPath, err)
	}
	return nil
}

// writeFile writes a reader to a file at outPath.
func writeFile(r io.Reader, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", outPath, err)
	}

	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing %s: %w", outPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", outPath, err)
	}
	return nil
}

// layerDescriptor extracts the v1.Descriptor from a layer so we can
// read annotations. go-containerregistry layers don't expose annotations
// directly, so we use the Descriptor() interface when available.
func layerDescriptor(layer v1.Layer) (*v1.Descriptor, error) {
	// The remote.Layer type carries the original descriptor with annotations.
	// Try to get it via the Descriptor interface.
	type describer interface {
		Descriptor() (*v1.Descriptor, error)
	}
	if d, ok := layer.(describer); ok {
		return d.Descriptor()
	}

	// Fallback: build a minimal descriptor without annotations.
	digest, err := layer.Digest()
	if err != nil {
		return nil, err
	}
	size, err := layer.Size()
	if err != nil {
		return nil, err
	}
	mt, err := layer.MediaType()
	if err != nil {
		return nil, err
	}
	return &v1.Descriptor{
		Digest:    digest,
		Size:      size,
		MediaType: mt,
	}, nil
}
