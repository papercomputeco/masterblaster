//go:build darwin && arm64

package vm

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	vz "github.com/Code-Hex/vz/v3"
	"github.com/papercomputeco/masterblaster/pkg/config"
	"github.com/papercomputeco/masterblaster/pkg/ssh"
	"github.com/papercomputeco/masterblaster/pkg/vsock"
)

const backendNameAppleVirt = "applevirt"

// AppleVirtBackend implements the Backend interface using Apple
// Virtualization.framework via the github.com/Code-Hex/vz/v3 bindings.
//
// Requirements:
//   - macOS 13 (Ventura) or newer for EFI boot support.
//   - The mb binary must be codesigned with the
//     com.apple.security.virtualization entitlement (see vz.entitlements and
//     the sign target in the makefile).
//   - Mixtape images must be raw (nixos.img). QCOW2 is not supported by
//     Apple Virtualization.framework; use the QEMU backend for qcow2 images.
//
// Unlike the QEMU backend, the VM runs in-process as goroutines managed by
// the framework. There is no PID file. In-memory state (the *vz.VirtualMachine
// handle) is held in the live map and is lost if the daemon restarts. After a
// daemon restart, running VMs continue running but the daemon cannot manage
// them until they are destroyed and recreated.
//
// SSH access is provided by a host-side TCP proxy goroutine that bridges a
// loopback port to the guest via the virtio-socket device, replacing the
// QEMU hostfwd mechanism.
//
// Shared directories use virtiofs (VirtioFileSystemDeviceConfiguration),
// not virtio-9p. The StereOS guest must have CONFIG_VIRTIO_FS enabled.
type AppleVirtBackend struct {
	baseDir string

	mu   sync.RWMutex
	live map[string]*appleVirtVM // name → running VM handle
}

// appleVirtVM holds the in-process runtime state for a single running VM.
type appleVirtVM struct {
	vm           *vz.VirtualMachine
	socketDevice *vz.VirtioSocketDevice
	sshPort      int
	cancelProxy  context.CancelFunc // stops the SSH proxy goroutine
}

// NewAppleVirtBackend creates an AppleVirtBackend rooted at baseDir.
func NewAppleVirtBackend(baseDir string) *AppleVirtBackend {
	return &AppleVirtBackend{
		baseDir: baseDir,
		live:    make(map[string]*appleVirtVM),
	}
}

// Up creates and starts a new sandbox VM from the given instance
// configuration. It copies the mixtape raw image, sets up EFI vars,
// generates a persistent machine identity, boots the VM, starts the SSH
// proxy, and runs post-boot provisioning (stereosd handshake, secret
// injection, directory mounting).
func (a *AppleVirtBackend) Up(ctx context.Context, inst *Instance) error {
	if inst.Config == nil {
		return fmt.Errorf("instance %q has no configuration", inst.Name)
	}
	cfg := inst.Config

	// Resolve and validate the mixtape image path
	imagePath, err := ResolveMixtapePath(a.baseDir, cfg.Mixtape)
	if err != nil {
		return fmt.Errorf("resolving mixtape: %w", err)
	}
	if strings.HasSuffix(imagePath, ".qcow2") {
		return fmt.Errorf(
			"Apple Virtualization.framework requires a raw disk image (nixos.img); "+
				"mixtape %q only has a qcow2 image at %s\n\n"+
				"Use the QEMU backend for qcow2 images: MB_BACKEND=qemu mb up",
			cfg.Mixtape, imagePath,
		)
	}

	// Create VM directory
	vmDir := filepath.Join(VMsDir(a.baseDir), inst.Name)
	if _, err := os.Stat(vmDir); err == nil {
		return fmt.Errorf("sandbox %q already exists at %s", inst.Name, vmDir)
	}
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		return fmt.Errorf("creating VM directory: %w", err)
	}
	inst.Dir = vmDir

	// Copy and resize the raw disk image
	copyStart := time.Now()
	if err := copyRawImage(imagePath, inst.DiskPath()); err != nil {
		_ = os.RemoveAll(vmDir)
		return fmt.Errorf("copying disk image: %w", err)
	}
	log.Printf("[applevirt] Up: disk copy took %s", time.Since(copyStart).Round(time.Millisecond))
	diskBytes, err := parseSizeBytes(cfg.Resources.Disk)
	if err != nil {
		_ = os.RemoveAll(vmDir)
		return fmt.Errorf("parsing disk size %q: %w", cfg.Resources.Disk, err)
	}
	if err := resizeRawImageGo(inst.DiskPath(), diskBytes); err != nil {
		_ = os.RemoveAll(vmDir)
		return fmt.Errorf("resizing disk image: %w", err)
	}

	// Resolve kernel artifacts for direct kernel boot (if available).
	// When kernel artifacts exist in the mixtape, we use VZLinuxBootLoader
	// which bypasses EFI entirely — no variable store or machine identity
	// is needed. This mirrors the QEMU direct kernel boot path from 106bae9.
	kernelArtifacts := ResolveKernelArtifacts(a.baseDir, cfg.Mixtape)

	var efiVarStore *vz.EFIVariableStore
	var machineIDBytes []byte
	if kernelArtifacts == nil {
		// EFI boot fallback: create a fresh EFI variable store
		efiVarStore, err = vz.NewEFIVariableStore(
			inst.EFIVarsPath(),
			vz.WithCreatingEFIVariableStore(),
		)
		if err != nil {
			_ = os.RemoveAll(vmDir)
			return fmt.Errorf("creating EFI variable store: %w", err)
		}

		// Generate a persistent machine identity so the guest sees consistent
		// hardware (same MAC address, EFI NVRAM variables) across reboots.
		machineID, err := vz.NewGenericMachineIdentifier()
		if err != nil {
			_ = os.RemoveAll(vmDir)
			return fmt.Errorf("generating machine identifier: %w", err)
		}
		machineIDBytes = machineID.DataRepresentation()
	}

	// Save jcard.toml into the VM directory
	if err := saveJcard(vmDir, cfg); err != nil {
		_ = os.RemoveAll(vmDir)
		return fmt.Errorf("saving jcard config: %w", err)
	}

	// Boot the VM
	if err := a.boot(ctx, inst, cfg, efiVarStore, machineIDBytes, kernelArtifacts); err != nil {
		// The VM directory was created — leave it for debugging unless
		// the VM itself never started (in which case the live map is empty).
		a.mu.RLock()
		_, started := a.live[inst.Name]
		a.mu.RUnlock()
		if !started {
			_ = os.RemoveAll(vmDir)
		}
		return err
	}

	return nil
}

