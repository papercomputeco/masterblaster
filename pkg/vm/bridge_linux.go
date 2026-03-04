//go:build linux

package vm

import (
	"os"
	"os/exec"
	"path/filepath"
)

// findBridgeHelper locates the QEMU bridge helper binary by searching
// common installation paths. The helper creates tap devices and attaches
// them to a bridge interface, enabling bridged networking without root
// privileges on the QEMU process itself.
//
// The helper binary must have setuid root or CAP_NET_ADMIN capability,
// and the target bridge must be listed in /etc/qemu/bridge.conf.
func findBridgeHelper(qemuBinary string) string {
	// Try the QEMU installation prefix first. This works for Nix,
	// Homebrew-on-Linux, and custom QEMU installs.
	if qemuBin, err := exec.LookPath(qemuBinary); err == nil {
		resolved, err := filepath.EvalSymlinks(qemuBin)
		if err == nil {
			prefix := filepath.Dir(filepath.Dir(resolved))
			candidate := filepath.Join(prefix, "libexec", "qemu-bridge-helper")
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}

	// Common distro paths.
	candidates := []string{
		"/usr/lib/qemu/qemu-bridge-helper",   // Ubuntu, Debian, Arch
		"/usr/libexec/qemu-bridge-helper",    // Fedora, RHEL
		"/usr/lib64/qemu/qemu-bridge-helper", // Some RHEL/CentOS variants
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}

	return ""
}
