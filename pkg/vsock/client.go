package vsock

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

const (
	// VsockPort is the well-known port that stereosd listens on.
	VsockPort = 1024

	// VsockGuestCID is the guest CID for vsock connections.
	VsockGuestCID = 3

	// DefaultTimeout is the default timeout for vsock operations.
	DefaultTimeout = 10 * time.Second
)

// Client communicates with stereosd running inside a StereOS guest over
// a network connection. The underlying transport is abstracted via the
// Transport interface, allowing TCP (macOS/HVF), AF_VSOCK (Linux/KVM),
// or other transports to be used interchangeably.
type Client struct {
	conn    net.Conn
	scanner *bufio.Scanner
	enc     *json.Encoder
	mu      sync.Mutex
}

// Connect establishes a connection to stereosd using the given transport.
// The transport determines the underlying mechanism (TCP, AF_VSOCK, etc.).
func Connect(transport Transport, timeout time.Duration) (*Client, error) {
	conn, err := transport.Dial(timeout)
	if err != nil {
		return nil, err
	}
	return newClient(conn), nil
}

// Dial connects to stereosd via TCP at the given address.
// This is a convenience wrapper around Connect with a TCPTransport.
// Deprecated: prefer Connect with an explicit Transport.
func Dial(address string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, fmt.Errorf("connecting to stereosd at %s: %w", address, err)
	}
	return newClient(conn), nil
}

// newClient creates a Client from an established connection.
func newClient(conn net.Conn) *Client {
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB max message

	return &Client{
		conn:    conn,
		scanner: scanner,
		enc:     json.NewEncoder(conn),
	}
}

// Close closes the connection to stereosd.
func (c *Client) Close() error {
	return c.conn.Close()
}

// send sends a message and waits for a response.
func (c *Client) send(ctx context.Context, env *Envelope) (*Envelope, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Set deadline from context
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetDeadline(deadline)
	} else {
		_ = c.conn.SetDeadline(time.Now().Add(DefaultTimeout))
	}
	defer func() { _ = c.conn.SetDeadline(time.Time{}) }()

	// Send the message as a single JSON line
	if err := c.enc.Encode(env); err != nil {
		return nil, fmt.Errorf("sending %s message: %w", env.Type, err)
	}

	// Read the response
	if !c.scanner.Scan() {
		if err := c.scanner.Err(); err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}
		return nil, fmt.Errorf("connection closed while waiting for response")
	}

	var resp Envelope
	if err := json.Unmarshal(c.scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &resp, nil
}

// Ping sends a ping and waits for a pong response.
func (c *Client) Ping(ctx context.Context) error {
	env, err := NewEnvelope(MsgPing, nil)
	if err != nil {
		return err
	}

	resp, err := c.send(ctx, env)
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	if resp.Type != MsgPong {
		return fmt.Errorf("ping: expected pong, got %s", resp.Type)
	}
	return nil
}

// SetConfig sends the jcard.toml configuration to the guest. stereosd writes
// it to /etc/stereos/jcard.toml where agentd picks it up via its reconciliation loop.
func (c *Client) SetConfig(ctx context.Context, content string) error {
	env, err := NewEnvelope(MsgSetConfig, &ConfigPayload{
		Content: content,
	})
	if err != nil {
		return err
	}

	resp, err := c.send(ctx, env)
	if err != nil {
		return fmt.Errorf("set config: %w", err)
	}

	return checkAck(resp, MsgSetConfig)
}

// InjectSecret writes a secret to the guest's tmpfs secret store.
func (c *Client) InjectSecret(ctx context.Context, name, value string) error {
	env, err := NewEnvelope(MsgInjectSecret, &SecretPayload{
		Name:  name,
		Value: value,
		Mode:  0600,
	})
	if err != nil {
		return err
	}

	resp, err := c.send(ctx, env)
	if err != nil {
		return fmt.Errorf("inject secret %q: %w", name, err)
	}

	return checkAck(resp, MsgInjectSecret)
}

