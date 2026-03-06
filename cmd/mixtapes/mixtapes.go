// Package mixtapescmder provides the mixtapes command group for managing
// StereOS mixtape images, both locally and in the OCI registry.
package mixtapescmder

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/mixtapes"
	"github.com/papercomputeco/masterblaster/pkg/ui"
)

const mixtapesLongDesc string = `Manage StereOS mixtapes (bootable VM images).

Mixtapes are pre-configured StereOS images bundled with agent harnesses
and workflows. Use "mb mixtapes list" to see what's available in the
registry, "mb mixtapes local" to see what's downloaded, and
"mb mixtapes pull <name>" to download new ones.

Examples:
  mb mixtapes list                  # List mixtapes in the registry
  mb mixtapes list opencode-mixtape # List tags for a mixtape
  mb mixtapes local                 # List locally downloaded mixtapes
  mb mixtapes pull opencode-mixtape`

const mixtapesShortDesc string = "Manage StereOS mixtapes"

// NewMixtapesCmd creates the mixtapes command group with list, local, pull, rm subcommands.
func NewMixtapesCmd(configDirFn func() string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mixtapes",
		Short: mixtapesShortDesc,
		Long:  mixtapesLongDesc,
	}

	cmd.AddCommand(newMixtapesListCmd())
	cmd.AddCommand(newMixtapesLocalCmd(configDirFn))
	cmd.AddCommand(newMixtapesPullCmd(configDirFn))
	cmd.AddCommand(newMixtapesRmCmd(configDirFn))

	return cmd
}

func newMixtapesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [name]",
		Short: "List mixtapes or tags in the remote registry",
		Long: `Query the OCI registry for available mixtapes.

With no arguments, lists known mixtape repositories.

With a mixtape name, lists all available tags for that mixtape.

Examples:
  mb mixtapes list                # List available mixtapes
  mb mixtapes list coder-arm64    # List tags for coder-arm64`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runMixtapesCatalog()
			}
			return runMixtapesTags(args[0])
		},
	}
}

func newMixtapesLocalCmd(configDirFn func() string) *cobra.Command {
	return &cobra.Command{
		Use:   "local",
		Short: "List locally downloaded mixtapes",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMixtapesLocal(configDirFn())
		},
	}
}

func newMixtapesPullCmd(configDirFn func() string) *cobra.Command {
	return &cobra.Command{
		Use:   "pull <name[:tag]>",
		Short: "Pull a mixtape from the registry",
		Long: `Download a StereOS mixtape from the Paper Compute registry (or a
third-party OCI registry) to ~/.config/mb/mixtapes/<name>/.

This is an alias for "mb pull <name[:tag]>".`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runMixtapesPull(configDirFn(), args[0])
		},
	}
}

func runMixtapesCatalog() error {
	entries, err := mixtapes.ListCatalog()
	if err != nil {
		return fmt.Errorf("listing mixtapes: %w", err)
	}
	mixtapes.PrintCatalog(entries)
	return nil
}

func runMixtapesTags(name string) error {
	ui.Status("Listing tags for %q...", name)
	entries, err := mixtapes.ListTags(name)
	if err != nil {
		return fmt.Errorf("listing tags for %q: %w", name, err)
	}
	mixtapes.PrintTags(entries)
	return nil
}

func runMixtapesLocal(baseDir string) error {
	list, err := mixtapes.List(baseDir)
	if err != nil {
		return err
	}
	mixtapes.PrintList(list)
	return nil
}

func runMixtapesPull(baseDir, name string) error {
	return ui.Step(os.Stderr, fmt.Sprintf("Pulling mixtape %q...", name), func() error {
		return mixtapes.Pull(baseDir, name)
	})
}

func newMixtapesRmCmd(configDirFn func() string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <name[:tag]>",
		Short: "Remove a locally downloaded mixtape",
		Long: `Remove a mixtape from the local disk.

With just a name, removes the mixtape and all of its tags.
With name:tag, removes only that specific tag.

Examples:
  mb mixtapes rm coder-arm64          # Remove all tags
  mb mixtapes rm coder-arm64:latest   # Remove only the "latest" tag`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			return runMixtapesRm(configDirFn(), args[0], force)
		},
	}

	cmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	return cmd
}

func runMixtapesRm(baseDir, ref string, force bool) error {
	name, tag := mixtapes.ParseNameTag(ref)

	display := name
	if tag != "" {
		display = name + ":" + tag
	}

	if !force {
		if !ui.Confirm("Remove mixtape %q?", display) {
			ui.Info("Aborted.")
			return nil
		}
	}

	if err := mixtapes.Remove(baseDir, name, tag); err != nil {
		return err
	}

	ui.Success("Removed %s", display)
	return nil
}
