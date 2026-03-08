//go:build linux && arm64

package vm

// NewPlatformBackend returns the QEMU backend for Linux on ARM64,
// configured for KVM acceleration with native vsock support.
func NewPlatformBackend(baseDir string) (Backend, error) {
	const binary = "qemu-system-aarch64"

	platform := &QEMUPlatformConfig{
		Accelerator:  "kvm",
		Binary:       binary,
		MachineType:  "virt",
		MachineProps: "highmem=on",
		EFISearchPaths: []string{
			"{qemu_prefix}/share/qemu/edk2-aarch64-code.fd",
			"/usr/share/qemu/edk2-aarch64-code.fd",
			"/usr/share/AAVMF/AAVMF_CODE.fd",
			"/usr/share/edk2/aarch64/QEMU_CODE.fd",
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
