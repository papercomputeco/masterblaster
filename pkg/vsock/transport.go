package vsock

import (
	"fmt"
	"net"
	"time"
)

// Transport abstracts the host-side connection to stereosd running inside
// a StereOS guest VM. Different backends use different transports:
//
//   - TCPTransport: connects via TCP through QEMU user-mode networking
//     (hostfwd). Fallback for hosts without vhost-vsock (macOS/HVF, WSL2,
//     and Linux kernels without vhost_vsock loaded).
//   - VsockTransport: connects via AF_VSOCK on Linux, against a QEMU
//     vhost-vsock-pci device (transport_linux.go). Preferred when
//     available — no SLIRP overhead and control plane survives
//     `network.mode = "none"`.
//   - FuncTransport: wraps a caller-supplied dial function. Used by the
//     Apple Virtualization.framework backend to reach stereosd through
//     vz.VirtioSocketDevice without importing the darwin-only vz package
//     into pkg/vsock.
//   - VirtioSerialTransport (future): connects via a chardev unix socket
//     backed by virtio-serial. Works on macOS/HVF without guest networking.
type Transport interface {
	// Dial establishes a connection to stereosd with the given timeout.
	Dial(timeout time.Duration) (net.Conn, error)

	// String returns a human-readable description for logs and error messages.
	String() string
}

// TCPTransport connects to stereosd via TCP. This is used when native vsock
// is not available (macOS/HVF). QEMU's user-mode networking forwards a host
// TCP port to the guest port where stereosd listens.
type TCPTransport struct {
	Host string
	Port int
}

// Dial connects to stereosd via TCP.
func (t *TCPTransport) Dial(timeout time.Duration) (net.Conn, error) {
	addr := net.JoinHostPort(t.Host, fmt.Sprintf("%d", t.Port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("connecting to stereosd at %s: %w", addr, err)
	}
	return conn, nil
}

// String returns the TCP address.
func (t *TCPTransport) String() string {
	return fmt.Sprintf("tcp:%s:%d", t.Host, t.Port)
}

// FuncTransport is a Transport backed by an arbitrary dial function.
// It is used by the Apple Virtualization.framework backend to inject a
// vz.VirtioSocketDevice connection without creating an import dependency
// between pkg/vsock and the darwin-only vz package.
type FuncTransport struct {
	// DialFn is called by Dial to establish the connection.
	DialFn func(timeout time.Duration) (net.Conn, error)
	// Label is returned by String() for logging and error messages.
	Label string
}

// Dial calls the underlying dial function.
func (f *FuncTransport) Dial(timeout time.Duration) (net.Conn, error) {
	return f.DialFn(timeout)
}

// String returns the human-readable label.
func (f *FuncTransport) String() string { return f.Label }
