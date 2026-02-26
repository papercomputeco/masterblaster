// Package sshcmder provides the ssh command for connecting to a running
// sandbox via SSH.
package sshcmder

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/masterblaster/pkg/daemon/client"
	"github.com/papercomputeco/masterblaster/pkg/ssh"
	"github.com/papercomputeco/masterblaster/pkg/ui"
)

const sshLongDesc string = `Connect to a running sandbox via SSH. Replaces the current process
with the ssh binary for a clean interactive experience.

If no name is given and only one sandbox is running, connects to that one.
The default user is "admin" (the operator account in StereOS).

Examples:
  mb ssh
  mb ssh my-sandbox
  mb ssh --user agent my-sandbox`

const sshShortDesc string = "SSH into a running sandbox"

// NewSSHCmd creates the ssh command. verboseFn is called at runtime to check
// whether verbose output is enabled (resolved via viper).
func NewSSHCmd(configDirFn func() string, verboseFn func() bool) *cobra.Command {
	var user string

	cmd := &cobra.Command{
		Use:   "ssh [name]",
		Short: sshShortDesc,
		Long:  sshLongDesc,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return runSSH(configDirFn(), name, user, verboseFn())
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "admin", "SSH user (default: admin)")

	return cmd
}

func runSSH(baseDir, name, user string, verbose bool) error {
	if err := client.EnsureDaemon(baseDir); err != nil {
		return err
	}

	c := client.New(baseDir)
	resp, err := c.Status(name, false)
	if err != nil {
		return err
	}

	if len(resp.Sandboxes) == 0 {
		return fmt.Errorf("no sandbox found")
	}

	sb := resp.Sandboxes[0]
	if sb.State != "running" {
		return fmt.Errorf("sandbox %q is not running (state: %s)", sb.Name, sb.State)
	}

	if verbose {
		ui.Info("Connecting to %s@127.0.0.1:%d", user, sb.SSHPort)
	}

	return ssh.ExecSSH(user, "127.0.0.1", sb.SSHPort, sb.SSHKeyPath)
}
