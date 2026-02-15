//go:build linux

package vm

// NewPlatformBackend returns the appropriate VM backend for Linux.
// This returns the QEMU backend configured for KVM acceleration with
// native vsock support via vhost-vsock-pci.
func NewPlatformBackend(baseDir string) (Backend, error) {
	// TODO: Implement KVM/libvirt backend for better performance.

	platform := &QEMUPlatformConfig{
		Accelerator: "kvm",
		Binary:      "qemu-system-aarch64",
		MachineType: "virt",
		EFISearchPaths: []string{
			"{qemu_prefix}/share/qemu/edk2-aarch64-code.fd",
			"/usr/share/qemu/edk2-aarch64-code.fd",
			"/usr/share/AAVMF/AAVMF_CODE.fd",
			"/usr/share/edk2/aarch64/QEMU_CODE.fd",
		},
		ControlPlaneMode: "vsock",
		VsockDevice:      "vhost-vsock-pci",
	}

	return NewQEMUBackend(baseDir, platform), nil
}
