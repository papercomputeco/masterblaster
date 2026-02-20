package vm

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MixtapesDir returns the path to the mixtapes directory.
func MixtapesDir(baseDir string) string {
	return filepath.Join(baseDir, "mixtapes")
}

// VMsDir returns the path to the VMs directory.
func VMsDir(baseDir string) string {
	return filepath.Join(baseDir, "vms")
}

// KernelArtifacts holds the resolved paths for direct kernel boot.
// These correspond to the bzImage, initrd, and cmdline files produced
// by the stereOS build (system.build.kernelArtifacts in image.nix).
type KernelArtifacts struct {
	Kernel  string // Path to bzImage
	Initrd  string // Path to initrd
	Cmdline string // Kernel command line (contents of cmdline file)
}

// ResolveKernelArtifacts checks if a mixtape has kernel artifacts for
// direct kernel boot. It looks for a kernel-artifacts/ directory alongside
// the disk image containing bzImage, initrd, and cmdline files.
//
// Returns nil if kernel artifacts are not available (caller should fall
// back to EFI boot).
func ResolveKernelArtifacts(baseDir, mixtape string) *KernelArtifacts {
	mixtapeDir := filepath.Join(MixtapesDir(baseDir), mixtape)

	// Look for kernel-artifacts/ subdirectory within the mixtape dir
	artifactsDir := filepath.Join(mixtapeDir, "kernel-artifacts")
	if _, err := os.Stat(artifactsDir); os.IsNotExist(err) {
		return nil
	}

	kernelPath := filepath.Join(artifactsDir, "bzImage")
	initrdPath := filepath.Join(artifactsDir, "initrd")
	cmdlinePath := filepath.Join(artifactsDir, "cmdline")

	// All three files must exist
	for _, p := range []string{kernelPath, initrdPath, cmdlinePath} {
		if _, err := os.Stat(p); err != nil {
			return nil
		}
	}

	// Read the cmdline file contents
	cmdlineBytes, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return nil
	}

	return &KernelArtifacts{
		Kernel:  kernelPath,
		Initrd:  initrdPath,
		Cmdline: strings.TrimSpace(string(cmdlineBytes)),
	}
}

// ResolveMixtapePath resolves a mixtape name to an image path. For short
// names, it looks in ~/.mb/mixtapes/<name>/. For now we support both raw
// and qcow2 images.
func ResolveMixtapePath(baseDir, mixtape string) (string, error) {
	mixtapeDir := filepath.Join(MixtapesDir(baseDir), mixtape)

	// Try raw image first (preferred for Apple Virt framework)
	rawPath := filepath.Join(mixtapeDir, "nixos.img")
	if _, err := os.Stat(rawPath); err == nil {
		return rawPath, nil
	}

	// Try qcow2 (QEMU)
	qcow2Path := filepath.Join(mixtapeDir, "nixos.qcow2")
	if _, err := os.Stat(qcow2Path); err == nil {
		return qcow2Path, nil
	}

	// Try just the mixtape dir as a single image file
	if info, err := os.Stat(mixtapeDir); err == nil && !info.IsDir() {
		return mixtapeDir, nil
	}

	return "", fmt.Errorf("mixtape %q not found at %s\n\n"+
		"Pull a mixtape first:\n"+
		"  mb mixtapes pull %s\n\n"+
		"Or place a StereOS image at:\n"+
		"  %s/nixos.img\n"+
		"  %s/nixos.qcow2",
		mixtape, mixtapeDir, mixtape, mixtapeDir, mixtapeDir)
}

// createQCOWOverlay creates a qcow2 copy-on-write overlay backed by the
// given base image. The overlay is created at overlayPath with the specified size.
func createQCOWOverlay(baseImage, overlayPath, diskSize string) error {
	qemuImg, err := exec.LookPath("qemu-img")
	if err != nil {
		return fmt.Errorf("qemu-img not found: %w (install QEMU: brew install qemu)", err)
	}

	cmd := exec.Command(qemuImg, "create",
		"-f", "qcow2",
		"-b", baseImage,
		"-F", "qcow2",
		overlayPath,
		diskSize,
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("creating qcow2 overlay: %w", err)
	}
	return nil
}

// copyRawImage copies a raw disk image for use as a writable disk.
// Unlike qcow2 overlays, raw images are used directly so we copy them.
//
// On macOS/APFS, this uses clonefile(2) for a near-instant copy-on-write
// clone. On other platforms, it falls back to streaming io.Copy.
func copyRawImage(src, dst string) error {
	// Try platform-optimized copy first.
	if err := cloneFile(src, dst); err == nil {
		// clonefile(2) preserves source permissions. Mixtape images are
		// typically read-only (0444), but the copy must be writable for
		// resize and VM disk I/O.
		if err := os.Chmod(dst, 0644); err != nil {
			return fmt.Errorf("making cloned image writable: %w", err)
		}
		return nil
	}

	// Fallback: streaming copy.
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source image: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating disk image: %w", err)
	}

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = dstFile.Close()
		return fmt.Errorf("copying disk image: %w", err)
	}
	if err := dstFile.Close(); err != nil {
		return fmt.Errorf("closing disk image: %w", err)
	}
	return nil
}

// resizeRawImage resizes a raw disk image to the given size using qemu-img.
// The size string uses QEMU notation (e.g. "20G", "512M").
func resizeRawImage(imagePath, size string) error {
	qemuImg, err := exec.LookPath("qemu-img")
	if err != nil {
		return fmt.Errorf("qemu-img not found: %w (install QEMU: brew install qemu)", err)
	}

	cmd := exec.Command(qemuImg, "resize", imagePath, size)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("resizing disk image: %w", err)
	}
	return nil
}
