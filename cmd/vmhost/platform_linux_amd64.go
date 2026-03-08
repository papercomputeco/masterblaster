//go:build linux && amd64

package vmhostcmder

import (
	"context"
	"fmt"
	"log"

	"github.com/papercomputeco/masterblaster/pkg/vm"
	"github.com/papercomputeco/masterblaster/pkg/vmhost"
)

func bootAppleVirt(_ context.Context, _ string, _ *vm.Instance, _ *log.Logger) (vmhost.VMController, error) {
	return nil, fmt.Errorf("apple Virtualization.framework is only available on macOS/Apple Silicon")
}

// getPlatformConfig returns the QEMU platform configuration for Linux x86_64.
func getPlatformConfig(_ string) *vm.QEMUPlatformConfig {
	const binary = "qemu-system-x86_64"

	return &vm.QEMUPlatformConfig{
		Accelerator: "kvm",
		Binary:      binary,
		MachineType: "q35",
		EFISearchPaths: []string{
			"{qemu_prefix}/share/qemu/edk2-x86_64-code.fd",
			"/usr/share/qemu/edk2-x86_64-code.fd",
			"/usr/share/OVMF/OVMF_CODE.fd",
			"/usr/share/edk2/x86_64/OVMF_CODE.fd",
		},
		ControlPlaneMode: "vsock",
		VsockDevice:      "vhost-vsock-pci",
		DirectKernelBoot: true,
		DiskAIO:          "io_uring",
		DiskCache:        "none",
		BridgeHelper:     findBridgeHelper(binary),
	}
}
