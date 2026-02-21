package vm

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

// allocatePort finds a free TCP port by binding to :0 and immediately
// releasing it. The port is returned for use by a hypervisor or proxy.
// There is a small TOCTOU window between release and the caller binding,
// but in practice this is not a problem on loopback.
func allocatePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("finding free port: %w", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port, nil
}

// processAlive reports whether a process with the given PID is still running.
// It uses signal 0, which checks process existence without sending a signal.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
