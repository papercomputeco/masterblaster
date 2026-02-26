package vm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PrepareQEMUDisk creates the VM directory and sets up the disk image for
// a QEMU-backed VM. This runs in the daemon before spawning the vmhost
// process. It handles:
//   - Creating the VM directory
//   - Resolving the mixtape image
//   - Creating qcow2 overlay or copying raw image
//   - Initializing EFI vars (if needed)
//   - Saving jcard.toml
//   - Writing initial state.json
//
// The inst.Dir field is set on the instance upon return.
func PrepareQEMUDisk(baseDir string, inst *Instance, platform *QEMUPlatformConfig) error {
	if inst.Config == nil {
		return fmt.Errorf("instance %q has no configuration", inst.Name)
	}
	cfg := inst.Config

	// Resolve the mixtape image
	imagePath, err := ResolveMixtapePath(baseDir, cfg.Mixtape)
	if err != nil {
		return fmt.Errorf("resolving mixtape: %w", err)
	}

	// Create VM directory
	vmDir := filepath.Join(VMsDir(baseDir), inst.Name)
	if _, err := os.Stat(vmDir); err == nil {
		return fmt.Errorf("sandbox %q already exists at %s", inst.Name, vmDir)
	}
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		return fmt.Errorf("creating VM directory: %w", err)
	}
	inst.Dir = vmDir

	// Resolve kernel artifacts for direct kernel boot (if available)
	var kernelArtifacts *KernelArtifacts
	if platform.DirectKernelBoot {
		kernelArtifacts = ResolveKernelArtifacts(baseDir, cfg.Mixtape)
	}

	// Determine image format and create disk
	if strings.HasSuffix(imagePath, ".qcow2") {
		if err := createQCOWOverlay(imagePath, inst.QCOWDiskPath(), convertSizeForQEMU(cfg.Resources.Disk)); err != nil {
			_ = os.RemoveAll(vmDir)
			return fmt.Errorf("creating disk overlay: %w", err)
		}
	} else {
		if err := copyRawImage(imagePath, inst.DiskPath()); err != nil {
			_ = os.RemoveAll(vmDir)
			return fmt.Errorf("copying disk image: %w", err)
		}
		if err := resizeRawImage(inst.DiskPath(), convertSizeForQEMU(cfg.Resources.Disk)); err != nil {
			_ = os.RemoveAll(vmDir)
			return fmt.Errorf("resizing disk image: %w", err)
		}
	}

	// Initialize EFI vars if needed (skipped with direct kernel boot)
	if kernelArtifacts == nil {
		if err := initEFIVars(inst.EFIVarsPath()); err != nil {
			_ = os.RemoveAll(vmDir)
			return fmt.Errorf("initializing EFI vars: %w", err)
		}
	}

	// Save jcard.toml into the VM directory
	if err := saveJcard(vmDir, cfg); err != nil {
		_ = os.RemoveAll(vmDir)
		return fmt.Errorf("saving jcard config: %w", err)
	}

	// Write initial state.json
	stateFile := &StateFile{
		Name:        inst.Name,
		Mixtape:     cfg.Mixtape,
		CPUs:        cfg.Resources.CPUs,
		Memory:      cfg.Resources.Memory,
		Disk:        cfg.Resources.Disk,
		NetworkMode: cfg.Network.Mode,
		Backend:     "qemu",
	}
	if err := saveState(vmDir, stateFile); err != nil {
		_ = os.RemoveAll(vmDir)
		return fmt.Errorf("saving state: %w", err)
	}

	return nil
}

// PrepareAppleVirtDisk creates the VM directory and sets up the disk image
// for an Apple Virtualization.framework-backed VM. This runs in the daemon
// before spawning the vmhost process.
//
// This is a platform-independent stub. The actual implementation is in
// prepare_darwin_arm64.go (build-tagged for darwin/arm64).
// On unsupported platforms, it returns an error.

// CleanupVMDir removes the VM directory. Used by the daemon when vmhost
// spawn fails and the VM directory needs to be cleaned up.
func CleanupVMDir(inst *Instance) {
	if inst.Dir != "" {
		_ = os.RemoveAll(inst.Dir)
	}
}

// ResolveBackend determines the backend type for a VM. Precedence:
//  1. Explicit requestedBackend parameter (from CLI flag or config)
//  2. MB_BACKEND environment variable
//  3. Platform default: "applevirt" on darwin/arm64, "qemu" elsewhere
func ResolveBackend(baseDir string, cfg interface{ GetMixtape() string }, requestedBackend string) string {
	if requestedBackend != "" {
		return requestedBackend
	}
	if env := os.Getenv("MB_BACKEND"); env != "" {
		return env
	}
	return DefaultBackend()
}

// LoadInstanceFromDisk reads a persisted VM instance by name from the vms
// directory, regardless of backend type.
func LoadInstanceFromDisk(baseDir, name string) (*Instance, error) {
	vmDir := filepath.Join(VMsDir(baseDir), name)
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

	// Try to read QEMU PID (if applicable)
	if pidData, err := os.ReadFile(inst.PIDFilePath()); err == nil {
		var pid int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(pidData)), "%d", &pid); err == nil {
			inst.PID = pid
		}
	}

	return inst, nil
}

// LoadStateFromDisk reads the state.json for a named VM.
func LoadStateFromDisk(baseDir, name string) (*StateFile, error) {
	vmDir := filepath.Join(VMsDir(baseDir), name)
	return loadState(vmDir)
}
