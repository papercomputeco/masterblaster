//go:build linux && amd64

package vm

// NewPlatformBackend returns the QEMU backend for Linux on x86_64,
// configured for KVM acceleration with native vsock support.
func NewPlatformBackend(baseDir string) (Backend, error) {
	const binary = "qemu-system-x86_64"

	platform := &QEMUPlatformConfig{
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

	return NewQEMUBackend(baseDir, platform), nil
}