// Start re-boots an existing stopped sandbox. The disk and EFI vars from a
// previous Up call are reused. The saved machine identity is restored from
// state.json for hardware consistency.
func (a *AppleVirtBackend) Start(ctx context.Context, inst *Instance) error {
	if inst.Dir == "" {
		inst.Dir = filepath.Join(VMsDir(a.baseDir), inst.Name)
	}
	if _, err := os.Stat(inst.Dir); os.IsNotExist(err) {
		return fmt.Errorf("sandbox %q has no VM directory at %s", inst.Name, inst.Dir)
	}

	cfg := inst.Config
	if cfg == nil {
		var err error
		cfg, err = config.Load(inst.JcardPath())
		if err != nil {
			return fmt.Errorf("loading saved config for %q: %w", inst.Name, err)
		}
		inst.Config = cfg
	}

	// Load the saved machine identity
	state, err := loadState(inst.Dir)
	if err != nil {
		return fmt.Errorf("loading state for %q: %w", inst.Name, err)
	}

	// Resolve kernel artifacts for direct kernel boot (if available)
	kernelArtifacts := ResolveKernelArtifacts(a.baseDir, cfg.Mixtape)

	var efiVarStore *vz.EFIVariableStore
	if kernelArtifacts == nil {
		// Reconstruct the EFI variable store from the existing file
		efiVarStore, err = vz.NewEFIVariableStore(inst.EFIVarsPath())
		if err != nil {
			return fmt.Errorf("loading EFI variable store: %w", err)
		}
	}

	return a.boot(ctx, inst, cfg, efiVarStore, state.PlatformData, kernelArtifacts)
}

