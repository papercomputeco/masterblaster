package vm

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// QMPClient communicates with a running QEMU instance over its QMP
// (QEMU Machine Protocol) unix domain socket.
type QMPClient struct {
	conn net.Conn
	dec  *json.Decoder
	enc  *json.Encoder
}

// DialQMP connects to a QMP unix socket, reads the greeting, and
// negotiates capabilities to enter command mode.
func DialQMP(socketPath string) (*QMPClient, error) {
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connecting to QMP socket: %w", err)
	}

	c := &QMPClient{
		conn: conn,
		dec:  json.NewDecoder(conn),
		enc:  json.NewEncoder(conn),
	}

	// Read server greeting
	var greeting map[string]interface{}
	if err := c.dec.Decode(&greeting); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("reading QMP greeting: %w", err)
	}

	// Enter command mode
	if err := c.execute("qmp_capabilities", nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("QMP capabilities negotiation: %w", err)
	}

	return c, nil
}

// execute sends a QMP command and waits for the response.
func (c *QMPClient) execute(command string, args interface{}) error {
	req := map[string]interface{}{"execute": command}
	if args != nil {
		req["arguments"] = args
	}

	if err := c.enc.Encode(req); err != nil {
		return fmt.Errorf("sending QMP command %q: %w", command, err)
	}

	// Read responses until we get a return or error (skip async events)
	for {
		var resp map[string]interface{}
		if err := c.dec.Decode(&resp); err != nil {
			return fmt.Errorf("reading QMP response: %w", err)
		}

		if errObj, ok := resp["error"]; ok {
			return fmt.Errorf("QMP error: %v", errObj)
		}

		if _, ok := resp["return"]; ok {
			return nil
		}

		// Skip event messages ({"event": ...}) and continue waiting
	}
}

// Shutdown sends ACPI power-off (clean shutdown, systemd handles it).
func (c *QMPClient) Shutdown() error {
	return c.execute("system_powerdown", nil)
}

// Quit force-kills the VM process.
func (c *QMPClient) Quit() error {
	return c.execute("quit", nil)
}

// QueryStatus returns the VM run state (e.g. "running", "paused", "shutdown").
func (c *QMPClient) QueryStatus() (string, error) {
	req := map[string]interface{}{"execute": "query-status"}
	if err := c.enc.Encode(req); err != nil {
		return "", fmt.Errorf("sending query-status: %w", err)
	}

	for {
		var resp map[string]interface{}
		if err := c.dec.Decode(&resp); err != nil {
			return "", fmt.Errorf("reading query-status response: %w", err)
		}

		if errObj, ok := resp["error"]; ok {
			return "", fmt.Errorf("QMP error: %v", errObj)
		}

		if ret, ok := resp["return"]; ok {
			retMap, ok := ret.(map[string]interface{})
			if !ok {
				return "", fmt.Errorf("unexpected query-status return type")
			}
			status, ok := retMap["status"].(string)
			if !ok {
				return "", fmt.Errorf("status field not a string")
			}
			return status, nil
		}
		// Skip event messages
	}
}

// Close closes the QMP connection.
func (c *QMPClient) Close() error {
	return c.conn.Close()
}
