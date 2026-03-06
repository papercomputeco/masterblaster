//go:build darwin && arm64

package vmhostcmder

import (
	"context"
	"log"
	"time"

	"github.com/papercomputeco/masterblaster/pkg/vm"
	"github.com/papercomputeco/masterblaster/pkg/vmhost"
)

// appleVirtController adapts the AppleVirtBackend to the VMController interface.
type appleVirtController struct {
	backend *vm.AppleVirtBackend
	inst    *vm.Instance
	logger  *log.Logger
}

func (c *appleVirtController) State() string {
	state, _ := c.backend.Status(context.Background(), c.inst)
	return string(state)
}

func (c *appleVirtController) Stop(ctx context.Context, timeout time.Duration) error {
	return c.backend.Down(ctx, c.inst, timeout)
}

func (c *appleVirtController) ForceStop(ctx context.Context) error {
	return c.backend.ForceDown(ctx, c.inst)
}

func (c *appleVirtController) SSHPort() int {
	return c.inst.SSHPort
}

func (c *appleVirtController) Backend() string {
	return "applevirt"
}

func (c *appleVirtController) Apply(ctx context.Context, configContent string, secrets map[string]string) error {
	return c.backend.ControlPlaneApply(ctx, c.inst.Name, configContent, secrets)
}

func (c *appleVirtController) Wait() error {
	return c.backend.WaitVM(c.inst.Name)
}

func bootAppleVirt(ctx context.Context, baseDir string, inst *vm.Instance, logger *log.Logger) (vmhost.VMController, error) {
	backend := vm.NewAppleVirtBackend(baseDir)

	logger.Printf("booting Apple Virt VM %q", inst.Name)
	if err := backend.Boot(ctx, inst); err != nil {
		return nil, err
	}

	return &appleVirtController{
		backend: backend,
		inst:    inst,
		logger:  logger,
	}, nil
}

// getPlatformConfig returns the QEMU platform configuration for darwin/arm64.
func getPlatformConfig(_ string) *vm.QEMUPlatformConfig {
	return &vm.QEMUPlatformConfig{
		Accelerator: "hvf",
		Binary:      "qemu-system-aarch64",
		MachineType: "virt",
		EFISearchPaths: []string{
			"{qemu_prefix}/share/qemu/edk2-aarch64-code.fd",
			"/opt/homebrew/share/qemu/edk2-aarch64-code.fd",
		},
		ControlPlaneMode: "tcp",
		DirectKernelBoot: true,
	}
}
