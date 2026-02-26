// Package statuscmder provides the status command for displaying the
// current state of a sandbox.
package statuscmder

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/daemon"
	"github.com/papercomputeco/masterblaster/pkg/daemon/client"
	"github.com/papercomputeco/masterblaster/pkg/ui"
)

const statusLongDesc string = `Display the current state of a sandbox, including its name, state,
mixtape, resources, and SSH address.

Use --all to show all sandboxes.

Examples:
  mb status
  mb status my-sandbox
  mb status --all`

const statusShortDesc string = "Show the status of a sandbox"

// NewStatusCmd creates the status command.
func NewStatusCmd(configDirFn func() string) *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "status [name]",
		Short: statusShortDesc,
		Long:  statusLongDesc,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return runStatus(configDirFn(), name, all)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Show all sandboxes")

	return cmd
}

func runStatus(baseDir, name string, all bool) error {
	if err := client.EnsureDaemon(baseDir); err != nil {
		return err
	}

	c := client.New(baseDir)
	resp, err := c.Status(name, all)
	if err != nil {
		return err
	}

	printSandboxes(resp.Sandboxes)
	return nil
}

func printSandboxes(sandboxes []daemon.SandboxInfo) {
	if len(sandboxes) == 0 {
		fmt.Println("No sandboxes found. Create one with: mb init && mb up")
		return
	}

	tbl := &ui.Table{
		Headers:  []string{"NAME", "STATE", "MIXTAPE", "CPUs", "MEMORY", "SSH"},
		StateCol: 1,
	}
	for _, sb := range sandboxes {
		tbl.Rows = append(tbl.Rows, []string{
			sb.Name, sb.State, sb.Mixtape,
			fmt.Sprintf("%d", sb.CPUs), sb.Memory, sb.SSHAddress,
		})
	}
	tbl.Render(os.Stdout)
}
