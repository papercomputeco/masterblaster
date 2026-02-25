// Package servecmder provides the serve command for starting the
// Masterblaster daemon.
package servecmder

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/daemon"
	"github.com/papercomputeco/masterblaster/pkg/ui"
)

const serveLongDesc string = `Start the long-lived Masterblaster daemon that manages sandbox VMs.

The daemon listens on ~/.mb/mb.sock for CLI commands and manages all
VM lifecycle operations. Other mb commands communicate with this daemon.

Each VM gets its own vmhost child process that holds the hypervisor handle
and survives daemon restarts. The daemon acts as a multiplexer, spawning
and monitoring vmhost processes.

If the daemon is already running, this command exits with an error.`

const serveShortDesc string = "Start the Masterblaster daemon"

var errDaemonAlreadyRunning = errors.New("daemon is already running")

// NewServeCmd creates the serve command. configDirFn is a function that
// returns the resolved config directory (deferred so flags are parsed first).
func NewServeCmd(configDirFn func() string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: serveShortDesc,
		Long:  serveLongDesc,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd, configDirFn())
		},
	}

	return cmd
}

func runServe(cmd *cobra.Command, baseDir string) error {
	if daemon.IsRunning(baseDir) {
		return errDaemonAlreadyRunning
	}

	d := daemon.New(baseDir)

	ui.Status("Starting Masterblaster daemon...")
	return d.Run(cmd.Context())
}
