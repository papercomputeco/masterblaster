package mixtapes

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/papercomputeco/masterblaster/pkg/ui"
)

// knownMixtapes is the list of mixtape names published in the default registry.
// Update this list when new mixtapes are added to download.stereos.ai.
var knownMixtapes = []string{
	"coder-arm64",
	"coder-x86",
}

// CatalogEntry describes a mixtape repository in the remote registry.
type CatalogEntry struct {
	Name string // Short name (e.g. "coder-arm64")
	Repo string // Full repository path (e.g. "download.stereos.ai/mixtapes/coder-arm64")
}

// TagEntry describes a tag for a mixtape in the remote registry.
type TagEntry struct {
	Mixtape string // Short mixtape name
	Tag     string // Tag string (e.g. "latest", "0.1.0")
}

// ListCatalog returns the known mixtape repositories in the default registry.
func ListCatalog() ([]CatalogEntry, error) {
	var entries []CatalogEntry
	for _, m := range knownMixtapes {
		entries = append(entries, CatalogEntry{
			Name: m,
			Repo: DefaultDownloadRegistry + "/" + defaultRepoPrefix + "/" + m,
		})
	}
	return entries, nil
}

// ListTags queries the OCI registry for all tags of a given mixtape.
// The mixtapeName can be a short name (e.g. "coder-arm64") or a full
// OCI repository reference (e.g. "myregistry.io/my/repo").
func ListTags(mixtapeName string) ([]TagEntry, error) {
	repoStr := mixtapeName
	if !strings.Contains(repoStr, "/") {
		repoStr = DefaultDownloadRegistry + "/" + defaultRepoPrefix + "/" + repoStr
	}

	repo, err := name.NewRepository(repoStr)
	if err != nil {
		return nil, fmt.Errorf("parsing repository %q: %w", repoStr, err)
	}

	tags, err := remote.List(repo)
	if err != nil {
		return nil, fmt.Errorf("listing tags for %s: %w", repo.String(), err)
	}

	// Derive a short name for display.
	shortName := mixtapeName
	if strings.Contains(shortName, "/") {
		parts := strings.Split(shortName, "/")
		shortName = parts[len(parts)-1]
	}

	var entries []TagEntry
	for _, tag := range tags {
		entries = append(entries, TagEntry{
			Mixtape: shortName,
			Tag:     tag,
		})
	}

	return entries, nil
}

// PrintCatalog writes a styled table of registry mixtape repositories to stdout.
func PrintCatalog(entries []CatalogEntry) {
	if len(entries) == 0 {
		fmt.Println("No mixtapes found.")
		return
	}

	tbl := &ui.Table{
		Headers:  []string{"NAME", "REPOSITORY"},
		StateCol: -1,
	}
	for _, e := range entries {
		tbl.Rows = append(tbl.Rows, []string{e.Name, e.Repo})
	}
	tbl.Render(os.Stdout)
	fmt.Printf("\nList tags with: mb mixtapes list <name>\n")
}

// PrintTags writes a styled table of tags for a mixtape to stdout.
func PrintTags(entries []TagEntry) {
	if len(entries) == 0 {
		fmt.Println("No tags found.")
		return
	}

	mixtapeName := entries[0].Mixtape
	tbl := &ui.Table{
		Headers:  []string{"MIXTAPE", "TAG"},
		StateCol: -1,
	}
	for _, e := range entries {
		tbl.Rows = append(tbl.Rows, []string{e.Mixtape, e.Tag})
	}
	tbl.Render(os.Stdout)
	fmt.Printf("\nPull with: mb pull %s:<tag>\n", mixtapeName)
}
