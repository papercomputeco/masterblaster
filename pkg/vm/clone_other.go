//go:build !darwin

package vm

import "errors"

// cloneFile is not supported on this platform.
func cloneFile(src, dst string) error {
	return errors.New("clonefile not supported on this platform")
}
