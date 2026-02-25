// Package listcmder provides the list command for showing all known
// sandbox instances.
package listcmder

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/client"
	"github.com/papercomputeco/masterblaster/pkg/ui"
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

	tbl := &ui.Table{
		Headers:  []string{"NAME", "STATE", "MIXTAPE", "CPUs", "MEMORY", "SSH"},
		StateCol: 1,
	}
	for _, sb := range resp.Sandboxes {
		tbl.Rows = append(tbl.Rows, []string{
			sb.Name, sb.State, sb.Mixtape,
			fmt.Sprintf("%d", sb.CPUs), sb.Memory, sb.SSHAddress,
		})
	}
	tbl.Render(os.Stdout)

	return nil
}
