// Package client provides a thin wrapper for CLI commands to communicate
// with the Masterblaster daemon over the unix domain socket. It also
// provides EnsureDaemon() to auto-start the daemon when needed.
package client

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/papercomputeco/masterblaster/pkg/daemon"
	"github.com/papercomputeco/masterblaster/pkg/ui"
)

// Client communicates with the Masterblaster daemon over the unix socket.
type Client struct {
	baseDir    string
	socketPath string
}

// New creates a new daemon client for the given base directory.
func New(baseDir string) *Client {
	return &Client{
		baseDir:    baseDir,
		socketPath: daemon.SocketPath(baseDir),
	}
}

// call sends an RPC request and returns the response.
func (c *Client) call(req *daemon.Request) (*daemon.Response, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connecting to daemon at %s: %w\n\nIs the daemon running? Start it with: mb serve", c.socketPath, err)
	}
	defer func() { _ = conn.Close() }()

	// Set a generous timeout for operations that may take a while
	_ = conn.SetDeadline(time.Now().Add(5 * time.Minute))

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	if err := enc.Encode(req); err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	var resp daemon.Response
	if err := dec.Decode(&resp); err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if !resp.OK && resp.Error != "" {
		return &resp, fmt.Errorf("%s", resp.Error)
	}

	return &resp, nil
}

// Ping checks if the daemon is alive.
func (c *Client) Ping() error {
	_, err := c.call(&daemon.Request{Method: daemon.MethodPing})
	return err
}

// Up starts a new sandbox with the given config file.
func (c *Client) Up(name, configPath string) (*daemon.Response, error) {
	return c.call(&daemon.Request{
		Method:     daemon.MethodUp,
		Name:       name,
		ConfigPath: configPath,
	})
}

// Down gracefully stops a sandbox.
func (c *Client) Down(name string, force bool) (*daemon.Response, error) {
	return c.call(&daemon.Request{
		Method: daemon.MethodDown,
		Name:   name,
		Force:  force,
	})
}

// Status returns the state of a sandbox (or all if name is empty and all is true).
func (c *Client) Status(name string, all bool) (*daemon.Response, error) {
	return c.call(&daemon.Request{
		Method: daemon.MethodStatus,
		Name:   name,
		All:    all,
	})
}

// Destroy removes a sandbox and all its resources.
func (c *Client) Destroy(name string) (*daemon.Response, error) {
	return c.call(&daemon.Request{
		Method: daemon.MethodDestroy,
		Name:   name,
	})
}

// List returns all known sandboxes.
func (c *Client) List() (*daemon.Response, error) {
	return c.call(&daemon.Request{Method: daemon.MethodList})
}

// EnsureDaemon starts the daemon if it's not already running. It finds
// the current mb binary, forks it with "serve" in a new session (setsid),
// and polls the socket until the daemon responds.
func EnsureDaemon(baseDir string) error {
	if daemon.IsRunning(baseDir) {
		return nil
	}

	ui.Status("Starting daemon...")

	mbBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding mb binary: %w", err)
	}

	cmd := exec.Command(mbBin, "serve")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}

	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("releasing daemon process: %w", err)
	}

	for i := 0; i < 30; i++ {
		time.Sleep(200 * time.Millisecond)
		if daemon.IsRunning(baseDir) {
			return nil
		}
	}

	return fmt.Errorf("daemon did not start within 6 seconds")
}
