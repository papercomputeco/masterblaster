package vmhost

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

// VMController is the interface that the vmhost server uses to interact
// with the underlying VM. Each backend (QEMU, Apple Virt) implements this
// to provide lifecycle control.
type VMController interface {
	// State returns the current VM lifecycle state as a string
	// ("running", "stopped", "error").
	State() string

	// Stop gracefully shuts down the VM within the given timeout.
	// It should attempt stereosd shutdown, then ACPI, then force kill.
	Stop(ctx context.Context, timeout time.Duration) error

	// ForceStop immediately terminates the VM.
	ForceStop(ctx context.Context) error

	// SSHPort returns the host-side SSH port for this VM.
	SSHPort() int

	// Backend returns the backend name ("qemu" or "applevirt").
	Backend() string

	// Apply sends updated configuration and secrets to stereosd inside the
	// guest. It connects to the guest control plane, sends set_config with
	// the serialized jcard.toml content, and re-injects all secrets.
	Apply(ctx context.Context, configContent string, secrets map[string]string) error

	// Wait blocks until the VM exits. It returns nil on clean exit
	// or an error if the VM crashed.
	Wait() error
}

// Server is the vmhost control socket server. It listens on vmhost.sock
// and dispatches requests to the VMController.
type Server struct {
	socketPath string
	pidPath    string
	controller VMController
	listener   net.Listener
	logger     *log.Logger

	mu       sync.Mutex
	shutdown bool
}

// NewServer creates a new vmhost control server.
func NewServer(socketPath, pidPath string, controller VMController, logger *log.Logger) *Server {
	return &Server{
		socketPath: socketPath,
		pidPath:    pidPath,
		controller: controller,
		logger:     logger,
	}
}

// Run starts the control socket server and blocks until the context is
// cancelled or the VM exits. It writes the PID file on startup and
// cleans up the socket and PID file on exit.
func (s *Server) Run(ctx context.Context) error {
	// Clean up stale socket
	_ = os.Remove(s.socketPath)

	// Write PID file
	if err := os.WriteFile(s.pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return fmt.Errorf("writing vmhost PID file: %w", err)
	}

	// Open unix socket
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		_ = os.Remove(s.pidPath)
		return fmt.Errorf("listening on %s: %w", s.socketPath, err)
	}
	s.listener = listener

	defer s.cleanup()

	s.logger.Printf("vmhost server listening on %s (PID %d)", s.socketPath, os.Getpid())

	// Accept connections in background
	connCh := make(chan net.Conn)
	errCh := make(chan error, 1)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					errCh <- err
					return
				}
			}
			connCh <- conn
		}
	}()

	// Wait for VM exit in background
	vmDone := make(chan error, 1)
	go func() {
		vmDone <- s.controller.Wait()
	}()

	for {
		select {
		case <-ctx.Done():
			s.logger.Println("vmhost shutting down (context cancelled)")
			return nil

		case conn := <-connCh:
			go s.handleConnection(ctx, conn)

		case err := <-errCh:
			return fmt.Errorf("accept error: %w", err)

		case vmErr := <-vmDone:
			if vmErr != nil {
				s.logger.Printf("VM exited with error: %v", vmErr)
			} else {
				s.logger.Println("VM exited cleanly")
			}
			return vmErr
		}
	}
}

// cleanup removes the socket and PID file.
func (s *Server) cleanup() {
	if s.listener != nil {
		_ = s.listener.Close()
	}
	_ = os.Remove(s.socketPath)
	_ = os.Remove(s.pidPath)
	s.logger.Println("vmhost cleaned up socket and PID file")
}

// handleConnection processes a single control socket connection.
func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	var req Request
	if err := dec.Decode(&req); err != nil {
		_ = enc.Encode(Response{Error: fmt.Sprintf("invalid request: %v", err)})
		return
	}

	resp := s.handleRequest(ctx, &req)
	_ = enc.Encode(resp)
}

// handleRequest dispatches an RPC request to the appropriate handler.
func (s *Server) handleRequest(ctx context.Context, req *Request) Response {
	switch req.Method {
	case MethodStatus:
		return s.handleStatus()

	case MethodStop:
		return s.handleStop(ctx, req)

	case MethodForceStop:
		return s.handleForceStop(ctx)

	case MethodInfo:
		return s.handleInfo()

	case MethodApply:
		return s.handleApply(ctx, req)

	default:
		return Response{Error: fmt.Sprintf("unknown method: %s", req.Method)}
	}
}

func (s *Server) handleStatus() Response {
	return Response{
		OK:    true,
		State: s.controller.State(),
	}
}

func (s *Server) handleStop(ctx context.Context, req *Request) Response {
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return Response{OK: true, State: "stopping"}
	}
	s.shutdown = true
	s.mu.Unlock()

	timeout := 30 * time.Second
	if req.TimeoutSeconds > 0 {
		timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}

	s.logger.Printf("stopping VM (timeout: %s)", timeout)

	// Run stop in background so we can return the response immediately.
	// The VM exit will be detected by the Run() loop via controller.Wait().
	go func() {
		if err := s.controller.Stop(ctx, timeout); err != nil {
			s.logger.Printf("stop error: %v", err)
		}
	}()

	return Response{OK: true, State: "stopping"}
}

func (s *Server) handleForceStop(ctx context.Context) Response {
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return Response{OK: true, State: "stopping"}
	}
	s.shutdown = true
	s.mu.Unlock()

	s.logger.Println("force-stopping VM")

	go func() {
		if err := s.controller.ForceStop(ctx); err != nil {
			s.logger.Printf("force-stop error: %v", err)
		}
	}()

	return Response{OK: true, State: "stopping"}
}

func (s *Server) handleInfo() Response {
	return Response{
		OK:      true,
		State:   s.controller.State(),
		SSHPort: s.controller.SSHPort(),
		Backend: s.controller.Backend(),
	}
}

func (s *Server) handleApply(ctx context.Context, req *Request) Response {
	if req.ConfigContent == "" {
		return Response{Error: "config_content is required for apply"}
	}

	s.logger.Printf("applying configuration (%d bytes, %d secrets)", len(req.ConfigContent), len(req.Secrets))

	if err := s.controller.Apply(ctx, req.ConfigContent, req.Secrets); err != nil {
		s.logger.Printf("apply failed: %v", err)
		return Response{Error: fmt.Sprintf("apply failed: %v", err)}
	}

	s.logger.Println("apply completed successfully")
	return Response{OK: true, State: s.controller.State()}
}
