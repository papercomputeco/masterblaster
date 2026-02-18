//go:build darwin && amd64

package vm

// NewPlatformBackend returns the appropriate VM backend for darwin/amd64.
func NewPlatformBackend(baseDir string) (Backend, error) {
	// TODO: darwin amd64 currently not supported.

	panic("darwin amd64 currently not supported")
}
