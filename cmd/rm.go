package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/paper-compute-co/masterblaster/internal/ui"
	"github.com/paper-compute-co/masterblaster/internal/vm"
)

var rmCmd = &cobra.Command{
	Use:   "rm [name]",
	Short: "Remove a VM and clean up all its resources",
	Long: `Destroy a VM and delete all associated resources including the disk
overlay, cloud-init ISO, EFI vars, state file, and QMP socket.

If the VM is still running, it will be stopped first.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRm,
}

func init() {
	rootCmd.AddCommand(rmCmd)
}

func runRm(cmd *cobra.Command, args []string) error {
	backend := vm.NewQEMUBackend(globalConfigDir())

	vmName, err := resolveVMName(backend, args, cmd)
	if err != nil {
		return err
	}

	inst, err := backend.LoadInstance(vmName)
	if err != nil {
		return fmt.Errorf("VM %q not found: %w", vmName, err)
	}

	ui.Status("Removing VM %q...", vmName)
	if err := backend.Remove(cmd.Context(), inst); err != nil {
		return fmt.Errorf("removing VM: %w", err)
	}

	ui.Success("VM %q removed", vmName)
	return nil
}
