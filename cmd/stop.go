package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/paper-compute-co/masterblaster/internal/ui"
	"github.com/paper-compute-co/masterblaster/internal/vm"
)

var stopCmd = &cobra.Command{
	Use:   "stop [name]",
	Short: "Gracefully stop a running VM",
	Long: `Send ACPI shutdown to the VM via QMP for a clean systemd shutdown.
Falls back to force kill after a timeout.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	backend := vm.NewQEMUBackend(globalConfigDir())

	vmName, err := resolveVMName(backend, args, cmd)
	if err != nil {
		return err
	}

	inst, err := backend.LoadInstance(vmName)
	if err != nil {
		return fmt.Errorf("VM %q not found: %w", vmName, err)
	}

	status, err := backend.Status(cmd.Context(), inst)
	if err != nil {
		return fmt.Errorf("checking VM status: %w", err)
	}
	if status != vm.StateRunning {
		ui.Warn("VM %q is not running (state: %s)", vmName, status)
		return nil
	}

	ui.Status("Stopping VM %q...", vmName)
	if err := backend.Stop(cmd.Context(), inst, 30*time.Second); err != nil {
		return fmt.Errorf("stopping VM: %w", err)
	}

	ui.Success("VM %q stopped", vmName)
	return nil
}