// boot is the shared boot sequence for Up and Start. It allocates the SSH
// proxy port, builds the VM configuration, starts the VirtualMachine,
// launches the SSH proxy goroutine, saves state, and runs post-boot
// provisioning.
//
// When kernelArtifacts is non-nil, direct kernel boot is used (VZLinuxBootLoader)
// and efiVarStore/machineIDBytes may be nil. Otherwise EFI boot is used.
func (a *AppleVirtBackend) boot(
	ctx context.Context,
	inst *Instance,
	cfg *config.JcardConfig,
	efiVarStore *vz.EFIVariableStore,
	machineIDBytes []byte,
	kernelArtifacts *KernelArtifacts,
) error {
	// Allocate a loopback port for the SSH proxy
	sshPort, err := allocatePort()
	if err != nil {
		return fmt.Errorf("allocating SSH port: %w", err)
	}
	inst.SSHPort = sshPort

	// The vsock port for the stereosd control plane is the well-known
	// guest-side port — no host TCP forwarding needed with native vsock.
	inst.VsockPort = vsock.VsockPort

	// Generate ephemeral SSH keypair for this sandbox
	sshKeyPath, sshPubKey, err := ssh.GenerateKeyPair(inst.Dir, fmt.Sprintf("mb-%s", inst.Name))
	if err != nil {
		return fmt.Errorf("generating SSH keypair: %w", err)
	}
	inst.SSHKeyPath = sshKeyPath
	inst.sshPublicKey = sshPubKey

	// Build the VirtualMachineConfiguration
	vmConfig, err := a.buildVMConfig(inst, cfg, efiVarStore, machineIDBytes, kernelArtifacts)
	if err != nil {
		return fmt.Errorf("building VM configuration: %w", err)
	}

	// Instantiate and start the virtual machine
	log.Printf("[applevirt] boot: creating virtual machine for %q", inst.Name)
	vzVM, err := vz.NewVirtualMachine(vmConfig)
	if err != nil {
		return fmt.Errorf("creating virtual machine: %w", err)
	}
	log.Printf("[applevirt] boot: starting virtual machine...")
	if err := vzVM.Start(); err != nil {
		return fmt.Errorf("starting virtual machine: %w", err)
	}

	// Wait for the VM to reach the Running state
	if err := waitForVMRunning(ctx, vzVM, 30*time.Second); err != nil {
		_ = vzVM.Stop()
		return fmt.Errorf("waiting for VM to start: %w", err)
	}
	log.Printf("[applevirt] boot: VM is running (state=%v)", vzVM.State())

	// Obtain the virtio-socket device for stereosd and SSH proxy
	socketDevices := vzVM.SocketDevices()
	log.Printf("[applevirt] boot: found %d socket device(s)", len(socketDevices))
	if len(socketDevices) == 0 {
		_ = vzVM.Stop()
		return fmt.Errorf("virtual machine has no virtio-socket device")
	}
	socketDevice := socketDevices[0]

	// Record the VM as running before starting the proxy and post-boot so
	// that the daemon can track the partial state if either step fails.
	inst.VMState = StateRunning

	// Start the SSH proxy goroutine
	proxyCtx, cancelProxy := context.WithCancel(context.Background())
	go startSSHProxy(proxyCtx, socketDevice, sshPort)

	avVM := &appleVirtVM{
		vm:           vzVM,
		socketDevice: socketDevice,
		sshPort:      sshPort,
		cancelProxy:  cancelProxy,
	}
	a.mu.Lock()
	a.live[inst.Name] = avVM
	a.mu.Unlock()

	// Persist state
	stateFile := &StateFile{
		Name:         inst.Name,
		CreatedAt:    time.Now().UTC(),
		Mixtape:      cfg.Mixtape,
		CPUs:         cfg.Resources.CPUs,
		Memory:       cfg.Resources.Memory,
		Disk:         cfg.Resources.Disk,
		NetworkMode:  cfg.Network.Mode,
		SSHPort:      sshPort,
		VsockPort:    vsock.VsockPort,
		SSHKeyPath:   sshKeyPath,
		Backend:      backendNameAppleVirt,
		PlatformData: machineIDBytes,
	}
	if err := saveState(inst.Dir, stateFile); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	// Post-boot provisioning: stereosd handshake, secrets, mounts
	log.Printf("[applevirt] boot: starting post-boot provisioning (serial log: %s)", inst.SerialLogPath())
	if err := a.postBoot(ctx, inst, cfg, socketDevice); err != nil {
		// VM is running; leave it alive so the user can debug.
		return fmt.Errorf("post-boot provisioning: %w", err)
	}

	return nil
}

