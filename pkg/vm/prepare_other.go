//go:build !(darwin && arm64)

package vm

import "fmt"

// DefaultBackend returns "qemu" on non-Apple-Silicon platforms.
func DefaultBackend() string {
	return "qemu"
}

// PrepareAppleVirtDisk is not available on this platform.
func PrepareAppleVirtDisk(_ string, _ *Instance) error {
	return fmt.Errorf("apple Virtualization.framework is only available on macOS/Apple Silicon")
}
