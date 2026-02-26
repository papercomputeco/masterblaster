// Package mixtapescmder provides the mixtapes command group for managing
// locally available StereOS mixtape images.
package mixtapescmder

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/mixtapes"
	"github.com/papercomputeco/masterblaster/pkg/ui"
)

const mixtapesLongDesc string = `Manage locally available StereOS mixtapes (bootable VM images).

Mixtapes are pre-configured StereOS images bundled with agent harnesses
and workflows. Use "mb mixtapes ls" to see what's available locally and
"mb mixtapes pull <name>" to download new ones.

Examples:
  mb mixtapes ls
  mb mixtapes pull opencode-mixtape`

const mixtapesShortDesc string = "Manage StereOS mixtapes"

// NewMixtapesCmd creates the mixtapes command group with ls and pull subcommands.
func NewMixtapesCmd(configDirFn func() string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mixtapes",
		Short: mixtapesShortDesc,
		Long:  mixtapesLongDesc,
	}

	cmd.AddCommand(newMixtapesLsCmd(configDirFn))
	cmd.AddCommand(newMixtapesPullCmd(configDirFn))

	return cmd
}

func newMixtapesLsCmd(configDirFn func() string) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List locally available mixtapes",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMixtapesLs(configDirFn())
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

func runMixtapesLs(baseDir string) error {
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
