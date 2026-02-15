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
//     (hostfwd). Used on macOS/HVF where native vsock is unavailable.
//   - VsockTransport (future): connects via AF_VSOCK on Linux/KVM.
//     Requires golang.org/x/sys/unix for socket creation.
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

// NOTE: VsockTransport for Linux/KVM (AF_VSOCK CID:3 port 1024) will be
// implemented when Linux backend support is built. It requires:
//
//   import "golang.org/x/sys/unix"
//
//   type VsockTransport struct {
//       CID  uint32
//       Port uint32
//   }
//
//   func (t *VsockTransport) Dial(timeout time.Duration) (net.Conn, error) {
//       fd, _ := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM, 0)
//       addr := &unix.SockaddrVM{CID: t.CID, Port: t.Port}
//       unix.Connect(fd, addr)
//       return net.FileConn(os.NewFile(uintptr(fd), "vsock"))
//   }
