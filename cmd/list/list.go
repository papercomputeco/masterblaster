// Package listcmder provides the list command for showing all known
// sandbox instances.
package listcmder

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/client"
	"github.com/papercomputeco/masterblaster/pkg/daemon"
)

const listLongDesc string = `Show all known sandbox instances with their current state, mixtape,
resources, and SSH address.

Examples:
  mb list
  mb ls`

const listShortDesc string = "List all sandboxes"

// NewListCmd creates the list command.
func NewListCmd(configDirFn func() string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   listShortDesc,
		Long:    listLongDesc,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runList(configDirFn())
		},
	}

	return cmd
}

func runList(baseDir string) error {
	c := client.New(baseDir)
	resp, err := c.List()
	if err != nil {
		return fmt.Errorf("listing sandboxes: %w", err)
	}

	if len(resp.Sandboxes) == 0 {
		fmt.Println("No sandboxes found. Create one with: mb init && mb up")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tSTATE\tMIXTAPE\tCPUs\tMEMORY\tSSH")
	for _, sb := range resp.Sandboxes {
		printSandboxRow(w, sb)
	}
	_ = w.Flush()

	return nil
}

func printSandboxRow(w *tabwriter.Writer, sb daemon.SandboxInfo) {
	_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
		sb.Name,
		sb.State,
		sb.Mixtape,
		sb.CPUs,
		sb.Memory,
		sb.SSHAddress,
	)
}
