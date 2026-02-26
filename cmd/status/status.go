// Package statuscmder provides the status command for displaying the
// current state of a sandbox.
package statuscmder

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/daemon"
	"github.com/papercomputeco/masterblaster/pkg/daemon/client"
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

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tSTATE\tMIXTAPE\tCPUs\tMEMORY\tSSH")
	for _, sb := range sandboxes {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
			sb.Name,
			sb.State,
			sb.Mixtape,
			sb.CPUs,
			sb.Memory,
			sb.SSHAddress,
		)
	}
	_ = w.Flush()
}
