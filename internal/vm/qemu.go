package vm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/paper-compute-co/masterblaster/internal/config"
)

// QEMUBackend implements the Backend interface using qemu-system-aarch64.
type QEMUBackend struct {
	configDir string // ~/.mb
}

// NewQEMUBackend creates a new QEMU backend with the given config directory.
func NewQEMUBackend(configDir string) *QEMUBackend {
	return &QEMUBackend{configDir: configDir}
}

// EnsureBaseImage validates that the base image is present and returns its path.
func (q *QEMUBackend) EnsureBaseImage(_ context.Context, image string) (string, error) {
	path := resolveBaseImagePath(q.configDir, image)
	if err := validateBaseImage(path); err != nil {
		return "", err
	}
	return path, nil
}

// Create prepares VM resources (overlay disk, cloud-init ISO, EFI vars) but does not start.
func (q *QEMUBackend) Create(ctx context.Context, opts CreateOpts) (*Instance, error) {
	vmDir := filepath.Join(q.configDir, "vms", opts.Name)

	// Check if VM already exists
	if _, err := os.Stat(vmDir); err == nil {
		return nil, fmt.Errorf("VM %q already exists at %s", opts.Name, vmDir)
	}

	// Create VM directory
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		return nil, fmt.Errorf("creating VM directory: %w", err)
	}

	inst := &Instance{
		Name:       opts.Name,
		Dir:        vmDir,
		QMPSocket:  filepath.Join(vmDir, "qmp.sock"),
		SSHAddress: fmt.Sprintf("127.0.0.1:%d", opts.SSHHostPort),
		VMState:    StateCreated,
	}

	// Create qcow2 overlay backed by base image
	if err := createOverlay(opts.BaseImage, inst.DiskPath(), opts.DiskSize); err != nil {
		os.RemoveAll(vmDir)
		return nil, fmt.Errorf("creating disk overlay: %w", err)
	}

	// Initialize writable EFI vars
	if err := initEFIVars(inst.EFIVarsPath()); err != nil {
		os.RemoveAll(vmDir)
		return nil, fmt.Errorf("initializing EFI vars: %w", err)
	}

	// Build volume guest mount map for cloud-init
	volGuests := make(map[string]string, len(opts.Volumes))
	for tag, vol := range opts.Volumes {
		volGuests[tag] = vol.Guest
	}

	// Load OpenCode config if specified
	ciData := opts.CloudInit
	if opts.CloudInit.OpenCode.ConfigFile != "" {
		data, err := os.ReadFile(opts.CloudInit.OpenCode.ConfigFile)
		if err != nil {
			os.RemoveAll(vmDir)
			return nil, fmt.Errorf("reading OpenCode config file: %w", err)
		}
		ciData.ConfigJSON = string(data)
	}

	// Generate cloud-init ISO
	if err := generateCloudInitISO(inst.CloudInitISO(), ciData, volGuests); err != nil {
		os.RemoveAll(vmDir)
		return nil, fmt.Errorf("generating cloud-init ISO: %w", err)
	}

	// Save state
	stateFile := &StateFile{
		Name:        opts.Name,
		CreatedAt:   time.Now().UTC(),
		BaseImage:   filepath.Base(opts.BaseImage),
		SSHUser:     opts.CloudInit.User,
		SSHHostPort: opts.SSHHostPort,
		CPUs:        opts.CPUs,
		Memory:      opts.Memory,
	}
	if err := saveState(vmDir, stateFile); err != nil {
		os.RemoveAll(vmDir)
		return nil, fmt.Errorf("saving state: %w", err)
	}

	return inst, nil
}

