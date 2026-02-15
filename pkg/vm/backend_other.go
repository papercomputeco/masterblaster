//go:build !darwin && !linux

package vm

import "fmt"

// NewPlatformBackend returns an error on unsupported platforms.
func NewPlatformBackend(baseDir string) (Backend, error) {
	return nil, fmt.Errorf("no VM backend available for this platform; QEMU backend requires darwin or linux")
}
