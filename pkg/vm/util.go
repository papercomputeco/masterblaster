package vm

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
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

// allocateVsockCID picks a guest CID for vhost-vsock by scanning existing VM
// state files under baseDir and returning the lowest unused CID >= 3.
//
// CIDs 0-2 are reserved (VMADDR_CID_HYPERVISOR, _LOCAL, _HOST). vhost-vsock
// CIDs are host-global: two concurrent VMs must not share one, or QEMU will
// refuse to start the second. The scan is best-effort (races with a
// concurrent bootVM are possible); QEMU's own collision detection is the
// authoritative enforcement.
func allocateVsockCID(baseDir string) (uint32, error) {
	used := map[uint32]bool{}

	entries, err := os.ReadDir(VMsDir(baseDir))
	if err != nil && !os.IsNotExist(err) {
		return 0, fmt.Errorf("reading VMs directory: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		state, err := loadState(filepath.Join(VMsDir(baseDir), e.Name()))
		if err != nil {
			continue
		}
		if state.VsockCID > 0 {
			used[state.VsockCID] = true
		}
	}

	// Pick the lowest free CID. We start at 3 (first non-reserved) and cap
	// at 2^31 defensively — in practice we run out of RAM long before CIDs.
	for cid := uint32(3); cid < 1<<31; cid++ {
		if !used[cid] {
			return cid, nil
		}
	}
	return 0, fmt.Errorf("no vsock CID available (all in use under %s)", baseDir)
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
