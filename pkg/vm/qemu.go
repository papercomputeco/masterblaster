package vm

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/papercomputeco/masterblaster/pkg/config"
	"github.com/papercomputeco/masterblaster/pkg/vsock"
)

// QEMUBackend implements the Backend interface using qemu-system-aarch64.
// It boots StereOS images with HVF acceleration on Apple Silicon Macs,
// uses vsock (via TCP forwarding) for stereosd communication, and QMP
// for hypervisor control.
type QEMUBackend struct {
	baseDir string // ~/.mb
}

// NewQEMUBackend creates a new QEMU backend with the given base directory.
func NewQEMUBackend(baseDir string) *QEMUBackend {
	return &QEMUBackend{baseDir: baseDir}
}

// Up creates and starts a new sandbox VM from the given instance configuration.
func (q *QEMUBackend) Up(ctx context.Context, inst *Instance) error {
	if inst.Config == nil {
		return fmt.Errorf("instance %q has no configuration", inst.Name)
	}
	cfg := inst.Config

	// Resolve the mixtape image
	imagePath, err := ResolveMixtapePath(q.baseDir, cfg.Mixtape)
	if err != nil {
		return fmt.Errorf("resolving mixtape: %w", err)
	}

	// Create VM directory
	vmDir := filepath.Join(VMsDir(q.baseDir), inst.Name)
	if _, err := os.Stat(vmDir); err == nil {
		return fmt.Errorf("sandbox %q already exists at %s", inst.Name, vmDir)
	}
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		return fmt.Errorf("creating VM directory: %w", err)
	}
	inst.Dir = vmDir

	// Determine image format and create disk
	if strings.HasSuffix(imagePath, ".qcow2") {
		// Create qcow2 overlay backed by base image
		if err := createQCOWOverlay(imagePath, inst.QCOWDiskPath(), convertSizeForQEMU(cfg.Resources.Disk)); err != nil {
			os.RemoveAll(vmDir)
			return fmt.Errorf("creating disk overlay: %w", err)
		}
	} else {
		// Raw image: copy and resize
		if err := copyRawImage(imagePath, inst.DiskPath()); err != nil {
			os.RemoveAll(vmDir)
			return fmt.Errorf("copying disk image: %w", err)
		}
		if err := resizeRawImage(inst.DiskPath(), convertSizeForQEMU(cfg.Resources.Disk)); err != nil {
			os.RemoveAll(vmDir)
			return fmt.Errorf("resizing disk image: %w", err)
		}
	}

	// Initialize writable EFI vars
	if err := initEFIVars(inst.EFIVarsPath()); err != nil {
		os.RemoveAll(vmDir)
		return fmt.Errorf("initializing EFI vars: %w", err)
	}

	// Allocate ports
	sshPort, err := allocatePort()
	if err != nil {
		os.RemoveAll(vmDir)
		return fmt.Errorf("allocating SSH port: %w", err)
	}
	inst.SSHPort = sshPort

	vsockTCPPort, err := allocatePort()
	if err != nil {
		os.RemoveAll(vmDir)
		return fmt.Errorf("allocating vsock port: %w", err)
	}
	inst.VsockPort = vsockTCPPort

	inst.QMPSocket = filepath.Join(vmDir, "qmp.sock")

	// Save jcard.toml into the VM directory for reference
	if err := saveJcard(vmDir, cfg); err != nil {
		os.RemoveAll(vmDir)
		return fmt.Errorf("saving jcard config: %w", err)
	}

	// Save state
	stateFile := &StateFile{
		Name:        inst.Name,
		CreatedAt:   time.Now().UTC(),
		Mixtape:     cfg.Mixtape,
		CPUs:        cfg.Resources.CPUs,
		Memory:      cfg.Resources.Memory,
		Disk:        cfg.Resources.Disk,
		NetworkMode: cfg.Network.Mode,
		SSHPort:     sshPort,
		VsockPort:   vsockTCPPort,
	}
	if err := saveState(vmDir, stateFile); err != nil {
		os.RemoveAll(vmDir)
		return fmt.Errorf("saving state: %w", err)
	}

	// Build and start QEMU
	if err := q.startQEMU(ctx, inst, cfg); err != nil {
		os.RemoveAll(vmDir)
		return fmt.Errorf("starting QEMU: %w", err)
	}

	// Post-boot: wait for stereosd, inject secrets, mount shares
	if err := q.postBoot(ctx, inst, cfg); err != nil {
		// Don't remove VM dir on post-boot failure; the VM is running.
		// Let the user debug with `mb ssh` or `mb down`.
		return fmt.Errorf("post-boot provisioning: %w", err)
	}

	return nil
}

