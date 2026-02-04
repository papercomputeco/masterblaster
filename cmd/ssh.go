package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/paper-compute-co/masterblaster/internal/ssh"
	"github.com/paper-compute-co/masterblaster/internal/ui"
	"github.com/paper-compute-co/masterblaster/internal/vm"
)

var sshCmd = &cobra.Command{
	Use:   "ssh [name]",
	Short: "SSH into a running VM",
	Long: `Connect to a running VM via SSH. Replaces the current process with
the ssh binary for a clean interactive experience.

If no name is given and only one VM is running, connects to that VM.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSSH,
}

func init() {
	rootCmd.AddCommand(sshCmd)
}

func runSSH(cmd *cobra.Command, args []string) error {
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
	if err != nil || status != vm.StateRunning {
		return fmt.Errorf("VM %q is not running (state: %s)", vmName, status)
	}

	// Load state to get SSH params
	state, err := inst.LoadState()
	if err != nil {
		return fmt.Errorf("loading VM state: %w", err)
	}

	if verbose {
		ui.Info("Connecting to %s@127.0.0.1:%d", state.SSHUser, state.SSHHostPort)
	}

	// Replace process with ssh — never returns on success
	return ssh.ExecSSH(state.SSHUser, "127.0.0.1", state.SSHHostPort, state.IdentityFile)
}

// resolveVMName determines the VM name from the args, or if only one VM
// is running, uses that one.
func resolveVMName(backend *vm.QEMUBackend, args []string, cmd *cobra.Command) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}

	// If no name given, try to find the only running VM
	instances, err := backend.List(cmd.Context())
	if err != nil {
		return "", fmt.Errorf("listing VMs: %w", err)
	}

	var running []*vm.Instance
	for _, inst := range instances {
		if inst.VMState == vm.StateRunning {
			running = append(running, inst)
		}
	}

	switch len(running) {
	case 0:
		return "", fmt.Errorf("no running VMs found")
	case 1:
		return running[0].Name, nil
	default:
		names := make([]string, len(running))
		for i, inst := range running {
			names[i] = inst.Name
		}
		return "", fmt.Errorf("multiple running VMs found, please specify one: %v", names)
	}
}
