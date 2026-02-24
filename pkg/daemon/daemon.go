package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/papercomputeco/masterblaster/pkg/config"
	"github.com/papercomputeco/masterblaster/pkg/vm"
)

// Daemon is the long-lived Masterblaster service that manages sandbox VMs.
type Daemon struct {
	mu sync.RWMutex

	// In-memory mapping of running/known VM instances.
	vms map[string]*vm.Instance

	// The hypervisor backend (QEMU, Apple Virt, etc.)
	backend vm.Backend

	// Base directory for all mb state (~/.mb/).
	baseDir string

	// Unix socket listener for CLI communication.
	listener net.Listener

	// Logger for daemon events.
	logger *log.Logger
}

// New creates a new Daemon with the given backend and base directory.
func New(backend vm.Backend, baseDir string) *Daemon {
	return &Daemon{
		vms:     make(map[string]*vm.Instance),
		backend: backend,
		baseDir: baseDir,
		logger:  log.New(os.Stderr, "[mb-daemon] ", log.LstdFlags),
	}
}

// SocketPath returns the path to the daemon's unix socket.
func SocketPath(baseDir string) string {
	return filepath.Join(baseDir, "mb.sock")
}

// PIDFilePath returns the path to the daemon's PID file.
func PIDFilePath(baseDir string) string {
	return filepath.Join(baseDir, "daemon.pid")
}

// Run starts the daemon, listens for CLI requests, and blocks until
// interrupted or the context is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	// Ensure base directories exist
	for _, dir := range []string{
		d.baseDir,
		filepath.Join(d.baseDir, "vms"),
		filepath.Join(d.baseDir, "mixtapes"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	// Clean up stale socket
	sockPath := SocketPath(d.baseDir)
	_ = os.Remove(sockPath)

	// Write PID file
	pidPath := PIDFilePath(d.baseDir)
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}
	defer func() { _ = os.Remove(pidPath) }()

	// Open unix socket
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", sockPath, err)
	}
	d.listener = listener
	defer func() {
		_ = listener.Close()
		_ = os.Remove(sockPath)
	}()

	// Load existing VMs into memory
	d.loadExistingVMs(ctx)

	d.logger.Printf("daemon started, listening on %s (PID %d)", sockPath, os.Getpid())

	// Handle signals for clean shutdown
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Accept connections
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
			go d.handleConnection(ctx, conn)
		}
	}()

	select {
	case <-ctx.Done():
		d.logger.Println("shutting down daemon...")
		return nil
	case err := <-errCh:
		return fmt.Errorf("accept error: %w", err)
	}
}

