// Package vmhostcmder provides the hidden vmhost subcommand that manages
// a single VM's lifecycle. The daemon spawns one vmhost process per VM;
// each vmhost process holds the hypervisor handle (QEMU child or Apple
// Virt in-process VM) and exposes a control socket for the daemon to
// communicate with.
package vmhostcmder

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/vm"
	"github.com/papercomputeco/masterblaster/pkg/vmhost"
)

// NewVMHostCmd creates the vmhost command. This is a hidden internal
// subcommand used by the daemon to manage individual VM processes.
func NewVMHostCmd(configDirFn func() string) *cobra.Command {
	var (
		name    string
		backend string
	)

	cmd := &cobra.Command{
		Use:    "vmhost",
		Short:  "Manage a single VM (internal)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runVMHost(cmd.Context(), configDirFn(), name, backend)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "VM name (required)")
	cmd.Flags().StringVar(&backend, "backend", "", "Backend type: qemu or applevirt (required)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("backend")

	return cmd
}

func runVMHost(ctx context.Context, baseDir, name, backend string) error {
	// Set up logging to vmhost.log in the VM directory
	vmDir := vm.VMsDir(baseDir)
	inst := &vm.Instance{
		Name: name,
		Dir:  fmt.Sprintf("%s/%s", vmDir, name),
	}

	logFile, err := os.OpenFile(inst.VMHostLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening vmhost log: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	logger := log.New(logFile, fmt.Sprintf("[vmhost:%s] ", name), log.LstdFlags)
	// Also send stdlib log output to the file for backend log.Printf calls
	log.SetOutput(logFile)
	log.SetPrefix(fmt.Sprintf("[vmhost:%s] ", name))

	logger.Printf("starting vmhost for %q (backend=%s, base-dir=%s)", name, backend, baseDir)

	// Boot the VM using the appropriate backend and get a controller
	controller, err := bootVM(ctx, baseDir, inst, backend, logger)
	if err != nil {
		logger.Printf("boot failed: %v", err)
		return fmt.Errorf("booting VM: %w", err)
	}

	logger.Printf("VM booted successfully (state=%s, ssh_port=%d)", controller.State(), controller.SSHPort())

	// Handle signals: SIGTERM/SIGINT trigger graceful shutdown
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run the control socket server
	server := vmhost.NewServer(
		inst.VMHostSocketPath(),
		inst.VMHostPIDPath(),
		controller,
		logger,
	)

	return server.Run(ctx)
}

// bootVM creates the appropriate backend and boots the VM. It returns a
// VMController that the vmhost server uses for lifecycle management.
// This function is the bridge between the vmhost command and the backend
// implementations.
func bootVM(ctx context.Context, baseDir string, inst *vm.Instance, backend string, logger *log.Logger) (vmhost.VMController, error) {
	switch backend {
	case "qemu":
		return bootQEMU(ctx, baseDir, inst, logger)
	case "applevirt":
		return bootAppleVirt(ctx, baseDir, inst, logger)
	default:
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}
}

// qemuController adapts the QEMUBackend to the VMController interface.
type qemuController struct {
	backend *vm.QEMUBackend
	inst    *vm.Instance
	logger  *log.Logger
}

func (c *qemuController) State() string {
	state, _ := c.backend.Status(context.Background(), c.inst)
	return string(state)
}

func (c *qemuController) Stop(ctx context.Context, timeout time.Duration) error {
	return c.backend.Down(ctx, c.inst, timeout)
}

func (c *qemuController) ForceStop(ctx context.Context) error {
	return c.backend.ForceDown(ctx, c.inst)
}

func (c *qemuController) SSHPort() int {
	return c.inst.SSHPort
}

func (c *qemuController) Backend() string {
	return "qemu"
}

func (c *qemuController) Wait() error {
	return c.backend.WaitQEMU()
}

func bootQEMU(ctx context.Context, baseDir string, inst *vm.Instance, logger *log.Logger) (vmhost.VMController, error) {
	platform := getPlatformConfig(baseDir)
	backend := vm.NewQEMUBackend(baseDir, platform)

	logger.Printf("booting QEMU VM %q", inst.Name)
	if err := backend.Boot(ctx, inst); err != nil {
		return nil, err
	}

	return &qemuController{
		backend: backend,
		inst:    inst,
		logger:  logger,
	}, nil
}
