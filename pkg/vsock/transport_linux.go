//go:build linux

package vsock

import (
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// VsockTransport connects to stereosd via AF_VSOCK on Linux/KVM.
// This provides an isolated control plane that works independently of
// guest networking, even with network.mode = "none". It requires a
// vhost-vsock-pci device attached to the QEMU guest.
type VsockTransport struct {
	// CID is the guest context identifier. QEMU uses guest-cid=3 by default.
	CID uint32

	// Port is the vsock port that stereosd listens on inside the guest.
	Port uint32
}

// Dial connects to stereosd via AF_VSOCK.
func (t *VsockTransport) Dial(timeout time.Duration) (net.Conn, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("creating vsock socket: %w", err)
	}

	// Set a send timeout so that Connect does not block indefinitely.
	// AF_VSOCK connect honors SO_SNDTIMEO on Linux.
	tv := unix.NsecToTimeval(timeout.Nanoseconds())
	if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_SNDTIMEO, &tv); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("setting vsock connect timeout: %w", err)
	}

	sa := &unix.SockaddrVM{
		CID:  t.CID,
		Port: t.Port,
	}
	if err := unix.Connect(fd, sa); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("connecting to vsock CID %d port %d: %w", t.CID, t.Port, err)
	}

	// Wrap the raw fd in a net.Conn via os.File. FileConn duplicates the
	// fd, so we must close the os.File to avoid leaking the original.
	file := os.NewFile(uintptr(fd), fmt.Sprintf("vsock:%d:%d", t.CID, t.Port))
	conn, err := net.FileConn(file)
	_ = file.Close()
	if err != nil {
		return nil, fmt.Errorf("wrapping vsock fd as net.Conn: %w", err)
	}

	return conn, nil
}

// String returns a human-readable description for logs.
func (t *VsockTransport) String() string {
	return fmt.Sprintf("vsock:%d:%d", t.CID, t.Port)
}