// loadExistingVMs scans the vms directory and loads known instances.
func (d *Daemon) loadExistingVMs(ctx context.Context) {
	instances, err := d.backend.List(ctx)
	if err != nil {
		d.logger.Printf("warning: loading existing VMs: %v", err)
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, inst := range instances {
		d.vms[inst.Name] = inst
		d.logger.Printf("loaded existing VM: %s (state: %s)", inst.Name, inst.VMState)
	}
}

// handleConnection processes a single CLI RPC connection.
func (d *Daemon) handleConnection(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	var req Request
	if err := dec.Decode(&req); err != nil {
		_ = enc.Encode(Response{Error: fmt.Sprintf("invalid request: %v", err)})
		return
	}

	resp := d.handleRequest(ctx, &req)
	_ = enc.Encode(resp)
}

// handleRequest dispatches an RPC request to the appropriate handler.
func (d *Daemon) handleRequest(ctx context.Context, req *Request) Response {
	switch req.Method {
	case MethodPing:
		return Response{OK: true}

	case MethodUp:
		return d.handleUp(ctx, req)

	case MethodDown:
		return d.handleDown(ctx, req)

	case MethodStatus:
		return d.handleStatus(ctx, req)

	case MethodDestroy:
		return d.handleDestroy(ctx, req)

	case MethodList:
		return d.handleList(ctx)

	default:
		return Response{Error: fmt.Sprintf("unknown method: %s", req.Method)}
	}
}

func (d *Daemon) handleUp(ctx context.Context, req *Request) Response {
	if req.ConfigPath == "" {
		return Response{Error: "config_path is required"}
	}

	cfg, err := config.Load(req.ConfigPath)
	if err != nil {
		return Response{Error: fmt.Sprintf("loading config: %v", err)}
	}

	// Use name from request or config
	name := req.Name
	if name == "" {
		name = cfg.Name
	}

	// Check if a sandbox with this name already exists
	d.mu.RLock()
	existing, exists := d.vms[name]
	d.mu.RUnlock()

	if exists {
		state, _ := d.backend.Status(ctx, existing)

		switch state {
		case vm.StateRunning:
			// Already running -- idempotent no-op
			existing.VMState = state
			d.logger.Printf("sandbox %q is already running", name)
			return Response{
				OK:        true,
				Sandboxes: []SandboxInfo{instanceToInfo(existing)},
			}

		default:
			// Stopped -- re-boot the existing sandbox with its disk
			existing.Config = cfg
			if err := d.backend.Start(ctx, existing); err != nil {
				// If the hypervisor started but post-boot failed, the
				// backend sets VMState = StateRunning before returning.
				// The instance is already in d.vms, so no map update is
				// needed — just log so the user knows they can debug or retry.
				if existing.VMState == vm.StateRunning {
					d.logger.Printf("sandbox %q partially re-started (VM running, post-boot failed): %v", name, err)
				}
				return Response{Error: fmt.Sprintf("starting sandbox: %v", err)}
			}

			d.mu.Lock()
			existing.VMState = vm.StateRunning
			d.mu.Unlock()

			d.logger.Printf("sandbox %q re-started", name)
			return Response{
				OK:        true,
				Sandboxes: []SandboxInfo{instanceToInfo(existing)},
			}
		}
	}

	// New sandbox -- create from scratch
	inst := &vm.Instance{
		Name:   name,
		Config: cfg,
	}

	if err := d.backend.Up(ctx, inst); err != nil {
		// If the hypervisor started but post-boot provisioning failed,
		// the backend sets inst.VMState = StateRunning before returning.
		// Register the instance so the user can debug with `mb ssh`,
		// retry with `mb up`, or clean up with `mb destroy`. Without
		// this, the daemon loses track of the running VM.
		//
		// Note: we check VMState rather than inst.PID because the Apple
		// Virtualization.framework backend has no PID (the VM runs
		// in-process). The backend is responsible for setting VMState
		// to StateRunning once the hypervisor is up, before postBoot.
		if inst.VMState == vm.StateRunning {
			d.mu.Lock()
			d.vms[name] = inst
			d.mu.Unlock()
			d.logger.Printf("sandbox %q partially started (VM running, post-boot failed): %v", name, err)
		}
		return Response{Error: fmt.Sprintf("starting sandbox: %v", err)}
	}

	d.mu.Lock()
	d.vms[name] = inst
	d.mu.Unlock()

	d.logger.Printf("sandbox %q started", name)

	return Response{
		OK: true,
		Sandboxes: []SandboxInfo{
			instanceToInfo(inst),
		},
	}
}

func (d *Daemon) handleDown(ctx context.Context, req *Request) Response {
	inst, err := d.resolveInstance(ctx, req.Name)
	if err != nil {
		return Response{Error: err.Error()}
	}

	var downErr error
	if req.Force {
		downErr = d.backend.ForceDown(ctx, inst)
	} else {
		downErr = d.backend.Down(ctx, inst, 30*1e9) // 30 seconds
	}

	if downErr != nil {
		return Response{Error: fmt.Sprintf("stopping sandbox: %v", downErr)}
	}

	d.mu.Lock()
	inst.VMState = vm.StateStopped
	d.mu.Unlock()

	d.logger.Printf("sandbox %q stopped", inst.Name)
	return Response{OK: true}
}

func (d *Daemon) handleStatus(ctx context.Context, req *Request) Response {
	if req.All {
		return d.handleList(ctx)
	}

	inst, err := d.resolveInstance(ctx, req.Name)
	if err != nil {
		return Response{Error: err.Error()}
	}

	state, _ := d.backend.Status(ctx, inst)
	inst.VMState = state

	return Response{
		OK:        true,
		Sandboxes: []SandboxInfo{instanceToInfo(inst)},
	}
}

func (d *Daemon) handleDestroy(ctx context.Context, req *Request) Response {
	inst, err := d.resolveInstance(ctx, req.Name)
	if err != nil {
		return Response{Error: err.Error()}
	}

	if err := d.backend.Destroy(ctx, inst); err != nil {
		return Response{Error: fmt.Sprintf("destroying sandbox: %v", err)}
	}

	d.mu.Lock()
	delete(d.vms, inst.Name)
	d.mu.Unlock()

	d.logger.Printf("sandbox %q destroyed", inst.Name)
	return Response{OK: true}
}

func (d *Daemon) handleList(ctx context.Context) Response {
	instances, err := d.backend.List(ctx)
	if err != nil {
		return Response{Error: fmt.Sprintf("listing sandboxes: %v", err)}
	}

	infos := make([]SandboxInfo, 0, len(instances))
	for _, inst := range instances {
		infos = append(infos, instanceToInfo(inst))
	}

	return Response{OK: true, Sandboxes: infos}
}

// resolveInstance finds an instance by name. If no name is given, it looks
// for a single sandbox (in any state) and returns it. When multiple sandboxes
// exist and no name is provided, it returns an error listing the options.
func (d *Daemon) resolveInstance(ctx context.Context, name string) (*vm.Instance, error) {
	if name != "" {
		d.mu.RLock()
		inst, ok := d.vms[name]
		d.mu.RUnlock()
		if ok {
			// Refresh state from the backend
			state, _ := d.backend.Status(ctx, inst)
			inst.VMState = state
			return inst, nil
		}
		// Try loading from disk
		inst, err := d.backend.LoadInstance(name)
		if err != nil {
			return nil, fmt.Errorf("sandbox %q not found: %w", name, err)
		}
		state, _ := d.backend.Status(ctx, inst)
		inst.VMState = state
		return inst, nil
	}

	// No name: find a single sandbox (any state).
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.vms) == 0 {
		return nil, fmt.Errorf("no sandboxes found")
	}

	if len(d.vms) == 1 {
		for _, inst := range d.vms {
			state, _ := d.backend.Status(ctx, inst)
			inst.VMState = state
			return inst, nil
		}
	}

	names := make([]string, 0, len(d.vms))
	for n := range d.vms {
		names = append(names, n)
	}
	return nil, fmt.Errorf("multiple sandboxes exist, please specify one: %v", names)
}

func instanceToInfo(inst *vm.Instance) SandboxInfo {
	info := SandboxInfo{
		Name:       inst.Name,
		State:      string(inst.VMState),
		SSHPort:    inst.SSHPort,
		SSHAddress: fmt.Sprintf("127.0.0.1:%d", inst.SSHPort),
		SSHKeyPath: inst.SSHKeyPath,
		VsockPort:  inst.VsockPort,
	}

	// Try loading state for extra info
	if state, err := inst.LoadState(); err == nil {
		info.Mixtape = state.Mixtape
		info.CPUs = state.CPUs
		info.Memory = state.Memory
		info.NetworkMode = state.NetworkMode
		if info.SSHKeyPath == "" {
			info.SSHKeyPath = state.SSHKeyPath
		}
	}

	return info
}

// IsRunning checks if the daemon is running by attempting to connect
// to the socket and sending a ping.
func IsRunning(baseDir string) bool {
	sockPath := SocketPath(baseDir)
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	if err := enc.Encode(Request{Method: MethodPing}); err != nil {
		return false
	}

	var resp Response
	if err := dec.Decode(&resp); err != nil {
		return false
	}

	return resp.OK
}