// Start boots the VM by launching qemu-system-aarch64 as a daemon.
func (q *QEMUBackend) Start(ctx context.Context, inst *Instance) error {
	qemuBin, err := exec.LookPath("qemu-system-aarch64")
	if err != nil {
		return fmt.Errorf("qemu-system-aarch64 not found: %w\nInstall QEMU: brew install qemu", err)
	}

	// Load state to get config
	state, err := loadState(inst.Dir)
	if err != nil {
		return fmt.Errorf("loading VM state: %w", err)
	}

	args, err := q.buildArgs(inst, state)
	if err != nil {
		return fmt.Errorf("building QEMU args: %w", err)
	}

	cmd := exec.CommandContext(ctx, qemuBin, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("starting QEMU: %w\nCheck serial log: %s", err, inst.SerialLogPath())
	}

	// Read PID from pidfile
	pidData, err := os.ReadFile(inst.PIDFilePath())
	if err != nil {
		return fmt.Errorf("reading QEMU PID file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return fmt.Errorf("parsing QEMU PID: %w", err)
	}
	inst.PID = pid
	inst.VMState = StateRunning

	return nil
}

// Stop sends ACPI shutdown via QMP. Falls back to kill after timeout.
func (q *QEMUBackend) Stop(ctx context.Context, inst *Instance, timeout time.Duration) error {
	client, err := DialQMP(inst.QMPSocket)
	if err != nil {
		// QMP not available — try to kill the process directly
		return q.Kill(ctx, inst)
	}
	defer client.Close()

	// Send graceful shutdown
	if err := client.Shutdown(); err != nil {
		return q.Kill(ctx, inst)
	}

	// Wait for process to exit
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			// Timeout — force kill
			return q.Kill(ctx, inst)
		case <-ticker.C:
			if !processAlive(inst.PID) {
				inst.VMState = StateStopped
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Kill force-terminates the VM process.
func (q *QEMUBackend) Kill(_ context.Context, inst *Instance) error {
	if inst.PID == 0 {
		// Try reading PID from file
		pidData, err := os.ReadFile(inst.PIDFilePath())
		if err != nil {
			inst.VMState = StateStopped
			return nil // No PID, assume stopped
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if err != nil {
			inst.VMState = StateStopped
			return nil
		}
		inst.PID = pid
	}

	if !processAlive(inst.PID) {
		inst.VMState = StateStopped
		return nil
	}

	proc, err := os.FindProcess(inst.PID)
	if err != nil {
		inst.VMState = StateStopped
		return nil
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// Try SIGKILL as last resort
		_ = proc.Kill()
	}

	// Wait briefly for the process to exit
	time.Sleep(1 * time.Second)
	if processAlive(inst.PID) {
		_ = proc.Kill()
	}

	inst.VMState = StateStopped
	return nil
}

// Remove deletes all VM resources.
func (q *QEMUBackend) Remove(ctx context.Context, inst *Instance) error {
	// Stop if running
	if processAlive(inst.PID) {
		if err := q.Stop(ctx, inst, 10*time.Second); err != nil {
			// Force kill on error
			_ = q.Kill(ctx, inst)
		}
	}

	// Remove QMP socket
	_ = os.Remove(inst.QMPSocket)

	// Remove entire VM directory
	if err := os.RemoveAll(inst.Dir); err != nil {
		return fmt.Errorf("removing VM directory: %w", err)
	}

	return nil
}

// Status returns the current state of the VM.
func (q *QEMUBackend) Status(_ context.Context, inst *Instance) (State, error) {
	// Check if PID file exists and process is alive
	pid := inst.PID
	if pid == 0 {
		pidData, err := os.ReadFile(inst.PIDFilePath())
		if err != nil {
			return StateStopped, nil
		}
		p, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if err != nil {
			return StateStopped, nil
		}
		pid = p
	}

	if !processAlive(pid) {
		return StateStopped, nil
	}

	// Try QMP for more precise status
	client, err := DialQMP(inst.QMPSocket)
	if err != nil {
		// Process alive but QMP not responding — could be starting up
		return StateRunning, nil
	}
	defer client.Close()

	status, err := client.QueryStatus()
	if err != nil {
		return StateRunning, nil
	}

	switch status {
	case "running":
		return StateRunning, nil
	case "shutdown", "postmigrate":
		return StateStopped, nil
	default:
		return StateRunning, nil
	}
}

// List returns all known VM instances by scanning the vms directory.
func (q *QEMUBackend) List(ctx context.Context) ([]*Instance, error) {
	vmsDir := filepath.Join(q.configDir, "vms")
	entries, err := os.ReadDir(vmsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading VMs directory: %w", err)
	}

	var instances []*Instance
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		inst, err := q.LoadInstance(entry.Name())
		if err != nil {
			continue // Skip broken entries
		}
		// Update status
		status, _ := q.Status(ctx, inst)
		inst.VMState = status
		instances = append(instances, inst)
	}

	return instances, nil
}

// LoadInstance reads a persisted VM instance by name.
func (q *QEMUBackend) LoadInstance(name string) (*Instance, error) {
	vmDir := filepath.Join(q.configDir, "vms", name)
	state, err := loadState(vmDir)
	if err != nil {
		return nil, fmt.Errorf("loading VM %q: %w", name, err)
	}

	inst := &Instance{
		Name:       state.Name,
		Dir:        vmDir,
		QMPSocket:  filepath.Join(vmDir, "qmp.sock"),
		SSHAddress: fmt.Sprintf("127.0.0.1:%d", state.SSHHostPort),
	}

	// Try to read PID
	pidData, err := os.ReadFile(inst.PIDFilePath())
	if err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(pidData))); err == nil {
			inst.PID = pid
		}
	}

	return inst, nil
}