// Down gracefully stops the VM.
func (q *QEMUBackend) Down(ctx context.Context, inst *Instance, timeout time.Duration) error {
	// First try vsock shutdown (preferred: allows stereosd to unmount, sync, etc.)
	if inst.VsockPort > 0 {
		if err := q.vsockShutdown(ctx, inst); err == nil {
			// Wait for process to exit
			if q.waitForExit(ctx, inst, timeout) {
				inst.VMState = StateStopped
				return nil
			}
		}
	}

	// Fall back to QMP ACPI shutdown
	client, err := DialQMP(inst.QMPSocket)
	if err != nil {
		return q.ForceDown(ctx, inst)
	}
	defer client.Close()

	if err := client.Shutdown(); err != nil {
		return q.ForceDown(ctx, inst)
	}

	if q.waitForExit(ctx, inst, timeout) {
		inst.VMState = StateStopped
		return nil
	}

	return q.ForceDown(ctx, inst)
}

// ForceDown immediately terminates the VM process.
func (q *QEMUBackend) ForceDown(_ context.Context, inst *Instance) error {
	if inst.PID == 0 {
		pidData, err := os.ReadFile(inst.PIDFilePath())
		if err != nil {
			inst.VMState = StateStopped
			return nil
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
		_ = proc.Kill()
	}

	time.Sleep(1 * time.Second)
	if processAlive(inst.PID) {
		_ = proc.Kill()
	}

	inst.VMState = StateStopped
	return nil
}

// Destroy stops the VM and removes all resources.
func (q *QEMUBackend) Destroy(ctx context.Context, inst *Instance) error {
	// Stop if running
	status, _ := q.Status(ctx, inst)
	if status == StateRunning {
		if err := q.Down(ctx, inst, 10*time.Second); err != nil {
			_ = q.ForceDown(ctx, inst)
		}
	}

	if err := os.RemoveAll(inst.Dir); err != nil {
		return fmt.Errorf("removing VM directory: %w", err)
	}

	return nil
}

// Status returns the current state of the VM.
func (q *QEMUBackend) Status(_ context.Context, inst *Instance) (State, error) {
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
	vmsDir := VMsDir(q.baseDir)
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
			continue
		}
		status, _ := q.Status(ctx, inst)
		inst.VMState = status
		instances = append(instances, inst)
	}

	return instances, nil
}

