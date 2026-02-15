// Package ssh provides SSH connectivity to StereOS sandbox VMs.
package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// ExecSSH replaces the current process with the ssh binary, providing a
// clean interactive experience. Signal handling, terminal resizing, SSH
// agent forwarding, and ~. escape sequences all work correctly because
// the user talks directly to OpenSSH.
func ExecSSH(user, host string, port int) error {
	sshBin, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh binary not found: %w", err)
	}

	args := []string{
		"ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-t", // Force PTY allocation
		"-p", fmt.Sprintf("%d", port),
	}

	args = append(args, fmt.Sprintf("%s@%s", user, host))

	// Replace process -- never returns on success
	return syscall.Exec(sshBin, args, os.Environ())
}
