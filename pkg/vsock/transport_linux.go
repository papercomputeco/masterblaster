//go:build linux

package vsock

import (
	"fmt"
	"net"
	"time"

	mdvsock "github.com/mdlayher/vsock"
)

// VsockTransport dials stereosd via AF_VSOCK directly on Linux. Requires the
// host kernel's vhost_vsock module to be loaded and a corresponding
// vhost-vsock-pci device in the guest (QEMU `-device
// vhost-vsock-pci,guest-cid=<CID>`). The (CID, Port) pair uniquely
// identifies the guest endpoint on the host.
//
// This is the preferred transport on Linux: no SLIRP/TCP hostfwd in the
// data path, and the control plane stays reachable even when the guest's
// user network is turned off (jcard `network.mode = "none"`).
//
// Implementation note: Go's `net.FileConn` rejects AF_VSOCK with "protocol
// not supported" because the stdlib's socket-family detection doesn't know
// about AF_VSOCK. We delegate to github.com/mdlayher/vsock, which wraps
// the AF_VSOCK fd in a proper net.Conn with Go's runtime poller.
type VsockTransport struct {
	CID  uint32 // guest CID; must match QEMU's -device ...,guest-cid=<N>
	Port uint32 // guest vsock port (stereosd listens on vsock.VsockPort = 1024)
}

// Dial connects to stereosd on (CID, Port). mdlayher/vsock.Dial is blocking
// with no timeout parameter; in practice the kernel either completes the
// connect or returns ECONNREFUSED within microseconds (there's no network
// round-trip on vsock). We race against time.After as a safety net for
// pathological cases.
func (t *VsockTransport) Dial(timeout time.Duration) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		c, err := mdvsock.Dial(t.CID, t.Port, nil)
		ch <- result{c, err}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("vsock: dial CID:%d port:%d: %w", t.CID, t.Port, r.err)
		}
		return r.conn, nil
	case <-time.After(timeout):
		// Drain the goroutine so we don't leak if Dial completes later.
		go func() {
			r := <-ch
			if r.err == nil && r.conn != nil {
				_ = r.conn.Close()
			}
		}()
		return nil, fmt.Errorf("vsock: connect timeout after %v to CID:%d port:%d", timeout, t.CID, t.Port)
	}
}

// String returns a human-readable address for logs and error messages.
func (t *VsockTransport) String() string {
	return fmt.Sprintf("vsock:CID=%d,port=%d", t.CID, t.Port)
}
