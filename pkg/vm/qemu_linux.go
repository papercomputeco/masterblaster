//go:build linux

package vm

import "github.com/papercomputeco/masterblaster/pkg/vsock"

// newVsockTransport returns a native AF_VSOCK transport for Linux/KVM.
// This connects directly to the guest via vhost-vsock-pci, bypassing
// TCP/SLIRP entirely.
func newVsockTransport() vsock.Transport {
	return &vsock.VsockTransport{
		CID:  vsock.VsockGuestCID,
		Port: uint32(vsock.VsockPort),
	}
}
