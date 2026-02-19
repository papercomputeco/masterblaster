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

// QEMUBackend implements the Backend interface using QEMU.
// Platform-specific behavior (acceleration, vsock transport, EFI paths)
// is configured via QEMUPlatformConfig, injected at construction time
// by the build-tagged NewPlatformBackend() functions.
type QEMUBackend struct {
	baseDir  string
	platform *QEMUPlatformConfig
}

// NewQEMUBackend creates a new QEMU backend with the given base directory
// and platform-specific configuration.
func NewQEMUBackend(baseDir string, platform *QEMUPlatformConfig) *QEMUBackend {
	return &QEMUBackend{baseDir: baseDir, platform: platform}
}

// Up creates and starts a new sandbox VM from the given instance configuration.
// It creates the VM directory, disk, and EFI vars from scratch, then delegates
// to boot() for port allocation, QEMU launch, and post-boot provisioning.
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

	// Resolve kernel artifacts for direct kernel boot (if available)
	var kernelArtifacts *KernelArtifacts
	if q.platform.DirectKernelBoot {
		kernelArtifacts = ResolveKernelArtifacts(q.baseDir, cfg.Mixtape)
	}

	// Determine image format and create disk
	if strings.HasSuffix(imagePath, ".qcow2") {
		// Create qcow2 overlay backed by base image
		if err := createQCOWOverlay(imagePath, inst.QCOWDiskPath(), convertSizeForQEMU(cfg.Resources.Disk)); err != nil {
			_ = os.RemoveAll(vmDir)
			return fmt.Errorf("creating disk overlay: %w", err)
		}
	} else {
		// Raw image: copy and resize
		if err := copyRawImage(imagePath, inst.DiskPath()); err != nil {
			_ = os.RemoveAll(vmDir)
			return fmt.Errorf("copying disk image: %w", err)
		}
		if err := resizeRawImage(inst.DiskPath(), convertSizeForQEMU(cfg.Resources.Disk)); err != nil {
			_ = os.RemoveAll(vmDir)
			return fmt.Errorf("resizing disk image: %w", err)
		}
	}

	// Initialize writable EFI vars — only needed for EFI boot (skipped
	// when direct kernel boot artifacts are available).
	if kernelArtifacts == nil {
		if err := initEFIVars(inst.EFIVarsPath()); err != nil {
			_ = os.RemoveAll(vmDir)
			return fmt.Errorf("initializing EFI vars: %w", err)
		}
	}

	// Save jcard.toml into the VM directory for reference
	if err := saveJcard(vmDir, cfg); err != nil {
		_ = os.RemoveAll(vmDir)
		return fmt.Errorf("saving jcard config: %w", err)
	}

	// Boot: allocate ports, start QEMU, post-boot provisioning
	if err := q.boot(ctx, inst, cfg, kernelArtifacts); err != nil {
		// If QEMU hasn't started yet, clean up the VM dir.
		// If it has (post-boot failure), leave it for debugging.
		if inst.PID == 0 {
			_ = os.RemoveAll(vmDir)
		}
		return err
	}

	return nil
}

// Start re-boots an existing stopped sandbox. The VM directory and disk
// are reused from a previous Up call. It re-reads the saved jcard.toml
// from the VM directory, allocates new ports, and boots QEMU.
func (q *QEMUBackend) Start(ctx context.Context, inst *Instance) error {
	// Ensure the VM directory exists
	if inst.Dir == "" {
		inst.Dir = filepath.Join(VMsDir(q.baseDir), inst.Name)
	}
	if _, err := os.Stat(inst.Dir); os.IsNotExist(err) {
		return fmt.Errorf("sandbox %q has no VM directory at %s", inst.Name, inst.Dir)
	}

	// Load config from the saved jcard.toml in the VM directory if not
	// already attached (the daemon may have re-loaded from a new config).
	cfg := inst.Config
	if cfg == nil {
		var err error
		cfg, err = config.Load(inst.JcardPath())
		if err != nil {
			return fmt.Errorf("loading saved config for %q: %w", inst.Name, err)
		}
		inst.Config = cfg
	}

	// Clean up stale runtime files from the previous run
	_ = os.Remove(inst.QMPSocket)
	_ = os.Remove(inst.PIDFilePath())

	// Resolve kernel artifacts for direct kernel boot (if available)
	var kernelArtifacts *KernelArtifacts
	if q.platform.DirectKernelBoot {
		kernelArtifacts = ResolveKernelArtifacts(q.baseDir, cfg.Mixtape)
	}

	// Boot: allocate ports, start QEMU, post-boot provisioning
	return q.boot(ctx, inst, cfg, kernelArtifacts)
}

