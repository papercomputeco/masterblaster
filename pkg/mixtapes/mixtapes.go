// Package mixtapes manages StereOS mixtape images -- the bootable VM images
// that contain pre-configured agentic workflows. Mixtapes are distributed
// as OCI artifacts and stored locally in ~/.mb/mixtapes/.
package mixtapes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

// DefaultRegistry is the default OCI registry for mixtapes.
const DefaultRegistry = "ghcr.io/papercomputeco/mixtapes"

// MixtapeInfo describes a locally available mixtape.
type MixtapeInfo struct {
	Name string
	Path string
	Size int64
}

// List returns all locally available mixtapes in ~/.mb/mixtapes/.
func List(baseDir string) ([]MixtapeInfo, error) {
	mixtapeDir := filepath.Join(baseDir, "mixtapes")
	entries, err := os.ReadDir(mixtapeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading mixtapes directory: %w", err)
	}

	var mixtapes []MixtapeInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		info := MixtapeInfo{
			Name: entry.Name(),
			Path: filepath.Join(mixtapeDir, entry.Name()),
		}

		// Calculate total size of images in the mixtape directory
		imgEntries, err := os.ReadDir(info.Path)
		if err != nil {
			continue
		}
		for _, imgEntry := range imgEntries {
			if imgEntry.IsDir() {
				continue
			}
			fi, err := imgEntry.Info()
			if err != nil {
				continue
			}
			info.Size += fi.Size()
		}

		mixtapes = append(mixtapes, info)
	}

	return mixtapes, nil
}

// Pull downloads a mixtape from the OCI registry to ~/.mb/mixtapes/<name>/.
// For the initial implementation, this is a placeholder that documents
// the intended OCI workflow using oras.land/oras-go/v2.
func Pull(baseDir, name string) error {
	mixtapeDir := filepath.Join(baseDir, "mixtapes", name)
	if err := os.MkdirAll(mixtapeDir, 0755); err != nil {
		return fmt.Errorf("creating mixtape directory: %w", err)
	}

	// Resolve the OCI reference
	ref := name
	if !strings.Contains(ref, "/") {
		ref = DefaultRegistry + "/" + name
	}

	// TODO: Implement OCI pull using oras.land/oras-go/v2
	//
	// The implementation would:
	// 1. Create an OCI store at mixtapeDir
	// 2. Resolve the reference to a descriptor
	// 3. Copy the artifact layers to the local store
	// 4. Extract the disk image from the OCI layer
	//
	// The OCI manifest for a mixtape looks like:
	// {
	//   "mediaType": "application/vnd.oci.image.manifest.v1+json",
	//   "layers": [
	//     {
	//       "mediaType": "application/vnd.papercompute.stereos.disk.v1+raw",
	//       "digest": "sha256:abc123...",
	//       "annotations": {
	//         "org.papercompute.stereos.version": "0.0.1",
	//         "org.papercompute.mixtape.name": "base"
	//       }
	//     }
	//   ]
	// }

	return fmt.Errorf("pulling mixtape %q from %s: OCI pull not yet implemented\n\n"+
		"For now, manually place a StereOS image at:\n"+
		"  %s/nixos.img   (raw)\n"+
		"  %s/nixos.qcow2 (qcow2)",
		name, ref, mixtapeDir, mixtapeDir)
}

// PrintList writes a formatted table of mixtapes to stdout.
func PrintList(mixtapes []MixtapeInfo) {
	if len(mixtapes) == 0 {
		fmt.Println("No mixtapes found. Pull one with: mb mixtapes pull <name>")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSIZE")
	for _, m := range mixtapes {
		fmt.Fprintf(w, "%s\t%s\n", m.Name, formatSize(m.Size))
	}
	w.Flush()
}

func formatSize(bytes int64) string {
	const (
		_  = iota
		kb = 1 << (10 * iota)
		mb
		gb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
