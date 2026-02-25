//go:build darwin && arm64

package vm

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	vz "github.com/Code-Hex/vz/v3"
)

// DefaultBackend returns "applevirt" on darwin/arm64. Apple Virtualization
// framework provides faster boot times and lower overhead than QEMU/HVF.
func DefaultBackend() string {
	return "applevirt"
}

// PrepareAppleVirtDisk creates the VM directory and sets up the disk image
// for an Apple Virtualization.framework-backed VM. This runs in the daemon
// before spawning the vmhost process.
func PrepareAppleVirtDisk(baseDir string, inst *Instance) error {
	if inst.Config == nil {
		return fmt.Errorf("instance %q has no configuration", inst.Name)
	}
	cfg := inst.Config

	// Resolve and validate the mixtape image path
	imagePath, err := ResolveMixtapePath(baseDir, cfg.Mixtape)
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
	vmDir := filepath.Join(VMsDir(baseDir), inst.Name)
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
	log.Printf("[applevirt] prepare: disk copy took %s", time.Since(copyStart).Round(time.Millisecond))

	diskBytes, err := parseSizeBytes(cfg.Resources.Disk)
	if err != nil {
		_ = os.RemoveAll(vmDir)
		return fmt.Errorf("parsing disk size %q: %w", cfg.Resources.Disk, err)
	}
	if err := resizeRawImageGo(inst.DiskPath(), diskBytes); err != nil {
		_ = os.RemoveAll(vmDir)
		return fmt.Errorf("resizing disk image: %w", err)
	}

	// Resolve kernel artifacts for direct kernel boot
	kernelArtifacts := ResolveKernelArtifacts(baseDir, cfg.Mixtape)

	var machineIDBytes []byte
	if kernelArtifacts == nil {
		// EFI boot: create EFI variable store
		_, err = vz.NewEFIVariableStore(
			inst.EFIVarsPath(),
			vz.WithCreatingEFIVariableStore(),
		)
		if err != nil {
			_ = os.RemoveAll(vmDir)
			return fmt.Errorf("creating EFI variable store: %w", err)
		}

		// Generate machine identity for stable hardware across reboots
		machineID, err := vz.NewGenericMachineIdentifier()
		if err != nil {
			_ = os.RemoveAll(vmDir)
			return fmt.Errorf("generating machine identifier: %w", err)
		}
		machineIDBytes = machineID.DataRepresentation()
	}

	// Save jcard.toml
	if err := saveJcard(vmDir, cfg); err != nil {
		_ = os.RemoveAll(vmDir)
		return fmt.Errorf("saving jcard config: %w", err)
	}

	// Write initial state.json
	stateFile := &StateFile{
		Name:         inst.Name,
		Mixtape:      cfg.Mixtape,
		CPUs:         cfg.Resources.CPUs,
		Memory:       cfg.Resources.Memory,
		Disk:         cfg.Resources.Disk,
		NetworkMode:  cfg.Network.Mode,
		Backend:      backendNameAppleVirt,
		PlatformData: machineIDBytes,
	}
	if err := saveState(vmDir, stateFile); err != nil {
		_ = os.RemoveAll(vmDir)
		return fmt.Errorf("saving state: %w", err)
	}

	return nil
}
