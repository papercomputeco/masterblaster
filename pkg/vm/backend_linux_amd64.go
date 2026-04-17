//go:build linux && amd64

package vm

// LinuxAMD64PlatformConfig returns the QEMU platform config for
// linux/amd64: KVM acceleration, q35 chipset, AF_VSOCK control plane
// via vhost-vsock-pci. boot() falls back to TCP hostfwd at runtime when
// the host lacks vhost_vsock (WSL2, kernels without the module loaded).
//
// EFI firmware paths cover the common distros:
//   - NixOS / generic: edk2-x86_64-code.fd next to the QEMU prefix
//   - Debian/Ubuntu:   /usr/share/OVMF/OVMF_CODE.fd
//   - Fedora/RHEL:     /usr/share/edk2/ovmf/OVMF_CODE.fd
//
// Exported so cmd/vmhost can share the same config without duplicating
// the struct literal.
func LinuxAMD64PlatformConfig() *QEMUPlatformConfig {
	return &QEMUPlatformConfig{
		Accelerator: "kvm",
		Binary:      "qemu-system-x86_64",
		// q35 is the modern x86 chipset; i440fx ("pc") is legacy.
		MachineType: "q35",
		EFISearchPaths: []string{
			"{qemu_prefix}/share/qemu/edk2-x86_64-code.fd",
			"/usr/share/qemu/edk2-x86_64-code.fd",
			"/usr/share/OVMF/OVMF_CODE.fd",
			"/usr/share/OVMF/OVMF_CODE_4M.fd",
			"/usr/share/edk2/ovmf/OVMF_CODE.fd",
			"/usr/share/edk2/x64/OVMF_CODE.fd",
		},
		ControlPlaneMode: "vsock",
		VsockDevice:      "vhost-vsock-pci",
		DirectKernelBoot: true,
		DiskAIO:          "io_uring",
		DiskCache:        "none",
	}
}

// NewPlatformBackend returns the QEMU backend for linux/amd64.
func NewPlatformBackend(baseDir string) (Backend, error) {
	return NewQEMUBackend(baseDir, LinuxAMD64PlatformConfig()), nil
}
