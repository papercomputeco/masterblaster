// Package initcmder provides the init command for creating a jcard.toml
// configuration file in the current directory.
package initcmder

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/config"
	"github.com/papercomputeco/masterblaster/pkg/ui"
)

const initLongDesc string = `Initialize a new Masterblaster sandbox configuration by creating a
jcard.toml file in the current directory. This file defines the mixtape,
resources, network, shared directories, and agent configuration.

Edit the generated jcard.toml to customize the sandbox, then run "mb up"
to boot it.

Examples:
  mb init`

const initShortDesc string = "Create a jcard.toml configuration file"

// NewInitCmd creates the init command.
func NewInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: initShortDesc,
		Long:  initLongDesc,
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runInit()
		},
	}

	return cmd
}

func runInit() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	jcardPath := filepath.Join(cwd, "jcard.toml")

	if _, err := os.Stat(jcardPath); err == nil {
		ui.Warn("jcard.toml already exists in this directory.")
		return nil
	}

	content := config.DefaultJcardTOML()
	if err := os.WriteFile(jcardPath, []byte(content), 0644); err != nil {
		return err
	}

	ui.Success("Created jcard.toml")
	ui.Info("Edit the file then run `mb up` to boot a sandbox.")
	return nil
}
