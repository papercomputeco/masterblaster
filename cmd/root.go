package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	verbose   bool
	configDir string
)

// rootCmd is the base command for the mb CLI.
var rootCmd = &cobra.Command{
	Use:   "mb",
	Short: "Masterblaster — sandboxed Linux VMs for AI coding agents",
	Long: `Masterblaster (mb) creates and manages full Linux virtual machines
as sandboxed environments for AI coding agents on Apple Silicon Macs.

It wraps QEMU with Apple's Hypervisor.framework (HVF) acceleration to run
aarch64 Fedora guests at near-native speed.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&configDir, "config-dir", "", "Config directory (default: ~/.mb)")
}

// globalConfigDir returns the resolved config directory path.
func globalConfigDir() string {
	if configDir != "" {
		return configDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %s\n", err)
		os.Exit(1)
	}
	return filepath.Join(home, ".mb")
}