// boot is the shared boot sequence used by both Up (new sandbox) and
// Start (re-boot existing). It allocates ports, updates state.json,
// starts QEMU, and runs post-boot provisioning (stereosd handshake,
// secret injection, shared directory mounting).
//
// kernelArtifacts may be nil, in which case QEMU boots via EFI firmware.
func (q *QEMUBackend) boot(ctx context.Context, inst *Instance, cfg *config.JcardConfig, kernelArtifacts *KernelArtifacts) error {
	// Allocate ports
	sshPort, err := allocatePort()
	if err != nil {
		return fmt.Errorf("allocating SSH port: %w", err)
	}
	inst.SSHPort = sshPort

	// Allocate a TCP port for the stereosd control plane.
	// TODO(@jpmcb): Once native AF_VSOCK transport is implemented for
	// Linux/KVM, this can be made conditional on ControlPlaneMode == "tcp".
	// For now, all platforms use TCP through QEMU user-mode networking
	// and stereosd listens on TCP via --listen-mode auto.
	vsockTCPPort, err := allocatePort()
	if err != nil {
		return fmt.Errorf("allocating control plane port: %w", err)
	}
	inst.VsockPort = vsockTCPPort

	inst.QMPSocket = filepath.Join(inst.Dir, "qmp.sock")

	// Save/update state
	stateFile := &StateFile{
		Name:        inst.Name,
		CreatedAt:   time.Now().UTC(),
		Mixtape:     cfg.Mixtape,
		CPUs:        cfg.Resources.CPUs,
		Memory:      cfg.Resources.Memory,
		Disk:        cfg.Resources.Disk,
		NetworkMode: cfg.Network.Mode,
		SSHPort:     inst.SSHPort,
		VsockPort:   inst.VsockPort,
	}
	if err := saveState(inst.Dir, stateFile); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	// Build and start QEMU
	if err := q.startQEMU(ctx, inst, cfg, kernelArtifacts); err != nil {
		return fmt.Errorf("starting QEMU: %w", err)
	}

	// Post-boot: wait for stereosd, inject secrets, mount shares
	if err := q.postBoot(ctx, inst, cfg); err != nil {
		// Don't kill QEMU on post-boot failure; the VM is running.
		// Let the user debug with `mb ssh` or `mb down`.
		return fmt.Errorf("post-boot provisioning: %w", err)
	}

	return nil
}

