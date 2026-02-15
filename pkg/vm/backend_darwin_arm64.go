//go:build darwin && arm64

package vm

// NewPlatformBackend returns the appropriate VM backend for the current
// platform. On darwin/arm64, this returns the QEMU backend configured for
// Apple Silicon with HVF acceleration.
//
// vsock devices (vhost-vsock-*) are not available in QEMU on macOS/HVF,
// so the stereosd control plane is forwarded via TCP through QEMU's
// user-mode networking.
func NewPlatformBackend(baseDir string) (Backend, error) {
	// TODO: Implement Apple Virtualization.framework backend
	// Apple Virt.framework supports native vsock via VZVirtioSocketDevice.
	// return NewAppleVirtBackend(baseDir)

	platform := &QEMUPlatformConfig{
		Accelerator: "hvf",
		Binary:      "qemu-system-aarch64",
		MachineType: "virt",
		EFISearchPaths: []string{
			"{qemu_prefix}/share/qemu/edk2-aarch64-code.fd",
			"/opt/homebrew/share/qemu/edk2-aarch64-code.fd",
		},
		ControlPlaneMode: "tcp",
	}

	return NewQEMUBackend(baseDir, platform), nil
}