// buildArgs constructs the qemu-system-aarch64 command line arguments.
func (q *QEMUBackend) buildArgs(inst *Instance, state *StateFile) ([]string, error) {
	efiCode, err := findEFIFirmware()
	if err != nil {
		return nil, err
	}

	args := []string{
		// Machine + acceleration
		"-machine", "virt,accel=hvf,highmem=on",
		"-cpu", "host",

		// Resources
		"-smp", fmt.Sprintf("%d", state.CPUs),
		"-m", state.Memory,

		// EFI firmware (read-only pflash for code, writable copy for vars)
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s,readonly=on", efiCode),
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", inst.EFIVarsPath()),

		// Boot disk (COW overlay backed by base image)
		"-drive", fmt.Sprintf("if=virtio,format=qcow2,file=%s", inst.DiskPath()),

		// cloud-init ISO
		"-drive", fmt.Sprintf("if=virtio,format=raw,file=%s,readonly=on", inst.CloudInitISO()),

		// Networking: user-mode with SSH port forward
		"-netdev", fmt.Sprintf("user,id=net0,hostfwd=tcp::%d-:22", state.SSHHostPort),
		"-device", "virtio-net-pci,netdev=net0",

		// QMP control socket
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", inst.QMPSocket),

		// Serial console to log file
		"-serial", fmt.Sprintf("file:%s", inst.SerialLogPath()),

		// Headless
		"-nographic",
		"-nodefaults",

		// PID file for process tracking
		"-pidfile", inst.PIDFilePath(),

		// Daemonize so mb init returns immediately
		"-daemonize",
	}

	return args, nil
}

// findEFIFirmware locates the UEFI firmware (edk2-aarch64-code.fd) by
// deriving the path from the qemu-system-aarch64 binary's location.
func findEFIFirmware() (string, error) {
	qemuBin, err := exec.LookPath("qemu-system-aarch64")
	if err != nil {
		return "", fmt.Errorf("qemu-system-aarch64 not found: %w", err)
	}

	// Resolve symlinks to get the real path
	resolved, err := filepath.EvalSymlinks(qemuBin)
	if err != nil {
		resolved = qemuBin
	}

	// Derive: .../bin/qemu-system-aarch64 → .../share/qemu/edk2-aarch64-code.fd
	binDir := filepath.Dir(resolved)
	prefix := filepath.Dir(binDir)
	candidate := filepath.Join(prefix, "share", "qemu", "edk2-aarch64-code.fd")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	// Try common Homebrew location
	brewCandidate := "/opt/homebrew/share/qemu/edk2-aarch64-code.fd"
	if _, err := os.Stat(brewCandidate); err == nil {
		return brewCandidate, nil
	}

	return "", fmt.Errorf("EFI firmware not found at %s or %s\nReinstall QEMU: brew reinstall qemu", candidate, brewCandidate)
}

// initEFIVars creates a writable EFI variable store (64MB of zeros).
// Done in pure Go to avoid portability issues between BSD dd (bs=1m)
// and GNU dd (bs=1M).
func initEFIVars(path string) error {
	const size = 64 * 1024 * 1024 // 64MB
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating EFI vars file: %w", err)
	}
	defer f.Close()
	if err := f.Truncate(size); err != nil {
		return fmt.Errorf("sizing EFI vars file: %w", err)
	}
	return nil
}

// processAlive checks if a process with the given PID exists.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Use signal 0 to check existence.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// addVolumeArgs appends virtio-9p shared directory arguments for each volume.
func addVolumeArgs(args []string, volumes map[string]config.VolumeMount) []string {
	for tag, vol := range volumes {
		args = append(args,
			"-fsdev", fmt.Sprintf("local,id=%s,path=%s,security_model=mapped-xattr", tag, vol.Host),
			"-device", fmt.Sprintf("virtio-9p-pci,fsdev=%s,mount_tag=%s", tag, tag),
		)
	}
	return args
}