// Down gracefully stops the VM.
func (q *QEMUBackend) Down(ctx context.Context, inst *Instance, timeout time.Duration) error {
	// First try stereosd shutdown (preferred: allows stereosd to unmount, sync, etc.)
	if inst.VsockPort > 0 || q.platform.ControlPlaneMode == "vsock" {
		if err := q.controlPlaneShutdown(ctx, inst); err == nil {
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
	defer func() { _ = client.Close() }()

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
	defer func() { _ = client.Close() }()

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

// startQEMU launches the QEMU system emulator as a daemon.
func (q *QEMUBackend) startQEMU(ctx context.Context, inst *Instance, cfg *config.JcardConfig, kernelArtifacts *KernelArtifacts) error {
	qemuBin, err := exec.LookPath(q.platform.Binary)
	if err != nil {
		return fmt.Errorf("%s not found: %w\nInstall QEMU: brew install qemu", q.platform.Binary, err)
	}

	args, err := q.buildArgs(inst, cfg, kernelArtifacts)
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

// buildArgs constructs the QEMU command line arguments using platform config.
// When kernelArtifacts is non-nil, direct kernel boot is used (bypassing
// EFI firmware and GRUB). Otherwise, EFI pflash boot is used.
func (q *QEMUBackend) buildArgs(inst *Instance, cfg *config.JcardConfig, kernelArtifacts *KernelArtifacts) ([]string, error) {
	// Determine disk format and path
	var diskFormat, diskPath string
	if _, err := os.Stat(inst.QCOWDiskPath()); err == nil {
		diskFormat = "qcow2"
		diskPath = inst.QCOWDiskPath()
	} else {
		diskFormat = "raw"
		diskPath = inst.DiskPath()
	}

	// Build the disk drive argument with optional AIO and cache settings
	diskArg := fmt.Sprintf("if=virtio,format=%s,file=%s", diskFormat, diskPath)
	if q.platform.DiskAIO != "" {
		diskArg += ",aio=" + q.platform.DiskAIO
	}
	if q.platform.DiskCache != "" {
		diskArg += ",cache=" + q.platform.DiskCache
	}
	diskArg += ",discard=unmap"

	// Parse memory for QEMU (convert GiB -> G, MiB -> M, etc.)
	memory := convertSizeForQEMU(cfg.Resources.Memory)

	// Machine type and acceleration from platform config
	machineArg := fmt.Sprintf("%s,accel=%s,highmem=on",
		q.platform.DefaultMachineType(), q.platform.Accelerator)

	args := []string{
		// Machine + acceleration
		"-machine", machineArg,
		"-cpu", "host",

		// Resources
		"-smp", fmt.Sprintf("%d", cfg.Resources.CPUs),
		"-m", memory,
	}

	// Boot method: direct kernel boot or EFI firmware
	if kernelArtifacts != nil {
		// Direct kernel boot — skip EFI firmware entirely.
		// This eliminates OVMF init (~1-2s) and GRUB (~0.5-1s).
		args = append(args,
			"-kernel", kernelArtifacts.Kernel,
			"-initrd", kernelArtifacts.Initrd,
			"-append", kernelArtifacts.Cmdline,
		)
	} else {
		// EFI firmware boot (fallback when kernel artifacts unavailable)
		efiCode, err := q.findEFIFirmware()
		if err != nil {
			return nil, err
		}
		args = append(args,
			"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s,readonly=on", efiCode),
			"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", inst.EFIVarsPath()),
		)
	}

	args = append(args,
		// Boot disk
		"-drive", diskArg,

		// QMP control socket
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", inst.QMPSocket),

		// Serial console to log file
		"-serial", fmt.Sprintf("file:%s", inst.SerialLogPath()),

		// Headless, no default devices, no user config overrides
		"-nographic",
		"-nodefaults",
		"-no-user-config",

		// PID file
		"-pidfile", inst.PIDFilePath(),

		// Daemonize so mb returns immediately
		"-daemonize",
	)

	// Networking
	args = append(args, q.buildNetworkArgs(inst, cfg)...)

	// Control plane device: native vsock when available, otherwise the
	// stereosd port is forwarded via TCP hostfwd in buildNetworkArgs.
	if q.platform.ControlPlaneMode == "vsock" {
		args = append(args,
			"-device", fmt.Sprintf("%s,guest-cid=%d", q.platform.VsockDevice, vsock.VsockGuestCID),
		)
	}

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
// When the control plane uses TCP transport, the stereosd port is forwarded
// here alongside SSH. With native vsock, only SSH is forwarded.
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
		// Always forward SSH
		hostfwds := fmt.Sprintf("hostfwd=tcp::%d-:22", inst.SSHPort)

		// Forward stereosd control plane via TCP through SLIRP.
		// TODO(@jpmcb): Once native AF_VSOCK transport is implemented for
		// Linux/KVM, this can be skipped when ControlPlaneMode == "vsock".
		hostfwds += fmt.Sprintf(",hostfwd=tcp::%d-:%d", inst.VsockPort, vsock.VsockPort)

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
	transport := q.controlPlaneTransport(inst)

	// Give the VM a moment to boot
	time.Sleep(2 * time.Second)

	// Try connecting to stereosd with retries
	var client *vsock.Client
	var err error
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		client, err = vsock.Connect(transport, 5*time.Second)
		if err == nil {
			break
		}
		time.Sleep(3 * time.Second)
	}
	if client == nil {
		return fmt.Errorf("could not connect to stereosd via %s after 120s: %w", transport, err)
	}
	defer func() { _ = client.Close() }()

	// Wait for ready state
	readyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := client.WaitForReady(readyCtx, 2*time.Second); err != nil {
		return fmt.Errorf("waiting for stereosd ready: %w", err)
	}

	// Send jcard.toml config to the guest for agentd
	cfgBytes, err := config.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config for guest: %w", err)
	}
	if err := client.SetConfig(ctx, string(cfgBytes)); err != nil {
		return fmt.Errorf("setting guest config: %w", err)
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

// controlPlaneShutdown sends a shutdown command to stereosd via the
// control plane transport.
func (q *QEMUBackend) controlPlaneShutdown(ctx context.Context, inst *Instance) error {
	transport := q.controlPlaneTransport(inst)
	client, err := vsock.Connect(transport, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	return client.Shutdown(ctx, "mb down")
}

// controlPlaneTransport returns the appropriate Transport for connecting
// to stereosd based on the platform's control plane mode.
//
// TODO(@jpmcb): Implement native AF_VSOCK transport for Linux/KVM.
// When ControlPlaneMode == "vsock", this should return a VsockTransport
// that dials AF_VSOCK CID:3 port 1024 directly, bypassing TCP/SLIRP.
// See the VsockTransport sketch in pkg/vsock/transport.go.
func (q *QEMUBackend) controlPlaneTransport(inst *Instance) vsock.Transport {
	// All platforms currently use TCP through QEMU user-mode networking.
	// stereosd inside the guest listens on TCP via --listen-mode auto.
	return &vsock.TCPTransport{Host: "127.0.0.1", Port: inst.VsockPort}
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

// findEFIFirmware locates the UEFI firmware by searching paths from the
// platform config. The special prefix "{qemu_prefix}" is resolved at
// runtime to the QEMU installation prefix.
func (q *QEMUBackend) findEFIFirmware() (string, error) {
	// Resolve the QEMU prefix (parent of bin/) for {qemu_prefix} expansion.
	qemuPrefix := ""
	if qemuBin, err := exec.LookPath(q.platform.Binary); err == nil {
		resolved, err := filepath.EvalSymlinks(qemuBin)
		if err != nil {
			resolved = qemuBin
		}
		qemuPrefix = filepath.Dir(filepath.Dir(resolved))
	}

	var searched []string
	for _, pattern := range q.platform.EFISearchPaths {
		candidate := pattern
		if qemuPrefix != "" {
			candidate = strings.ReplaceAll(candidate, "{qemu_prefix}", qemuPrefix)
		}
		searched = append(searched, candidate)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("EFI firmware not found; searched:\n  %s\n\nIs QEMU installed?",
		strings.Join(searched, "\n  "))
}

// initEFIVars creates a writable EFI variable store (64MB of zeros).
func initEFIVars(path string) error {
	const size = 64 * 1024 * 1024
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating EFI vars file: %w", err)
	}
	defer func() { _ = f.Close() }()
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
	_ = l.Close()
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
