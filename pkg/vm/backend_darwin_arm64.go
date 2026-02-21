//go:build darwin && arm64

package vm

import "os"

// NewPlatformBackend returns the VM backend for darwin/arm64.
//
// The default backend is QEMU with HVF acceleration. QEMU supports both
// qcow2 and raw disk images and does not require binary codesigning beyond
// normal macOS requirements.
//
// Set MB_BACKEND=applevirt (or pass --backend=applevirt to mb serve) to use
// the Apple Virtualization.framework backend instead. The Apple Virt backend:
//
//   - Requires macOS 13 (Ventura) or newer.
//   - Requires raw disk images (nixos.img); qcow2 is not supported.
//   - Requires the mb binary to be codesigned with the
//     com.apple.security.virtualization entitlement (run `make build-local`
//     which calls `make sign` automatically).
//   - Uses native virtio-socket for the stereosd control plane, eliminating
//     the TCP-over-SLIRP forwarding used by the QEMU backend.
//   - Uses virtiofs for shared directories instead of virtio-9p; the StereOS
//     guest must have CONFIG_VIRTIO_FS enabled.
//   - Provides faster boot times and lower overhead than QEMU/HVF.
func NewPlatformBackend(baseDir string) (Backend, error) {
	if os.Getenv("MB_BACKEND") == "applevirt" {
		return NewAppleVirtBackend(baseDir), nil
	}

	// Default: QEMU with HVF acceleration.
	//
	// vsock devices (vhost-vsock-*) are not available in QEMU on macOS/HVF,
	// so the stereosd control plane is forwarded via TCP through QEMU's
	// user-mode networking.
	platform := &QEMUPlatformConfig{
		Accelerator: "hvf",
		Binary:      "qemu-system-aarch64",
		MachineType: "virt",
		EFISearchPaths: []string{
			"{qemu_prefix}/share/qemu/edk2-aarch64-code.fd",
			"/opt/homebrew/share/qemu/edk2-aarch64-code.fd",
		},
		ControlPlaneMode: "tcp",
		DirectKernelBoot: true,
		// DiskAIO and DiskCache left empty: io_uring is Linux-only,
		// macOS uses QEMU's default posix_aio/threads backend.
	}
	return NewQEMUBackend(baseDir, platform), nil
}
