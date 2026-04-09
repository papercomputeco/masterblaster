package mixtapes

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/papercomputeco/masterblaster/pkg/ui"
)

// catalogTimeout bounds how long ListCatalog will wait on the registry's
// /v2/_catalog endpoint before falling back to the offline list.
const catalogTimeout = 10 * time.Second

// fallbackMixtapes is used only when the registry catalog query fails
// (offline, DNS, registry down). Keep this list minimal — it should only
// contain names known to have a working :latest manifest.
var fallbackMixtapes = []string{"coder"}

// priorityMixtapes are surfaced first in catalog listings. Lower number =
// higher priority; entries not in the map sort after all priority entries
// and are then ordered alphabetically. Currently only "coder" has a fully
// working multi-arch :latest manifest; remove this bias once the other
// mixtapes are republished.
var priorityMixtapes = map[string]int{"coder": 0}

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

// ListCatalog queries the OCI registry's /v2/_catalog endpoint for the
// repositories under the mixtapes/ prefix and returns them as CatalogEntries.
// On network failure or an empty result it falls back to a small hardcoded
// list of names known to have a working :latest manifest.
func ListCatalog() ([]CatalogEntry, error) {
	reg, err := name.NewRegistry(DefaultDownloadRegistry)
	if err != nil {
		return nil, fmt.Errorf("parsing registry %q: %w", DefaultDownloadRegistry, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), catalogTimeout)
	defer cancel()

	repos, err := remote.Catalog(ctx, reg)
	if err != nil {
		ui.Warn("registry catalog query failed (%v); using fallback list", err)
		return fallbackCatalog(), nil
	}

	prefix := defaultRepoPrefix + "/"
	var entries []CatalogEntry
	for _, r := range repos {
		short := strings.TrimPrefix(r, prefix)
		if short == "" || short == r {
			// Either empty after trimming, or the prefix wasn't there at all.
			continue
		}
		entries = append(entries, CatalogEntry{
			Name: short,
			Repo: DefaultDownloadRegistry + "/" + r,
		})
	}

	if len(entries) == 0 {
		return fallbackCatalog(), nil
	}

	sortCatalog(entries)
	return entries, nil
}

// sortCatalog orders entries by priorityMixtapes first, then alphabetically
// by short name. Stable so equal-priority entries keep input order.
func sortCatalog(entries []CatalogEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		pi, oki := priorityMixtapes[entries[i].Name]
		pj, okj := priorityMixtapes[entries[j].Name]
		switch {
		case oki && !okj:
			return true
		case !oki && okj:
			return false
		case oki && okj && pi != pj:
			return pi < pj
		default:
			return entries[i].Name < entries[j].Name
		}
	})
}

func fallbackCatalog() []CatalogEntry {
	out := make([]CatalogEntry, 0, len(fallbackMixtapes))
	for _, m := range fallbackMixtapes {
		out = append(out, CatalogEntry{
			Name: m,
			Repo: DefaultDownloadRegistry + "/" + defaultRepoPrefix + "/" + m,
		})
	}
	return out
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
