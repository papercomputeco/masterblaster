//go:build !linux

package vsock

import (
	"fmt"
	"net"
	"time"
)

// VsockTransport is a compile-time stub on non-Linux platforms. AF_VSOCK is
// Linux-only; macOS code paths use FuncTransport wrapping a
// vz.VirtioSocketDevice instead (see pkg/vm/applevirt.go).
type VsockTransport struct {
	CID  uint32
	Port uint32
}

// Dial always fails on non-Linux targets.
func (t *VsockTransport) Dial(_ time.Duration) (net.Conn, error) {
	return nil, fmt.Errorf("vsock: AF_VSOCK transport is Linux-only")
}

// String returns a human-readable address for logs and error messages.
func (t *VsockTransport) String() string {
	return fmt.Sprintf("vsock:CID=%d,port=%d (unsupported on this OS)", t.CID, t.Port)
}
