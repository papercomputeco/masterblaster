//go:build !linux

package vm

// HostHasVsock is always false off Linux — AF_VSOCK sockets are Linux-only
// on the host side, and macOS reaches stereosd via the applevirt backend's
// vz.VirtioSocketDevice instead.
func HostHasVsock() bool { return false }
