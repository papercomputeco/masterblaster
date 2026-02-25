// Package downcmder provides the down command for gracefully stopping
// a running sandbox.
package downcmder

import (
	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/daemon/client"
	"github.com/papercomputeco/masterblaster/pkg/ui"
)

const downLongDesc string = `Gracefully stop a running StereOS sandbox. Sends a shutdown command
to stereosd inside the VM, allowing it to unmount shared directories and
sync filesystems before powering off.

If no name is given and only one sandbox is running, that sandbox is stopped.

Use --force to immediately terminate the VM process.

Examples:
  mb down
  mb down my-sandbox
  mb down --force`

const downShortDesc string = "Stop a running sandbox"

// NewDownCmd creates the down command.
func NewDownCmd(configDirFn func() string) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "down [name]",
		Short: downShortDesc,
		Long:  downLongDesc,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return runDown(configDirFn(), name, force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force kill the VM process")

	return cmd
}

func runDown(baseDir, name string, force bool) error {
	if err := client.EnsureDaemon(baseDir); err != nil {
		return err
	}

	c := client.New(baseDir)

	ui.Status("Stopping sandbox...")
	_, err := c.Down(name, force)
	if err != nil {
		return err
	}

	ui.Success("Sandbox stopped")
	return nil
}
