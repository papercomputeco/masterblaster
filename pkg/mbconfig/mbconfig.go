// Package mbconfig centralizes Masterblaster configuration using Viper.
//
// It provides XDG-compliant config directory resolution with the following
// precedence (highest to lowest):
//
//  1. --config-dir CLI flag
//  2. MB_CONFIG_DIR environment variable
//  3. config.toml file in the config directory
//  4. Default: $XDG_CONFIG_HOME/mb (falls back to ~/.config/mb)
//
// The verbose flag follows the same pattern: --verbose flag > MB_VERBOSE env.
package mbconfig

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Init binds Viper to the root command's persistent flags, sets the env
// prefix, applies defaults, and optionally reads a config.toml from the
// resolved config directory.
//
// Call this from the root command's PersistentPreRunE so that flag values
// are parsed before Viper reads them.
func Init(cmd *cobra.Command) error {
	viper.SetEnvPrefix("MB")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	// Bind CLI flags to viper keys.
	if f := cmd.Root().PersistentFlags().Lookup("config-dir"); f != nil {
		if err := viper.BindPFlag("config-dir", f); err != nil {
			return err
		}
	}
	if f := cmd.Root().PersistentFlags().Lookup("verbose"); f != nil {
		if err := viper.BindPFlag("verbose", f); err != nil {
			return err
		}
	}
	if f := cmd.Root().PersistentFlags().Lookup("disable-telemetry"); f != nil {
		if err := viper.BindPFlag("disable-telemetry", f); err != nil {
			return err
		}
	}

	// Set defaults after binding so flags/env take precedence.
	viper.SetDefault("config-dir", defaultConfigDir())
	viper.SetDefault("verbose", false)
	viper.SetDefault("disable-telemetry", false)

	// Attempt to read a config file from the resolved config directory.
	// This is intentionally best-effort: if the file doesn't exist yet,
	// we silently continue with flag/env/defaults.
	dir := viper.GetString("config-dir")
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath(dir)

	if err := viper.ReadInConfig(); err != nil {
		// Only propagate errors that aren't "file not found".
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// If the directory doesn't exist yet, that's fine too.
			if !os.IsNotExist(err) {
				return err
			}
		}
	}

	return nil
}

// ConfigDir returns the resolved config directory path.
// Must be called after Init.
func ConfigDir() string {
	return viper.GetString("config-dir")
}

// Verbose returns whether verbose output is enabled.
// Must be called after Init.
func Verbose() bool {
	return viper.GetBool("verbose")
}

// defaultConfigDir returns $XDG_CONFIG_HOME/mb, falling back to ~/.config/mb
// when XDG_CONFIG_HOME is not set.
func defaultConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "mb")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Last resort; this matches the XDG spec fallback.
		return filepath.Join(os.Getenv("HOME"), ".config", "mb")
	}
	return filepath.Join(home, ".config", "mb")
}
