package vm

// QEMUPlatformConfig contains platform-specific settings for the QEMU backend.
// It is populated by the build-tagged NewPlatformBackend() functions and
// injected into QEMUBackend at construction time.
//
// This struct captures the differences between running QEMU on macOS/HVF
// (Apple Silicon) vs. Linux/KVM, without scattering platform conditionals
// throughout the QEMU backend code.
type QEMUPlatformConfig struct {
	// Accelerator is the QEMU acceleration method: "hvf", "kvm", or "tcg".
	Accelerator string

	// Binary is the QEMU system emulator binary name (e.g.,
	// "qemu-system-aarch64" for ARM64 guests).
	Binary string

	// MachineType is the QEMU machine type (e.g., "virt" for ARM64,
	// "q35" for x86_64). Defaults to "virt" if empty.
	MachineType string

	// EFISearchPaths is an ordered list of paths to search for EFI firmware.
	// The special prefix "{qemu_prefix}" is replaced at runtime with the
	// resolved QEMU installation prefix (parent of the bin/ directory).
	// The first existing file wins.
	EFISearchPaths []string

	// ControlPlaneMode controls how the host communicates with stereosd
	// inside the guest VM.
	//
	//   "tcp"   -- Forward stereosd port via QEMU user-mode networking
	//              (hostfwd). Works on all platforms but the control plane
	//              shares the guest network stack. This is the only option
	//              on macOS/HVF where vsock devices are unavailable.
	//
	//   "vsock" -- Attach a vhost-vsock device and connect via AF_VSOCK.
	//              Requires Linux/KVM. Provides an isolated control plane
	//              that works even with network.mode = "none".
	//
	// Future modes: "virtio-serial" (chardev unix socket via
	// virtio-serial-device, works on macOS without guest networking).
	ControlPlaneMode string

	// VsockDevice is the QEMU device model name for native vsock.
	// Only used when ControlPlaneMode is "vsock".
	// Typically "vhost-vsock-pci" on x86 and "vhost-vsock-device" on ARM.
	VsockDevice string
}

// DefaultMachineType returns the machine type, defaulting to "virt".
func (p *QEMUPlatformConfig) DefaultMachineType() string {
	if p.MachineType != "" {
		return p.MachineType
	}
	return "virt"
}
