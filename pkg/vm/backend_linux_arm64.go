//go:build linux && arm64

package vm

// LinuxARM64PlatformConfig returns the QEMU platform config for
// linux/arm64: KVM acceleration on the virt machine with native vsock
// via vhost-vsock-pci.
//
// Exported so cmd/vmhost can share the same config without duplicating
// the struct literal.
func LinuxARM64PlatformConfig() *QEMUPlatformConfig {
	return &QEMUPlatformConfig{
		Accelerator: "kvm",
		Binary:      "qemu-system-aarch64",
		// `highmem=on` extends the aarch64 virt machine's 40-bit IPA space
		// so guests with >3GiB RAM map correctly. q35 doesn't accept this
		// option — don't copy it into the amd64 backend.
		MachineType: "virt,highmem=on",
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
	}
}

// NewPlatformBackend returns the QEMU backend for linux/arm64.
func NewPlatformBackend(baseDir string) (Backend, error) {
	// TODO: Implement KVM/libvirt backend for better performance.
	return NewQEMUBackend(baseDir, LinuxARM64PlatformConfig()), nil
}