// LoadInstance reads a persisted VM instance by name.
func (q *QEMUBackend) LoadInstance(name string) (*Instance, error) {
	vmDir := filepath.Join(VMsDir(q.baseDir), name)
	state, err := loadState(vmDir)
	if err != nil {
		return nil, fmt.Errorf("loading VM %q: %w", name, err)
	}

	inst := &Instance{
		Name:      state.Name,
		Dir:       vmDir,
		QMPSocket: filepath.Join(vmDir, "qmp.sock"),
		SSHPort:   state.SSHPort,
		VsockPort: state.VsockPort,
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

// startQEMU launches qemu-system-aarch64 as a daemon.
func (q *QEMUBackend) startQEMU(ctx context.Context, inst *Instance, cfg *config.JcardConfig) error {
	qemuBin, err := exec.LookPath("qemu-system-aarch64")
	if err != nil {
		return fmt.Errorf("qemu-system-aarch64 not found: %w\nInstall QEMU: brew install qemu", err)
	}

	args, err := q.buildArgs(inst, cfg)
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

// buildArgs constructs the qemu-system-aarch64 command line arguments.
func (q *QEMUBackend) buildArgs(inst *Instance, cfg *config.JcardConfig) ([]string, error) {
	efiCode, err := findEFIFirmware()
	if err != nil {
		return nil, err
	}

	// Determine disk format and path
	diskArg := ""
	if _, err := os.Stat(inst.QCOWDiskPath()); err == nil {
		diskArg = fmt.Sprintf("if=virtio,format=qcow2,file=%s", inst.QCOWDiskPath())
	} else {
		diskArg = fmt.Sprintf("if=virtio,format=raw,file=%s", inst.DiskPath())
	}

	// Parse memory for QEMU (convert GiB -> G, MiB -> M, etc.)
	memory := convertSizeForQEMU(cfg.Resources.Memory)

	args := []string{
		// Machine + acceleration
		"-machine", "virt,accel=hvf,highmem=on",
		"-cpu", "host",

		// Resources
		"-smp", fmt.Sprintf("%d", cfg.Resources.CPUs),
		"-m", memory,

		// EFI firmware (read-only pflash for code, writable copy for vars)
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s,readonly=on", efiCode),
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", inst.EFIVarsPath()),

		// Boot disk
		"-drive", diskArg,

		// QMP control socket
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", inst.QMPSocket),

		// Serial console to log file
		"-serial", fmt.Sprintf("file:%s", inst.SerialLogPath()),

		// Headless
		"-nographic",
		"-nodefaults",

		// PID file
		"-pidfile", inst.PIDFilePath(),

		// Daemonize so mb returns immediately
		"-daemonize",
	}

	// Networking
	args = append(args, q.buildNetworkArgs(inst, cfg)...)

	// Vsock device for host<->guest communication with stereosd.
	// On the virt machine type (aarch64), use vhost-vsock-device (virtio-mmio).
	// vhost-vsock-pci is only valid for x86 q35/pc machine types.
	args = append(args,
		"-device", fmt.Sprintf("vhost-vsock-device,guest-cid=%d", vsock.VsockGuestCID),
	)

	// Shared directories via virtio-9p
	for i, shared := range cfg.Shared {
		tag := fmt.Sprintf("share%d", i)
		securityModel := "mapped-xattr"
		if shared.ReadOnly {
			securityModel = "none"
		}
		args = append(args,
			"-fsdev", fmt.Sprintf("local,id=%s,path=%s,security_model=%s", tag, shared.Host, securityModel),
			"-device", fmt.Sprintf("virtio-9p-pci,fsdev=%s,mount_tag=%s", tag, tag),
		)
	}

	return args, nil
}

// buildNetworkArgs constructs QEMU network arguments based on config.
func (q *QEMUBackend) buildNetworkArgs(inst *Instance, cfg *config.JcardConfig) []string {
	switch cfg.Network.Mode {
	case "none":
		return []string{"-nic", "none"}

	case "bridged":
		// bridged mode uses vmnet-shared on macOS
		return []string{
			"-netdev", "vmnet-shared,id=net0",
			"-device", "virtio-net-pci,netdev=net0",
		}

	default: // "nat"
		// Build host forward string: always include SSH
		hostfwds := fmt.Sprintf("hostfwd=tcp::%d-:22", inst.SSHPort)

		// Add configured port forwards
		for _, fwd := range cfg.Network.Forwards {
			hostfwds += fmt.Sprintf(",hostfwd=%s::%d-:%d", fwd.Proto, fwd.Host, fwd.Guest)
		}

		return []string{
			"-netdev", fmt.Sprintf("user,id=net0,%s", hostfwds),
			"-device", "virtio-net-pci,netdev=net0",
		}
	}
}

// postBoot handles post-boot provisioning: wait for stereosd, inject secrets,
// mount shared directories.
func (q *QEMUBackend) postBoot(ctx context.Context, inst *Instance, cfg *config.JcardConfig) error {
	// Wait for stereosd to be ready via vsock
	addr := fmt.Sprintf("127.0.0.1:%d", inst.VsockPort)

	// Give the VM a moment to boot
	time.Sleep(2 * time.Second)

	// Try connecting to stereosd with retries
	var client *vsock.Client
	var err error
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		client, err = vsock.Dial(addr, 5*time.Second)
		if err == nil {
			break
		}
		time.Sleep(3 * time.Second)
	}
	if client == nil {
		return fmt.Errorf("could not connect to stereosd at %s after 120s: %w", addr, err)
	}
	defer client.Close()

	// Wait for ready state
	readyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := client.WaitForReady(readyCtx, 2*time.Second); err != nil {
		return fmt.Errorf("waiting for stereosd ready: %w", err)
	}

	// Inject secrets
	for name, value := range cfg.Secrets {
		if err := client.InjectSecret(ctx, name, value); err != nil {
			return fmt.Errorf("injecting secret %q: %w", name, err)
		}
	}

	// Mount shared directories
	for i, shared := range cfg.Shared {
		tag := fmt.Sprintf("share%d", i)
		if err := client.Mount(ctx, tag, shared.Guest, "9p", shared.ReadOnly); err != nil {
			return fmt.Errorf("mounting %q at %q: %w", shared.Host, shared.Guest, err)
		}
	}

	return nil
}

// vsockShutdown sends a shutdown command to stereosd via vsock.
func (q *QEMUBackend) vsockShutdown(ctx context.Context, inst *Instance) error {
	addr := fmt.Sprintf("127.0.0.1:%d", inst.VsockPort)
	client, err := vsock.Dial(addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Shutdown(ctx, "mb down")
}

// waitForExit polls until the QEMU process exits or timeout.
func (q *QEMUBackend) waitForExit(_ context.Context, inst *Instance, timeout time.Duration) bool {
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return false
		case <-ticker.C:
			if !processAlive(inst.PID) {
				return true
			}
		}
	}
}

// findEFIFirmware locates the UEFI firmware (edk2-aarch64-code.fd).
func findEFIFirmware() (string, error) {
	qemuBin, err := exec.LookPath("qemu-system-aarch64")
	if err != nil {
		return "", fmt.Errorf("qemu-system-aarch64 not found: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(qemuBin)
	if err != nil {
		resolved = qemuBin
	}

	binDir := filepath.Dir(resolved)
	prefix := filepath.Dir(binDir)
	candidate := filepath.Join(prefix, "share", "qemu", "edk2-aarch64-code.fd")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	brewCandidate := "/opt/homebrew/share/qemu/edk2-aarch64-code.fd"
	if _, err := os.Stat(brewCandidate); err == nil {
		return brewCandidate, nil
	}

	return "", fmt.Errorf("EFI firmware not found at %s or %s\nReinstall QEMU: brew reinstall qemu", candidate, brewCandidate)
}

// initEFIVars creates a writable EFI variable store (64MB of zeros).
func initEFIVars(path string) error {
	const size = 64 * 1024 * 1024
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
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// allocatePort finds a free TCP port by binding to :0.
func allocatePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("finding free port: %w", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// convertSizeForQEMU converts human-friendly size strings like "4GiB" to
// QEMU-compatible suffixes like "4G". Used for both memory and disk sizes.
func convertSizeForQEMU(mem string) string {
	mem = strings.TrimSpace(mem)
	replacements := map[string]string{
		"GiB": "G",
		"MiB": "M",
		"KiB": "K",
		"gib": "G",
		"mib": "M",
		"kib": "K",
	}
	for old, new := range replacements {
		if strings.HasSuffix(mem, old) {
			return strings.TrimSuffix(mem, old) + new
		}
	}
	return mem
}

// saveJcard writes the jcard configuration to the VM directory.
func saveJcard(vmDir string, cfg *config.JcardConfig) error {
	data, err := config.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling jcard config: %w", err)
	}
	return os.WriteFile(filepath.Join(vmDir, "jcard.toml"), data, 0644)
}
