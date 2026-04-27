//go:build linux && arm64

package vmhostcmder

import (
	"context"
	"fmt"
	"log"

	"github.com/papercomputeco/masterblaster/pkg/vm"
	"github.com/papercomputeco/masterblaster/pkg/vmhost"
)

func bootAppleVirt(_ context.Context, _ string, _ *vm.Instance, _ *log.Logger) (vmhost.VMController, error) {
	return nil, fmt.Errorf("apple Virtualization.framework is only available on macOS/Apple Silicon")
}

// getPlatformConfig returns the QEMU platform configuration for linux/arm64.
// Delegates to pkg/vm so the config lives in exactly one place.
func getPlatformConfig(_ string) *vm.QEMUPlatformConfig {
	return vm.LinuxARM64PlatformConfig()
}
