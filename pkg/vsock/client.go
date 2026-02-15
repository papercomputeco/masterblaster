package vsock

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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
// a network connection. On Linux with real vsock, this connects to
// AF_VSOCK CID:3 port 1024. On macOS for development, this connects
// via TCP to a forwarded port.
type Client struct {
	conn    net.Conn
	scanner *bufio.Scanner
	enc     *json.Encoder
	mu      sync.Mutex
}

// Dial connects to stereosd. The address format depends on the transport:
// for TCP (development): "127.0.0.1:1024"
// for vsock (production): use DialVsock instead.
func Dial(address string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, fmt.Errorf("connecting to stereosd at %s: %w", address, err)
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB max message

	return &Client{
		conn:    conn,
		scanner: scanner,
		enc:     json.NewEncoder(conn),
	}, nil
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
		c.conn.SetDeadline(deadline)
	} else {
		c.conn.SetDeadline(time.Now().Add(DefaultTimeout))
	}
	defer c.conn.SetDeadline(time.Time{})

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

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			health, err := c.GetHealth(ctx)
			if err != nil {
				continue // Not ready yet
			}
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