// InjectSSHKey sends a public key to stereosd for installation into the
// specified user's authorized_keys file. stereosd creates ~/.ssh/ if needed
// and writes the key with appropriate ownership and permissions.
func (c *Client) InjectSSHKey(ctx context.Context, user, publicKey string) error {
	env, err := NewEnvelope(MsgInjectSSHKey, &SSHKeyPayload{
		User:      user,
		PublicKey: publicKey,
	})
	if err != nil {
		return err
	}

	resp, err := c.send(ctx, env)
	if err != nil {
		return fmt.Errorf("inject SSH key for %q: %w", user, err)
	}

	return checkAck(resp, MsgInjectSSHKey)
}

// Mount requests stereosd to mount a shared directory.
func (c *Client) Mount(ctx context.Context, tag, guestPath, fsType string, readOnly bool) error {
	env, err := NewEnvelope(MsgMount, &MountPayload{
		Tag:       tag,
		GuestPath: guestPath,
		FSType:    fsType,
		ReadOnly:  readOnly,
	})
	if err != nil {
		return err
	}

	resp, err := c.send(ctx, env)
	if err != nil {
		return fmt.Errorf("mount %q at %q: %w", tag, guestPath, err)
	}

	return checkAck(resp, MsgMount)
}

// Shutdown requests a graceful shutdown of the StereOS instance.
func (c *Client) Shutdown(ctx context.Context, reason string) error {
	env, err := NewEnvelope(MsgShutdown, &ShutdownPayload{
		Reason: reason,
	})
	if err != nil {
		return err
	}

	resp, err := c.send(ctx, env)
	if err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	return checkAck(resp, MsgShutdown)
}

// GetHealth requests the current health status from stereosd.
func (c *Client) GetHealth(ctx context.Context) (*HealthPayload, error) {
	env, err := NewEnvelope(MsgGetHealth, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.send(ctx, env)
	if err != nil {
		return nil, fmt.Errorf("get health: %w", err)
	}

	if resp.Type != MsgHealth {
		return nil, fmt.Errorf("get health: expected health, got %s", resp.Type)
	}

	var health HealthPayload
	if err := resp.DecodePayload(&health); err != nil {
		return nil, fmt.Errorf("decoding health: %w", err)
	}

	return &health, nil
}

// WaitForReady polls stereosd until it reports ready or healthy state,
// or the context is cancelled.
func (c *Client) WaitForReady(ctx context.Context, pollInterval time.Duration) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	attempts := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			attempts++
			health, err := c.GetHealth(ctx)
			if err != nil {
				if attempts <= 5 || attempts%20 == 0 {
					log.Printf("[vsock] WaitForReady: attempt %d: GetHealth error: %v", attempts, err)
				}
				continue // Not ready yet
			}
			log.Printf("[vsock] WaitForReady: attempt %d: state=%q uptime=%ds", attempts, health.State, health.Uptime)
			switch health.State {
			case StateReady, StateHealthy:
				return nil
			case StateShutdown:
				return fmt.Errorf("guest is shutting down")
			}
			// Still booting or degraded, keep polling
		}
	}
}

// checkAck verifies that a response is a successful ack for the given message type.
func checkAck(resp *Envelope, expectedReplyTo MessageType) error {
	if resp.Type != MsgAck {
		return fmt.Errorf("expected ack, got %s", resp.Type)
	}

	var ack AckPayload
	if err := resp.DecodePayload(&ack); err != nil {
		return fmt.Errorf("decoding ack: %w", err)
	}

	if ack.ReplyTo != expectedReplyTo {
		return fmt.Errorf("ack for %s, expected %s", ack.ReplyTo, expectedReplyTo)
	}

	if !ack.OK {
		return fmt.Errorf("%s failed: %s", expectedReplyTo, ack.Error)
	}

	return nil
}
