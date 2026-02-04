package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/paper-compute-co/masterblaster/internal/vm"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all VMs with their status",
	Long:    `Show all known VM instances with their current state, SSH address, CPUs, and memory.`,
	Args:    cobra.NoArgs,
	RunE:    runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	backend := vm.NewQEMUBackend(globalConfigDir())

	instances, err := backend.List(cmd.Context())
	if err != nil {
		return fmt.Errorf("listing VMs: %w", err)
	}

	if len(instances) == 0 {
		fmt.Println("No VMs found. Use 'mb init <config.toml>' to create one.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATE\tSSH\tCPUs\tMEMORY")

	for _, inst := range instances {
		state, _ := inst.LoadState()
		cpus := 0
		memory := ""
		if state != nil {
			cpus = state.CPUs
			memory = state.Memory
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
			inst.Name,
			inst.VMState,
			inst.SSHAddress,
			cpus,
			memory,
		)
	}

	return w.Flush()
}
