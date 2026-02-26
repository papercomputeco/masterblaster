// Package versioncmder provides the "version" spf13/cobra command
// which prints the version, buildtime, and sha of the build.
package versioncmder

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/ui"
	"github.com/papercomputeco/masterblaster/pkg/utils"
)

type VersionCommander struct{}

func NewVersionCmd() *cobra.Command {
	cmder := &VersionCommander{}

	cmd := &cobra.Command{
		Use:   "version",
		Short: "displays version",
		Long:  "displays the version of this CLI",
		RunE: func(_ *cobra.Command, _ []string) error {
			return cmder.run()
		},
	}

	return cmd
}

func (c *VersionCommander) run() error {
	fmt.Fprintln(os.Stderr, ui.Label("Version:", utils.Version))
	fmt.Fprintln(os.Stderr, ui.Label("Sha:    ", utils.Sha))
	fmt.Fprintln(os.Stderr, ui.Label("Built:  ", utils.Buildtime))
	return nil
}
