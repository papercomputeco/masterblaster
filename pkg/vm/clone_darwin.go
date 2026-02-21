package vm

import (
	"golang.org/x/sys/unix"
)

// cloneFile uses macOS clonefile(2) to create a copy-on-write clone.
// On APFS this completes in microseconds regardless of file size.
// Returns an error if the filesystem doesn't support cloning.
func cloneFile(src, dst string) error {
	return unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW)
}
