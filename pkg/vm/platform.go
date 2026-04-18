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

	// DirectKernelBoot enables -kernel/-initrd/-append boot, bypassing
	// EFI firmware and GRUB. Requires kernel artifacts (bzImage, initrd,
	// cmdline) in the mixtape directory. When enabled and kernel artifacts
	// are available, EFI pflash drives and EFI vars initialization are
	// skipped entirely. Falls back to EFI boot if artifacts are missing.
	DirectKernelBoot bool

	// DiskAIO is the async I/O backend for QEMU disk drives.
	// Set to "io_uring" on Linux 5.1+ for best performance, or leave
	// empty to use QEMU's default (threads). Not supported on macOS.
	DiskAIO string

	// DiskCache is the cache mode for QEMU disk drives.
	// Set to "none" (O_DIRECT) when using io_uring for best performance,
	// or leave empty to use QEMU's default (writeback).
	DiskCache string

	// MachineProps are extra comma-separated properties appended to the
	// -machine argument. For example, "highmem=on" is valid for the ARM
	// "virt" machine type but not for x86_64 "q35".
	MachineProps string

	// BridgeHelper is the resolved path to the QEMU bridge helper binary
	// (qemu-bridge-helper). Only set on Linux where bridged networking
	// uses tap devices via the helper. Empty on macOS where vmnet-shared
	// is used instead.
	BridgeHelper string
}

// DefaultMachineType returns the machine type, defaulting to "virt".
func (p *QEMUPlatformConfig) DefaultMachineType() string {
	if p.MachineType != "" {
		return p.MachineType
	}
	return "virt"
}
