package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/ui"

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
	"github.com/papercomputeco/masterblaster/pkg/mbconfig"
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
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return mbconfig.Init(cmd)
		},
	}

	cmd.PersistentFlags().String("config-dir", "", "Config directory (default: $XDG_CONFIG_HOME/mb)")
	cmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")

	cmd.AddCommand(servecmder.NewServeCmd(mbconfig.ConfigDir))
	cmd.AddCommand(initcmder.NewInitCmd())
	cmd.AddCommand(upcmder.NewUpCmd(mbconfig.ConfigDir))
	cmd.AddCommand(downcmder.NewDownCmd(mbconfig.ConfigDir))
	cmd.AddCommand(statuscmder.NewStatusCmd(mbconfig.ConfigDir))
	cmd.AddCommand(destroycmder.NewDestroyCmd(mbconfig.ConfigDir))
	cmd.AddCommand(sshcmder.NewSSHCmd(mbconfig.ConfigDir, mbconfig.Verbose))
	cmd.AddCommand(listcmder.NewListCmd(mbconfig.ConfigDir))
	cmd.AddCommand(mixtapescmder.NewMixtapesCmd(mbconfig.ConfigDir))
	cmd.AddCommand(pullcmder.NewPullCmd(mbconfig.ConfigDir))
	cmd.AddCommand(versioncmder.NewVersionCmd())

	return cmd
}

func main() {
	cmd := NewMbCmd()

	err := cmd.Execute()
	if err != nil {
		ui.Error("%v", err)
		os.Exit(1)
	}
}