// buildVMConfig assembles a vz.VirtualMachineConfiguration for the given
// instance and jcard config. When kernelArtifacts is non-nil, direct kernel
// boot is used via VZLinuxBootLoader (macOS 11+). Otherwise, EFI boot is
// used via VZEFIBootLoader (macOS 13+).
func (a *AppleVirtBackend) buildVMConfig(
	inst *Instance,
	cfg *config.JcardConfig,
	efiVarStore *vz.EFIVariableStore,
	machineIDBytes []byte,
	kernelArtifacts *KernelArtifacts,
) (*vz.VirtualMachineConfiguration, error) {
	// --- Boot loader ---
	var bootLoader vz.BootLoader
	if kernelArtifacts != nil {
		log.Printf("[applevirt] buildVMConfig: using direct kernel boot (kernel=%s)", kernelArtifacts.Kernel)
		// Direct kernel boot via VZLinuxBootLoader (macOS 11+).
		// This mirrors the QEMU -kernel/-initrd/-append path and
		// eliminates EFI/GRUB overhead for faster boot.
		var linuxOpts []vz.LinuxBootLoaderOption
		linuxOpts = append(linuxOpts, vz.WithInitrd(kernelArtifacts.Initrd))
		if kernelArtifacts.Cmdline != "" {
			// Strip "quiet" and bump loglevel for boot debugging.
			// TODO: remove once Apple Virt boot is stable.
			cmdline := kernelArtifacts.Cmdline
			cmdline = strings.ReplaceAll(cmdline, "quiet ", "")
			cmdline = strings.ReplaceAll(cmdline, "loglevel=0", "loglevel=7")
			log.Printf("[applevirt] buildVMConfig: cmdline=%s", cmdline)
			linuxOpts = append(linuxOpts, vz.WithCommandLine(cmdline))
		}
		lb, err := vz.NewLinuxBootLoader(kernelArtifacts.Kernel, linuxOpts...)
		if err != nil {
			return nil, fmt.Errorf("creating Linux boot loader: %w", err)
		}
		bootLoader = lb
	} else {
		log.Printf("[applevirt] buildVMConfig: using EFI boot (no kernel artifacts found)")
		// EFI firmware boot (fallback, macOS 13+)
		lb, err := vz.NewEFIBootLoader(
			vz.WithEFIVariableStore(efiVarStore),
		)
		if err != nil {
			return nil, fmt.Errorf("creating EFI boot loader (requires macOS 13+): %w", err)
		}
		bootLoader = lb
	}

	// --- Memory ---
	memBytes, err := parseSizeBytes(cfg.Resources.Memory)
	if err != nil {
		return nil, fmt.Errorf("parsing memory size %q: %w", cfg.Resources.Memory, err)
	}

	// --- Root configuration object ---
	vmConfig, err := vz.NewVirtualMachineConfiguration(
		bootLoader,
		uint(cfg.Resources.CPUs),
		uint64(memBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("creating VM configuration: %w", err)
	}

	// --- Disk (raw only) ---
	// Use cached + fsync mode to ensure guest flush/sync operations are
	// honored by the Virtualization.framework. Without explicit fsync mode,
	// the framework may use DiskImageSynchronizationModeNone which silently
	// drops guest sync calls. This causes ext4 metadata corruption during
	// boot-time operations like growpart + resize2fs that rely on write
	// ordering and fsync for journal/checksum consistency.
	diskAttachment, err := vz.NewDiskImageStorageDeviceAttachmentWithCacheAndSync(
		inst.DiskPath(),
		false, // read-write
		vz.DiskImageCachingModeCached,
		vz.DiskImageSynchronizationModeFsync,
	)
	if err != nil {
		return nil, fmt.Errorf("creating disk attachment for %s: %w", inst.DiskPath(), err)
	}
	blockDev, err := vz.NewVirtioBlockDeviceConfiguration(diskAttachment)
	if err != nil {
		return nil, fmt.Errorf("creating block device configuration: %w", err)
	}
	vmConfig.SetStorageDevicesVirtualMachineConfiguration(
		[]vz.StorageDeviceConfiguration{blockDev},
	)

	// --- Network (NAT) ---
	// Port forwarding is not available through NATNetworkDeviceAttachment.
	// SSH is proxied via the virtio-socket device instead (see startSSHProxy).
	// User-configured port forwards from jcard.toml are not yet supported on
	// this backend; they require a userspace network stack.
	natAttachment, err := vz.NewNATNetworkDeviceAttachment()
	if err != nil {
		return nil, fmt.Errorf("creating NAT network attachment: %w", err)
	}
	netDev, err := vz.NewVirtioNetworkDeviceConfiguration(natAttachment)
	if err != nil {
		return nil, fmt.Errorf("creating network device configuration: %w", err)
	}
	vmConfig.SetNetworkDevicesVirtualMachineConfiguration(
		[]*vz.VirtioNetworkDeviceConfiguration{netDev},
	)

	// --- Virtio socket device (stereosd control plane + SSH proxy) ---
	socketDevConfig, err := vz.NewVirtioSocketDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("creating socket device configuration: %w", err)
	}
	vmConfig.SetSocketDevicesVirtualMachineConfiguration(
		[]vz.SocketDeviceConfiguration{socketDevConfig},
	)

	// --- Serial console → serial.log ---
	serialAttachment, err := vz.NewFileSerialPortAttachment(
		inst.SerialLogPath(),
		true, // append mode
	)
	if err != nil {
		return nil, fmt.Errorf("creating serial port attachment: %w", err)
	}
	serialPort, err := vz.NewVirtioConsoleDeviceSerialPortConfiguration(serialAttachment)
	if err != nil {
		return nil, fmt.Errorf("creating serial port configuration: %w", err)
	}
	vmConfig.SetSerialPortsVirtualMachineConfiguration(
		[]*vz.VirtioConsoleDeviceSerialPortConfiguration{serialPort},
	)

	// --- Entropy device ---
	entropy, err := vz.NewVirtioEntropyDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("creating entropy device configuration: %w", err)
	}
	vmConfig.SetEntropyDevicesVirtualMachineConfiguration(
		[]*vz.VirtioEntropyDeviceConfiguration{entropy},
	)

	// --- Shared directories (virtiofs, macOS 12+) ---
	// Each [[shared]] entry in jcard.toml becomes a VirtioFileSystemDevice
	// with the tag "share0", "share1", etc. The guest mounts these as
	// virtiofs (not 9p); StereOS must have CONFIG_VIRTIO_FS enabled.
	if len(cfg.Shared) > 0 {
		var dirSharingDevices []vz.DirectorySharingDeviceConfiguration
		for i, mount := range cfg.Shared {
			tag := fmt.Sprintf("share%d", i)

			sharedDir, err := vz.NewSharedDirectory(mount.Host, mount.ReadOnly)
			if err != nil {
				return nil, fmt.Errorf(
					"creating shared directory for %q (share%d): %w",
					mount.Host, i, err,
				)
			}
			share, err := vz.NewSingleDirectoryShare(sharedDir)
			if err != nil {
				return nil, fmt.Errorf("creating directory share for share%d: %w", i, err)
			}
			fsDev, err := vz.NewVirtioFileSystemDeviceConfiguration(tag)
			if err != nil {
				return nil, fmt.Errorf("creating fs device configuration for %s: %w", tag, err)
			}
			fsDev.SetDirectoryShare(share)
			dirSharingDevices = append(dirSharingDevices, fsDev)
		}
		vmConfig.SetDirectorySharingDevicesVirtualMachineConfiguration(dirSharingDevices)
	}

	// --- Platform (machine identity, macOS 13+) ---
	// Restoring the saved machine identifier gives the guest a stable hardware
	// identity across reboots (consistent MAC address, EFI NVRAM variables).
	var platformOpts []vz.GenericPlatformConfigurationOption
	if len(machineIDBytes) > 0 {
		machineID, err := vz.NewGenericMachineIdentifierWithData(machineIDBytes)
		if err != nil {
			return nil, fmt.Errorf("restoring machine identifier: %w", err)
		}
		platformOpts = append(platformOpts, vz.WithGenericMachineIdentifier(machineID))
	}
	platformConfig, err := vz.NewGenericPlatformConfiguration(platformOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating platform configuration (requires macOS 12+): %w", err)
	}
	vmConfig.SetPlatformVirtualMachineConfiguration(platformConfig)

	// Validate the full configuration before returning
	if ok, err := vmConfig.Validate(); !ok || err != nil {
		if err != nil {
			return nil, fmt.Errorf("invalid VM configuration: %w", err)
		}
		return nil, fmt.Errorf("invalid VM configuration (unknown reason)")
	}

	return vmConfig, nil
}

