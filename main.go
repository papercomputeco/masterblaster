package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/papercomputeco/masterblaster/pkg/ui"
	"github.com/papercomputeco/masterblaster/pkg/utils"

	applycmder "github.com/papercomputeco/masterblaster/cmd/apply"
	destroycmder "github.com/papercomputeco/masterblaster/cmd/destroy"
	downcmder "github.com/papercomputeco/masterblaster/cmd/down"
	initcmder "github.com/papercomputeco/masterblaster/cmd/init"
	listcmder "github.com/papercomputeco/masterblaster/cmd/list"
	mixtapescmder "github.com/papercomputeco/masterblaster/cmd/mixtapes"
	pullcmder "github.com/papercomputeco/masterblaster/cmd/pull"
	servecmder "github.com/papercomputeco/masterblaster/cmd/serve"
	sshcmder "github.com/papercomputeco/masterblaster/cmd/ssh"
	statuscmder "github.com/papercomputeco/masterblaster/cmd/status"
	upcmder "github.com/papercomputeco/masterblaster/cmd/up"
	versioncmder "github.com/papercomputeco/masterblaster/cmd/version"
	vmhostcmder "github.com/papercomputeco/masterblaster/cmd/vmhost"
	"github.com/papercomputeco/masterblaster/pkg/mbconfig"
	"github.com/papercomputeco/masterblaster/pkg/telemetry"
)

const rootLongDesc string = `Masterblaster (mb) is an AI agent sandbox management, build, and
infrastructure tool for operators embracing safe, sandboxed agentic workflows.

It manages StereOS virtual machines, bootstraps agent environments, and
provides the foundation for the Paper Compute ecosystem.`

const rootShortDesc string = "Masterblaster -- AI agent sandbox management"

// NewMbCmd creates and returns the root mb command with all subcommands registered.
func NewMbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "mb",
		Short:         rootShortDesc,
		Long:          rootLongDesc,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE:  initTelemetry,
		PersistentPostRunE: closeTelemetry,
	}

	cmd.PersistentFlags().String("config-dir", "", "Config directory (default: $XDG_CONFIG_HOME/mb)")
	cmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	cmd.PersistentFlags().Bool("disable-telemetry", false, "Disable anonymous telemetry")

	cmd.AddCommand(servecmder.NewServeCmd(mbconfig.ConfigDir))
	cmd.AddCommand(initcmder.NewInitCmd())
	cmd.AddCommand(upcmder.NewUpCmd(mbconfig.ConfigDir))
	cmd.AddCommand(applycmder.NewApplyCmd(mbconfig.ConfigDir))
	cmd.AddCommand(downcmder.NewDownCmd(mbconfig.ConfigDir))
	cmd.AddCommand(statuscmder.NewStatusCmd(mbconfig.ConfigDir))
	cmd.AddCommand(destroycmder.NewDestroyCmd(mbconfig.ConfigDir))
	cmd.AddCommand(sshcmder.NewSSHCmd(mbconfig.ConfigDir, mbconfig.Verbose))
	cmd.AddCommand(listcmder.NewListCmd(mbconfig.ConfigDir))
	cmd.AddCommand(mixtapescmder.NewMixtapesCmd(mbconfig.ConfigDir))
	cmd.AddCommand(pullcmder.NewPullCmd(mbconfig.ConfigDir))
	cmd.AddCommand(versioncmder.NewVersionCmd())
	cmd.AddCommand(vmhostcmder.NewVMHostCmd(mbconfig.ConfigDir))

	return cmd
}

// initTelemetry initializes anonymous telemetry and stores the client in the
// command context. Telemetry is silently skipped when disabled via config/flag/env
// or CI detection -- errors during init never block command execution.
func initTelemetry(cmd *cobra.Command, _ []string) error {
	// Init config first so Viper binds the disable-telemetry flag/env/config.
	if err := mbconfig.Init(cmd); err != nil {
		return err
	}

	// Viper handles flag < env < config precedence for disable-telemetry.
	if viper.GetBool("disable-telemetry") {
		return nil
	}

	if telemetry.IsCI() {
		return nil
	}

	telem := telemetry.NewPosthogClient(true, utils.Version)
	telem.CaptureInstall()

	cmd.SetContext(telemetry.WithContext(cmd.Context(), telem))

	return nil
}

// closeTelemetry flushes pending events and shuts down the PostHog client.
func closeTelemetry(cmd *cobra.Command, _ []string) error {
	telemetry.FromContext(cmd.Context()).Done()
	return nil
}

func main() {
	cmd := NewMbCmd()

	err := cmd.Execute()
	if err != nil {
		ui.Error("%v", err)
		os.Exit(1)
	}
}
