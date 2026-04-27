//go:build linux

package vm

import (
	"os"
	"strings"
)

// HostHasVsock reports whether this host can participate as the vsock
// endpoint for a QEMU guest using vhost-vsock-pci. Requires:
//
//  1. /dev/vhost-vsock exists and is a character device (vhost_vsock module
//     loaded + udev has populated /dev).
//  2. The process is not running under WSL2 (Microsoft's WSL kernel doesn't
//     expose nested vsock cleanly; TCP hostfwd is the safer path there).
//
// When this returns false, callers should fall back to TCP transport even
// when the platform config requests vsock.
func HostHasVsock() bool {
	if runningUnderWSL() {
		return false
	}
	info, err := os.Stat("/dev/vhost-vsock")
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// runningUnderWSL reports whether /proc/version carries the Microsoft WSL
// marker. Kernel strings on WSL2 look like:
//
//	Linux version 5.15.167.4-microsoft-standard-WSL2 ...
//
// so both "microsoft" and "wsl" substrings are reliable signals.
func runningUnderWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	v := strings.ToLower(string(data))
	return strings.Contains(v, "microsoft") || strings.Contains(v, "wsl")
}
