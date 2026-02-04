package ssh

import (
	"fmt"
	"net"
	"time"
)

// WaitForSSH polls the given address (host:port) until a TCP connection
// succeeds or the timeout (in seconds) expires. This indicates that sshd
// in the guest is accepting connections.
func WaitForSSH(address string, timeoutSeconds int) error {
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	interval := 2 * time.Second

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 3*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(interval)
	}

	return fmt.Errorf("SSH not ready at %s after %d seconds", address, timeoutSeconds)
}
