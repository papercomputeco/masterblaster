// Package destroycmder provides the destroy command for removing a sandbox
// and all its on-disk resources.
package destroycmder

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/daemon/client"
	"github.com/papercomputeco/masterblaster/pkg/ui"
)

const destroyLongDesc string = `Destroy a sandbox by first attempting a graceful shutdown and then
removing all on-disk resources (disk image, EFI vars, state, etc.) from
~/.mb/vms/.

This is a destructive operation and will prompt for confirmation unless
--yes is provided. Use --force to forcibly kill a hung VM before removing.

Examples:
  mb destroy my-sandbox
  mb destroy --yes
  mb destroy --force --yes`

const destroyShortDesc string = "Remove a sandbox and all its resources"

// NewDestroyCmd creates the destroy command.
func NewDestroyCmd(configDirFn func() string) *cobra.Command {
	var (
		yes   bool
		force bool
	)

	cmd := &cobra.Command{
		Use:   "destroy [name]",
		Short: destroyShortDesc,
		Long:  destroyLongDesc,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return runDestroy(configDirFn(), name, yes, force)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&force, "force", false, "Force kill a hung VM before removing")

	return cmd
}

func runDestroy(baseDir, name string, yes, force bool) error {
	if !yes {
		target := name
		if target == "" {
			target = "the sandbox"
		}
		fmt.Fprintf(os.Stderr, "Destroy %s? This will delete all sandbox data. [y/N] ", target)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil
		}
	}

	if err := client.EnsureDaemon(baseDir); err != nil {
		return err
	}

	c := client.New(baseDir)

	if force {
		_, _ = c.Down(name, true)
	}

	ui.Status("Destroying sandbox...")
	_, err := c.Destroy(name)
	if err != nil {
		return err
	}

	ui.Success("Sandbox destroyed")
	return nil
}
