//go:build !linux

package vm

import "github.com/papercomputeco/masterblaster/pkg/vsock"

// newVsockTransport is not available on non-Linux platforms.
// The caller should never reach this path because ControlPlaneMode
// is only set to "vsock" on Linux.
func newVsockTransport() vsock.Transport {
	panic("vsock transport is only available on Linux/KVM")
}
