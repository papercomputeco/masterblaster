// Package applycmder provides the apply command for updating agent
// configuration on a running StereOS sandbox VM.
package applycmder

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/daemon/client"
	"github.com/papercomputeco/masterblaster/pkg/ui"
)

const applyLongDesc string = `Apply updated agent configuration to a running StereOS sandbox.

Reads the jcard.toml (or the file given with -f/--config), sends the
[[agents]] configuration and [secrets] to the sandbox via stereosd, and
agentd reconciles agents within ~5 seconds: starting new agents,
stopping removed ones, and restarting any whose config changed.

The sandbox must already be running (use 'mb up' to create one first).

Examples:
  mb apply
  mb apply -f ./jcard.toml
  mb apply -f ./agents-only.toml --name my-sandbox`

const applyShortDesc string = "Apply agent configuration to a running sandbox"

// NewApplyCmd creates the apply command.
func NewApplyCmd(configDirFn func() string) *cobra.Command {
	var (
		cfgPath string
		name    string
	)

	cmd := &cobra.Command{
		Use:   "apply",
		Short: applyShortDesc,
		Long:  applyLongDesc,
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runApply(configDirFn(), cfgPath, name)
		},
	}

	cmd.Flags().StringVarP(&cfgPath, "config", "f", "", "Path to jcard.toml (default: ./jcard.toml)")
	cmd.Flags().StringVar(&name, "name", "", "Target sandbox name (default: name from config)")

	return cmd
}

func runApply(baseDir, cfgPath, name string) error {
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
	if err := ui.Step(os.Stderr, "Applying configuration...", func() error {
		_, err := c.Apply(name, cfgPath)
		return err
	}); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr)
	ui.Success("Configuration applied")
	fmt.Fprintln(os.Stderr)
	ui.Info("Agents will converge within ~5 seconds")
	ui.Info("Check status with: mb status")

	return nil
}
