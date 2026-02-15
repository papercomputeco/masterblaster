//go:build darwin && arm64

package vm

// NewPlatformBackend returns the appropriate VM backend for the current
// platform. On darwin/arm64, this returns the QEMU backend for now.
// In the future, this will return an Apple Virtualization.framework backend.
func NewPlatformBackend(baseDir string) (Backend, error) {
	// TODO: Implement Apple Virtualization.framework backend
	// return NewAppleVirtBackend(baseDir)
	return NewQEMUBackend(baseDir), nil
}
