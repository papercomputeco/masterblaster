//go:build linux

package vm

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// cloneFile attempts an instant copy-on-write clone via ioctl(FICLONE).
// This works on btrfs, xfs (with reflink), and other CoW filesystems.
// Returns an error if the filesystem doesn't support cloning, causing
// the caller to fall back to streaming io.Copy.
func cloneFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source for clone: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating destination for clone: %w", err)
	}
	defer dstFile.Close()

	return unix.IoctlFileClone(int(dstFile.Fd()), int(srcFile.Fd()))
}