// postBoot handles post-boot provisioning: waits for stereosd to become
// ready, sends the jcard.toml configuration, injects secrets, and mounts
// shared directories. It is identical to the QEMU postBoot except it
// connects via the virtio-socket device and uses "virtiofs" as the fs type.
func (a *AppleVirtBackend) postBoot(
	ctx context.Context,
	inst *Instance,
	cfg *config.JcardConfig,
	socketDevice *vz.VirtioSocketDevice,
) error {
	// Build a transport that dials stereosd via the virtio-socket device.
	transport := newVzSocketTransport(socketDevice, vsock.VsockPort, "Apple Virt postBoot")

	// Connect to stereosd with aggressive exponential backoff.
	// The guest typically boots in ~3.5s; we start trying immediately with
	// short intervals (100ms, 200ms, 400ms, 800ms) then cap at 1s, so we
	// connect within ~500ms of stereosd becoming ready.
	log.Printf("[applevirt] postBoot: connecting to stereosd via %s", transport)
	var client *vsock.Client
	var connErr error
	connectStart := time.Now()
	deadline := time.Now().Add(120 * time.Second)
	backoff := 100 * time.Millisecond
	const maxBackoff = 1 * time.Second
	attempts := 0
	for time.Now().Before(deadline) {
		attempts++
		client, connErr = vsock.Connect(transport, 2*time.Second)
		if connErr == nil {
			log.Printf("[applevirt] postBoot: connected to stereosd after %d attempts (%s)",
				attempts, time.Since(connectStart).Round(time.Millisecond))
			break
		}
		if attempts <= 5 || attempts%10 == 0 {
			log.Printf("[applevirt] postBoot: connect attempt %d failed: %v", attempts, connErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
	if client == nil {
		log.Printf("[applevirt] postBoot: gave up after %d attempts (%s): %v",
			attempts, time.Since(connectStart).Round(time.Millisecond), connErr)
		return fmt.Errorf("could not connect to stereosd via %s after 120s: %w",
			transport, connErr)
	}
	defer func() { _ = client.Close() }()

	// Wait for ready state with tight polling — stereosd is usually ready
	// immediately after accepting the connection.
	log.Printf("[applevirt] postBoot: waiting for stereosd ready state...")
	readyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := client.WaitForReady(readyCtx, 200*time.Millisecond); err != nil {
		// Log the last health check result for debugging
		health, healthErr := client.GetHealth(ctx)
		if healthErr != nil {
			log.Printf("[applevirt] postBoot: WaitForReady failed: %v (last GetHealth error: %v)", err, healthErr)
		} else {
			log.Printf("[applevirt] postBoot: WaitForReady failed: %v (last health state: %q, uptime: %ds)", err, health.State, health.Uptime)
		}
		return fmt.Errorf("waiting for stereosd ready: %w", err)
	}
	log.Printf("[applevirt] postBoot: stereosd is ready")

	// Send jcard.toml content to the guest for agentd
	cfgBytes, err := config.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config for guest: %w", err)
	}
	if err := client.SetConfig(ctx, string(cfgBytes)); err != nil {
		return fmt.Errorf("setting guest config: %w", err)
	}

	// Inject ephemeral SSH public key for the admin user
	if inst.sshPublicKey != "" {
		if err := client.InjectSSHKey(ctx, "admin", inst.sshPublicKey); err != nil {
			return fmt.Errorf("injecting SSH key: %w", err)
		}
	}

	// Inject secrets
	for name, value := range cfg.Secrets {
		if err := client.InjectSecret(ctx, name, value); err != nil {
			return fmt.Errorf("injecting secret %q: %w", name, err)
		}
	}

	// Mount shared directories. Apple Virt uses virtiofs, not 9p.
	for i, shared := range cfg.Shared {
		tag := fmt.Sprintf("share%d", i)
		if err := client.Mount(ctx, tag, shared.Guest, "virtiofs", shared.ReadOnly); err != nil {
			return fmt.Errorf("mounting %q at %q: %w", shared.Host, shared.Guest, err)
		}
	}

	return nil
}

// Boot is the exported entry point for vmhost processes. It loads the
// config, allocates ports, boots the VM, starts the SSH proxy, and runs
// post-boot provisioning. The VM directory and disk must already exist
// (created by the daemon via PrepareAppleVirtDisk).
func (a *AppleVirtBackend) Boot(ctx context.Context, inst *Instance) error {
	if inst.Dir == "" {
		inst.Dir = filepath.Join(VMsDir(a.baseDir), inst.Name)
	}

	cfg := inst.Config
	if cfg == nil {
		var err error
		cfg, err = config.Load(inst.JcardPath())
		if err != nil {
			return fmt.Errorf("loading config for %q: %w", inst.Name, err)
		}
		inst.Config = cfg
	}

	// Load the saved state for machine identity
	state, err := loadState(inst.Dir)
	if err != nil {
		return fmt.Errorf("loading state for %q: %w", inst.Name, err)
	}

	// Resolve kernel artifacts for direct kernel boot
	kernelArtifacts := ResolveKernelArtifacts(a.baseDir, cfg.Mixtape)

	var efiVarStore *vz.EFIVariableStore
	if kernelArtifacts == nil {
		efiVarStore, err = vz.NewEFIVariableStore(inst.EFIVarsPath())
		if err != nil {
			return fmt.Errorf("loading EFI variable store: %w", err)
		}
	}

	return a.boot(ctx, inst, cfg, efiVarStore, state.PlatformData, kernelArtifacts)
}

// WaitVM blocks until the VM exits. Returns nil on clean exit.
func (a *AppleVirtBackend) WaitVM(name string) error {
	a.mu.RLock()
	avVM := a.live[name]
	a.mu.RUnlock()

	if avVM == nil {
		return fmt.Errorf("VM %q not found in live map", name)
	}

	// Poll VM state until stopped or errored
	for {
		switch avVM.vm.State() {
		case vz.VirtualMachineStateStopped:
			return nil
		case vz.VirtualMachineStateError:
			return fmt.Errorf("VM entered error state")
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// Down gracefully stops the VM. It first asks stereosd to shut down via
// vsock, then sends an ACPI power-off signal via RequestStop, and finally
// force-stops if the VM has not halted within the timeout.
func (a *AppleVirtBackend) Down(ctx context.Context, inst *Instance, timeout time.Duration) error {
	a.mu.RLock()
	avVM := a.live[inst.Name]
	a.mu.RUnlock()

	if avVM == nil {
		inst.VMState = StateStopped
		return nil
	}

	// Ask stereosd to shut down gracefully
	_ = a.controlPlaneShutdown(ctx, avVM.socketDevice)

	// Give stereosd a moment to initiate the shutdown sequence
	if waitForVMStopped(avVM.vm, timeout) {
		return a.cleanupLive(inst, avVM)
	}

	// ACPI power button
	if _, err := avVM.vm.RequestStop(); err == nil {
		if waitForVMStopped(avVM.vm, 15*time.Second) {
			return a.cleanupLive(inst, avVM)
		}
	}

	// Force stop
	return a.ForceDown(ctx, inst)
}

// ForceDown immediately terminates the VM.
func (a *AppleVirtBackend) ForceDown(_ context.Context, inst *Instance) error {
	a.mu.RLock()
	avVM := a.live[inst.Name]
	a.mu.RUnlock()

	if avVM == nil {
		inst.VMState = StateStopped
		return nil
	}

	_ = avVM.vm.Stop()
	return a.cleanupLive(inst, avVM)
}

// cleanupLive cancels the SSH proxy, removes the VM from the live map,
// and marks the instance as stopped.
func (a *AppleVirtBackend) cleanupLive(inst *Instance, avVM *appleVirtVM) error {
	avVM.cancelProxy()
	a.mu.Lock()
	delete(a.live, inst.Name)
	a.mu.Unlock()
	inst.VMState = StateStopped
	return nil
}

// Destroy stops the VM (if running) and removes all its on-disk resources.
func (a *AppleVirtBackend) Destroy(ctx context.Context, inst *Instance) error {
	status, _ := a.Status(ctx, inst)
	if status == StateRunning {
		if err := a.Down(ctx, inst, 10*time.Second); err != nil {
			_ = a.ForceDown(ctx, inst)
		}
	}
	if err := os.RemoveAll(inst.Dir); err != nil {
		return fmt.Errorf("removing VM directory: %w", err)
	}
	return nil
}

// Status returns the current state of the VM.
// If the VM is not in the live map (e.g. after a daemon restart), it reports
// StateStopped because we cannot reconnect to an orphaned vz.VirtualMachine.
func (a *AppleVirtBackend) Status(_ context.Context, inst *Instance) (State, error) {
	a.mu.RLock()
	avVM := a.live[inst.Name]
	a.mu.RUnlock()

	if avVM == nil {
		return StateStopped, nil
	}

	switch avVM.vm.State() {
	case vz.VirtualMachineStateRunning,
		vz.VirtualMachineStateStarting:
		return StateRunning, nil
	case vz.VirtualMachineStateError:
		return StateError, nil
	default:
		return StateStopped, nil
	}
}

// List returns all VM instances owned by the Apple Virt backend by scanning
// the vms directory. VMs with a different backend field in state.json are
// skipped so that QEMU and Apple Virt VMs can coexist in the same base dir.
func (a *AppleVirtBackend) List(ctx context.Context) ([]*Instance, error) {
	vmsDir := VMsDir(a.baseDir)
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

		state, err := loadState(filepath.Join(vmsDir, entry.Name()))
		if err != nil {
			continue
		}
		if state.Backend != backendNameAppleVirt {
			continue
		}

		inst, err := a.LoadInstance(entry.Name())
		if err != nil {
			continue
		}
		status, _ := a.Status(ctx, inst)
		inst.VMState = status
		instances = append(instances, inst)
	}

	return instances, nil
}

// LoadInstance reads a persisted Apple Virt VM instance by name.
func (a *AppleVirtBackend) LoadInstance(name string) (*Instance, error) {
	vmDir := filepath.Join(VMsDir(a.baseDir), name)
	state, err := loadState(vmDir)
	if err != nil {
		return nil, fmt.Errorf("loading VM %q: %w", name, err)
	}

	inst := &Instance{
		Name:       state.Name,
		Dir:        vmDir,
		SSHPort:    state.SSHPort,
		VsockPort:  state.VsockPort,
		SSHKeyPath: state.SSHKeyPath,
	}
	return inst, nil
}

// newVzSocketTransport creates a vsock.FuncTransport that dials the guest
// via the virtio-socket device with a per-attempt timeout. The label is used
// for logging and String() output.
func newVzSocketTransport(socketDevice *vz.VirtioSocketDevice, port int, label string) *vsock.FuncTransport {
	return &vsock.FuncTransport{
		Label: label,
		DialFn: func(timeout time.Duration) (net.Conn, error) {
			type result struct {
				conn net.Conn
				err  error
			}
			ch := make(chan result, 1)
			go func() {
				log.Printf("[applevirt] vsock dial (%s): connecting to guest port %d...", label, port)
				conn, err := socketDevice.Connect(uint32(port))
				if err != nil {
					log.Printf("[applevirt] vsock dial (%s): Connect() error: %v", label, err)
				} else {
					log.Printf("[applevirt] vsock dial (%s): Connect() succeeded, remote=%v", label, conn.RemoteAddr())
				}
				ch <- result{conn, err}
			}()
			select {
			case r := <-ch:
				return r.conn, r.err
			case <-time.After(timeout):
				return nil, fmt.Errorf("vsock connect to port %d timed out after %s",
					port, timeout)
			}
		},
	}
}

// controlPlaneShutdown sends a shutdown message to stereosd via the
// virtio-socket device.
func (a *AppleVirtBackend) controlPlaneShutdown(ctx context.Context, socketDevice *vz.VirtioSocketDevice) error {
	transport := newVzSocketTransport(socketDevice, vsock.VsockPort, "Apple Virt shutdown")
	client, err := vsock.Connect(transport, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	return client.Shutdown(ctx, "mb down")
}

// startSSHProxy listens on a loopback TCP port and proxies each connection
// to the guest's SSH daemon (port 22) via the virtio-socket device.
// This replaces QEMU's hostfwd=tcp::<sshPort>-:22 mechanism.
// The goroutine runs until ctx is cancelled (typically when the VM is stopped).
func startSSHProxy(ctx context.Context, socketDevice *vz.VirtioSocketDevice, port int) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go proxySSHConn(conn, socketDevice)
	}
}

// proxySSHConn bridges a single TCP connection to the guest's SSH daemon
// through the virtio-socket device.
func proxySSHConn(conn net.Conn, socketDevice *vz.VirtioSocketDevice) {
	defer func() { _ = conn.Close() }()

	vsockConn, err := socketDevice.Connect(22)
	if err != nil {
		return
	}
	defer func() { _ = vsockConn.Close() }()

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(vsockConn, conn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, vsockConn)
		done <- struct{}{}
	}()
	// Close both sides when either direction finishes (EOF or error)
	<-done
}

// waitForVMRunning blocks until the VM reaches VirtualMachineStateRunning or
// the context / timeout expires.
func waitForVMRunning(ctx context.Context, vzVM *vz.VirtualMachine, timeout time.Duration) error {
	timeoutCh := time.After(timeout)
	stateCh := vzVM.StateChangedNotify()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeoutCh:
			return fmt.Errorf("VM did not reach running state within %s", timeout)
		case state, ok := <-stateCh:
			if !ok {
				return fmt.Errorf("VM state channel closed unexpectedly")
			}
			switch state {
			case vz.VirtualMachineStateRunning:
				return nil
			case vz.VirtualMachineStateError:
				return fmt.Errorf("VM entered error state during start")
			case vz.VirtualMachineStateStopped:
				return fmt.Errorf("VM stopped unexpectedly during start")
			}
		}
	}
}

// waitForVMStopped polls the VM state until it is stopped or timeout elapses.
// Returns true if the VM stopped within the timeout.
func waitForVMStopped(vzVM *vz.VirtualMachine, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		<-ticker.C
		switch vzVM.State() {
		case vz.VirtualMachineStateStopped:
			return true
		case vz.VirtualMachineStateStopping:
			continue // transient; re-poll on next tick
		case vz.VirtualMachineStateError:
			return true // treat error as effectively stopped
		}
	}
	return false
}
