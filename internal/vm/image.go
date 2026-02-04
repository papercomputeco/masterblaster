package vm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// imagesDir returns the path to the shared base image cache directory.
func imagesDir(configDir string) string {
	return filepath.Join(configDir, "images")
}

// resolveBaseImagePath maps an image identifier (e.g. "fedora-42") to the
// expected path in the image cache. For the POC, we expect the user to have
// manually downloaded the image.
func resolveBaseImagePath(configDir, image string) string {
	return filepath.Join(imagesDir(configDir), image+"-aarch64.qcow2")
}

// validateBaseImage checks that a base qcow2 image exists at the expected path.
func validateBaseImage(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("base image not found at %s\n\nFor this POC, manually download the Fedora Cloud image:\n"+
				"  mkdir -p %s\n"+
				"  curl -L -o %s \\\n"+
				"    'https://download.fedoraproject.org/pub/fedora/linux/releases/42/Cloud/aarch64/images/Fedora-Cloud-Base-Generic-42-1.1.aarch64.qcow2'",
				path, filepath.Dir(path), path)
		}
		return fmt.Errorf("checking base image: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("base image path %s is a directory, expected a file", path)
	}
	return nil
}

// createOverlay creates a qcow2 copy-on-write overlay backed by the given base image.
// The overlay is created at overlayPath with the specified disk size.
func createOverlay(baseImage, overlayPath, diskSize string) error {
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
