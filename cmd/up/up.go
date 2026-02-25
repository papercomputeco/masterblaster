// Package upcmder provides the up command for creating and starting
// a StereOS sandbox VM.
package upcmder

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/daemon/client"
	"github.com/papercomputeco/masterblaster/pkg/ui"
)

const upLongDesc string = `Boot a new StereOS sandbox VM using the jcard.toml in the current
directory (or the path given with --config). Communicates with the
Masterblaster daemon to create, configure, and start the VM.

If the daemon is not running, it will be automatically started in the
background.

Examples:
  mb up
  mb up --config /path/to/jcard.toml`

const upShortDesc string = "Create and start a sandbox"

// NewUpCmd creates the up command.
func NewUpCmd(configDirFn func() string) *cobra.Command {
	var cfgPath string

	cmd := &cobra.Command{
		Use:   "up",
		Short: upShortDesc,
		Long:  upLongDesc,
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runUp(configDirFn(), cfgPath)
		},
	}

	cmd.Flags().StringVar(&cfgPath, "config", "", "Path to jcard.toml (default: ./jcard.toml)")

	return cmd
}

func runUp(baseDir, cfgPath string) error {
	// Resolve config path
	if cfgPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		cfgPath = filepath.Join(cwd, "jcard.toml")
	}

	var err error
	cfgPath, err = filepath.Abs(cfgPath)
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}

	if _, err := os.Stat(cfgPath); err != nil {
		return fmt.Errorf("config not found at %s\n\nCreate one with: mb init", cfgPath)
	}

	if err := client.EnsureDaemon(baseDir); err != nil {
		return err
	}

	ui.Status("Starting sandbox...")

	c := client.New(baseDir)
	resp, err := c.Up("", cfgPath)
	if err != nil {
		return err
	}

	if len(resp.Sandboxes) > 0 {
		sb := resp.Sandboxes[0]
		ui.Success("Sandbox %q started", sb.Name)
		if sb.SSHKeyPath != "" {
			ui.Info("SSH:   ssh -i %s -p %d admin@127.0.0.1", sb.SSHKeyPath, sb.SSHPort)
		} else {
			ui.Info("SSH:   ssh -p %d admin@127.0.0.1", sb.SSHPort)
		}
		ui.Info("Or:    mb ssh %s", sb.Name)
	}

	return nil
}
