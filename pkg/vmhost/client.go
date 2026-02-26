package vmhost

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Client communicates with a vmhost process over its control socket.
type Client struct {
	socketPath string
}

// NewClient creates a new vmhost client for the given socket path.
func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

// call sends a request to the vmhost and returns the response.
func (c *Client) call(req *Request) (*Response, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connecting to vmhost socket %s: %w", c.socketPath, err)
	}
	defer func() { _ = conn.Close() }()

	// Set a deadline for the entire exchange
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return nil, fmt.Errorf("setting deadline: %w", err)
	}

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	if err := enc.Encode(req); err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	var resp Response
	if err := dec.Decode(&resp); err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if !resp.OK && resp.Error != "" {
		return &resp, fmt.Errorf("vmhost error: %s", resp.Error)
	}

	return &resp, nil
}

// Status queries the VM state from the vmhost process.
func (c *Client) Status() (*Response, error) {
	return c.call(&Request{Method: MethodStatus})
}

// Stop sends a graceful shutdown request with the given timeout.
func (c *Client) Stop(timeoutSeconds int) (*Response, error) {
	return c.call(&Request{
		Method:         MethodStop,
		TimeoutSeconds: timeoutSeconds,
	})
}

// ForceStop sends an immediate termination request.
func (c *Client) ForceStop() (*Response, error) {
	return c.call(&Request{Method: MethodForceStop})
}

// Info queries detailed VM information from the vmhost process.
func (c *Client) Info() (*Response, error) {
	return c.call(&Request{Method: MethodInfo})
}

// IsAlive checks if the vmhost process is reachable by sending a status
// request. Returns true if the vmhost responds, false otherwise.
func (c *Client) IsAlive() bool {
	resp, err := c.Status()
	if err != nil {
		return false
	}
	return resp.OK
}
