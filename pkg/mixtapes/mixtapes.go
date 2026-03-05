// Package mixtapes manages StereOS mixtape images -- the bootable VM images
// that contain pre-configured agentic workflows. Mixtapes are distributed
// as OCI artifacts and stored locally in ~/.mb/mixtapes/.
package mixtapes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/papercomputeco/masterblaster/pkg/ui"
)

// DefaultRegistry is the default OCI registry for mixtapes.
// Deprecated: Use DefaultDownloadRegistry in pull.go instead.
const DefaultRegistry = "ghcr.io/papercomputeco/mixtapes"

// MixtapeInfo describes a locally available mixtape.
type MixtapeInfo struct {
	Name string // Short name (e.g. "opencode-mixtape")
	Tag  string // Tag (e.g. "latest", "0.1.0")
	Path string // Full path to the tag directory
	Size int64  // Total size of all files
}

// DisplayName returns "name:tag" for display purposes.
func (m MixtapeInfo) DisplayName() string {
	return m.Name + ":" + m.Tag
}

// List returns all locally available mixtapes in ~/.config/mb/mixtapes/.
// The on-disk layout is mixtapes/{name}/{tag}/.
func List(baseDir string) ([]MixtapeInfo, error) {
	mixtapesRoot := filepath.Join(baseDir, "mixtapes")
	nameEntries, err := os.ReadDir(mixtapesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading mixtapes directory: %w", err)
	}

	var mixtapes []MixtapeInfo
	for _, nameEntry := range nameEntries {
		if !nameEntry.IsDir() {
			continue
		}
		nameDir := filepath.Join(mixtapesRoot, nameEntry.Name())

		tagEntries, err := os.ReadDir(nameDir)
		if err != nil {
			continue
		}

		for _, tagEntry := range tagEntries {
			if !tagEntry.IsDir() {
				continue
			}

			info := MixtapeInfo{
				Name: nameEntry.Name(),
				Tag:  tagEntry.Name(),
				Path: filepath.Join(nameDir, tagEntry.Name()),
			}

			// Calculate total size of all files in the tag directory.
			_ = filepath.WalkDir(info.Path, func(_ string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				fi, err := d.Info()
				if err != nil {
					return nil
				}
				info.Size += fi.Size()
				return nil
			})

			mixtapes = append(mixtapes, info)
		}
	}

	return mixtapes, nil
}

// Pull downloads a mixtape from the OCI registry to
// ~/.config/mb/mixtapes/<name>/<tag>/. It resolves short names (e.g.
// "opencode-mixtape", "opencode-mixtape:0.1.0") against the default
// Paper Compute registry. Full OCI references are also accepted.
func Pull(baseDir, rawRef string) error {
	ref, err := ParseReference(rawRef)
	if err != nil {
		return fmt.Errorf("parsing reference %q: %w", rawRef, err)
	}

	// Derive the local mixtape name and tag from the reference.
	mixtapeName := mixtapeNameFromRef(rawRef, ref)
	tag := tagFromRef(ref)

	return PullOCI(baseDir, mixtapeName, tag, ref)
}

// mixtapeNameFromRef derives a short local directory name from an OCI
// reference. For short names ("opencode-mixtape", "opencode-mixtape:0.1.0")
// it strips the tag. For full refs it uses the last path component of the
// repository.
func mixtapeNameFromRef(rawRef string, ref name.Reference) string {
	// If the user gave a short name, strip the tag portion.
	if !strings.Contains(rawRef, "/") {
		n := rawRef
		if idx := strings.Index(n, ":"); idx != -1 {
			n = n[:idx]
		}
		return n
	}
	// Full reference: use the last path component of the repository.
	repo := ref.Context().RepositoryStr()
	parts := strings.Split(repo, "/")
	return parts[len(parts)-1]
}

// tagFromRef extracts the tag from a parsed OCI reference.
// Defaults to "latest" for untagged references.
func tagFromRef(ref name.Reference) string {
	if t, ok := ref.(name.Tag); ok {
		return t.TagStr()
	}
	// Digest references don't have a tag; use "latest".
	return "latest"
}

// PrintList writes a styled table of mixtapes to stdout.
func PrintList(mixtapes []MixtapeInfo) {
	if len(mixtapes) == 0 {
		fmt.Println("No mixtapes found. Pull one with: mb pull <name>")
		return
	}

	tbl := &ui.Table{
		Headers:  []string{"NAME", "TAG", "SIZE"},
		StateCol: -1,
	}
	for _, m := range mixtapes {
		tbl.Rows = append(tbl.Rows, []string{m.Name, m.Tag, formatSize(m.Size)})
	}
	tbl.Render(os.Stdout)
}

// Remove deletes a locally downloaded mixtape. If tag is empty, the entire
// mixtape directory (all tags) is removed. If tag is specified, only that
// tag directory is removed -- and if it was the last tag, the parent name
// directory is cleaned up too.
func Remove(baseDir, mixtapeName, tag string) error {
	mixtapesRoot := filepath.Join(baseDir, "mixtapes")
	nameDir := filepath.Join(mixtapesRoot, mixtapeName)

	// Verify the mixtape exists.
	if _, err := os.Stat(nameDir); os.IsNotExist(err) {
		return fmt.Errorf("mixtape %q not found locally", mixtapeName)
	}

	if tag == "" {
		// Remove the entire mixtape (all tags).
		if err := os.RemoveAll(nameDir); err != nil {
			return fmt.Errorf("removing mixtape %q: %w", mixtapeName, err)
		}
		return nil
	}

	// Remove a specific tag.
	tagDir := filepath.Join(nameDir, tag)
	if _, err := os.Stat(tagDir); os.IsNotExist(err) {
		return fmt.Errorf("mixtape %q tag %q not found locally", mixtapeName, tag)
	}

	if err := os.RemoveAll(tagDir); err != nil {
		return fmt.Errorf("removing %s:%s: %w", mixtapeName, tag, err)
	}

	// Clean up the parent name directory if no tags remain.
	remaining, err := os.ReadDir(nameDir)
	if err == nil && len(remaining) == 0 {
		_ = os.Remove(nameDir)
	}

	return nil
}

// ParseNameTag splits a "name[:tag]" string into name and tag components.
// If no tag is present, tag is returned as empty.
func ParseNameTag(ref string) (name, tag string) {
	if idx := strings.Index(ref, ":"); idx != -1 {
		return ref[:idx], ref[idx+1:]
	}
	return ref, ""
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
