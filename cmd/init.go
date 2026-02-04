package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/paper-compute-co/masterblaster/internal/config"
	"github.com/paper-compute-co/masterblaster/internal/ssh"
	"github.com/paper-compute-co/masterblaster/internal/ui"
	"github.com/paper-compute-co/masterblaster/internal/vm"
)

var initCmd = &cobra.Command{
	Use:   "init <config.toml>",
	Short: "Create and start a new VM from a config file",
	Long: `Parse the TOML config, provision a cloud-init ISO, create a qcow2
overlay disk, and launch a QEMU virtual machine. Waits for SSH to become
available before returning.`,
	Args: cobra.ExactArgs(1),
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(args[0])
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	backend := vm.NewQEMUBackend(globalConfigDir())

	// Ensure base image is available
	ui.Status("Checking base image...")
	basePath, err := backend.EnsureBaseImage(cmd.Context(), cfg.VM.Image)
	if err != nil {
		return fmt.Errorf("preparing base image: %w", err)
	}

	// Read SSH public key
	pubKey, err := config.ReadPublicKey(cfg.SSH.PublicKeyFile)
	if err != nil {
		return fmt.Errorf("reading SSH public key: %w", err)
	}

	// Download OpenCode binary to embed in the cloud-init ISO
	openCodeBin, err := vm.CacheOpenCode(globalConfigDir())
	if err != nil {
		ui.Warn("Failed to download OpenCode: %s", err)
		ui.Warn("OpenCode will not be pre-installed in the VM")
	}

	// Build cloud-init data
	ciData := vm.CloudInitData{
		InstanceID:     cfg.VM.Name,
		Hostname:       cfg.VM.Name,
		User:           cfg.SSH.User,
		SSHPublicKey:   pubKey,
		Packages:       []string{"git", "vim", "tmux", "curl", "unzip", "openssh-server"},
		Environment:    cfg.Environment,
		OpenCode:       cfg.OpenCode,
		OpenCodeBinary: openCodeBin,
	}

	// Create VM
	ui.Status("Creating VM %q...", cfg.VM.Name)
	inst, err := backend.Create(cmd.Context(), vm.CreateOpts{
		Name:        cfg.VM.Name,
		BaseImage:   basePath,
		DiskSize:    cfg.VM.DiskSize,
		CPUs:        cfg.VM.CPUs,
		Memory:      cfg.VM.Memory,
		CloudInit:   ciData,
		Volumes:     cfg.Volumes,
		SSHHostPort: cfg.SSH.HostPort,
		Network:     cfg.Network.Mode,
	})
	if err != nil {
		return fmt.Errorf("creating VM: %w", err)
	}

	// Start VM
	ui.Status("Starting VM...")
	if err := backend.Start(cmd.Context(), inst); err != nil {
		return fmt.Errorf("starting VM: %w", err)
	}

	// Wait for SSH
	ui.Status("Waiting for SSH (timeout %ds)...", cfg.SSH.ConnectTimeout)
	if err := ssh.WaitForSSH(inst.SSHAddress, cfg.SSH.ConnectTimeout); err != nil {
		ui.Warn("SSH not ready yet. VM is running — check 'mb list' and try 'mb ssh' later.")
		ui.Warn("Boot log: %s", inst.SerialLogPath())
		return nil
	}

	ui.Success("VM %q ready", cfg.VM.Name)
	ui.Info("SSH: ssh -i %s -p %d %s@127.0.0.1", cfg.SSH.IdentityFile, cfg.SSH.HostPort, cfg.SSH.User)
	ui.Info("Or:  mb ssh %s", cfg.VM.Name)
	return nil
}
