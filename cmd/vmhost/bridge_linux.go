//go:build linux

package vmhostcmder

import (
	"os"
	"os/exec"
	"path/filepath"
)

// findBridgeHelper locates the QEMU bridge helper binary by searching
// common installation paths. See pkg/vm/bridge_linux.go for details.
func findBridgeHelper(qemuBinary string) string {
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

	candidates := []string{
		"/usr/lib/qemu/qemu-bridge-helper",
		"/usr/libexec/qemu-bridge-helper",
		"/usr/lib64/qemu/qemu-bridge-helper",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}

	return ""
}
