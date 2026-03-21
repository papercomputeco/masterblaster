// Package pullcmder provides the top-level "mb pull" command for downloading
// StereOS mixtape images from an OCI registry.
package pullcmder

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/mixtapes"
	"github.com/papercomputeco/masterblaster/pkg/telemetry"
	"github.com/papercomputeco/masterblaster/pkg/ui"
)

const pullLongDesc string = `Pull a StereOS mixtape image from the Paper Compute registry.

Downloads an OCI artifact containing the VM disk image (zstd-compressed),
kernel artifacts (bzImage, initrd, cmdline, init), and mixtape manifest,
then stores them locally in ~/.config/mb/mixtapes/<name>/<tag>/.

The registry hosts an OCI index with both raw and qcow2 format manifests.
The raw format is preferred (for Apple Virtualization.framework); qcow2
is used as a fallback (for QEMU).

The argument is a mixtape reference in the form name[:tag]. Short names are
resolved against the default registry (download.stereos.ai/mixtapes/).
Full OCI references (e.g. myregistry.io/my/repo:tag) are also accepted.

Examples:
  mb pull opencode-mixtape
  mb pull opencode-mixtape:0.1.0
  mb pull download.stereos.ai/mixtapes/opencode-mixtape:latest`

const pullShortDesc string = "Pull a mixtape from the registry"

// NewPullCmd creates the top-level pull command.
func NewPullCmd(configDirFn func() string) *cobra.Command {
	return &cobra.Command{
		Use:   "pull <name[:tag]>",
		Short: pullShortDesc,
		Long:  pullLongDesc,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			telem := telemetry.FromContext(cmd.Context())
			telem.CaptureCommandRun(cmd.CommandPath())
			return runPull(configDirFn(), args[0], telem)
		},
	}
}

func runPull(baseDir, rawRef string, telem *telemetry.PosthogClient) error {
	err := ui.Step(os.Stderr, fmt.Sprintf("Pulling mixtape %q...", rawRef), func() error {
		return mixtapes.Pull(baseDir, rawRef)
	})

	telem.CapturePull(rawRef, err == nil)

	return err
}
