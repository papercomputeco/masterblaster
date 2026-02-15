//go:build linux

package vm

// NewPlatformBackend returns the appropriate VM backend for Linux.
// Currently returns the QEMU backend. In the future, this could
// return a KVM/libvirt backend for better performance.
func NewPlatformBackend(baseDir string) (Backend, error) {
	// TODO: Implement KVM/libvirt backend
	return NewQEMUBackend(baseDir), nil
}
