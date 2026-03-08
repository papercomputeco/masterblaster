// Package upcmder provides the up command for creating and starting
// a StereOS sandbox VM.
package upcmder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/daemon"
	"github.com/papercomputeco/masterblaster/pkg/daemon/client"
	"github.com/papercomputeco/masterblaster/pkg/telemetry"
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			telem := telemetry.FromContext(cmd.Context())
			return runUp(configDirFn(), cfgPath, telem)
		},
	}

	cmd.Flags().StringVar(&cfgPath, "config", "", "Path to jcard.toml (default: ./jcard.toml)")

	return cmd
}

func runUp(baseDir, cfgPath string, telem *telemetry.PosthogClient) error {
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

	c := client.New(baseDir)
	var resp *daemon.Response
	if err := ui.Step(os.Stderr, "Starting sandbox...", func() error {
		var stepErr error
		resp, stepErr = c.Up("", cfgPath)
		return stepErr
	}); err != nil {
		telem.CaptureUp("", false)
		return err
	}

	telem.CaptureUp("", true)

	if len(resp.Sandboxes) > 0 {
		sb := resp.Sandboxes[0]
		fmt.Fprintln(os.Stderr)
		ui.Success("Sandbox %q launched", sb.Name)
		fmt.Fprintln(os.Stderr)
		if sb.SSHKeyPath != "" {
			short := shortenHome(sb.SSHKeyPath)
			ui.Info("ssh -p %d -i %s admin@127.0.0.1", sb.SSHPort, short)
		} else {
			ui.Info("ssh -p %d admin@127.0.0.1", sb.SSHPort)
		}
		ui.Info("mb ssh %s", sb.Name)
	}

	return nil
}

// shortenHome replaces the user's home directory prefix with ~.
func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
