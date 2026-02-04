package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config is the top-level configuration parsed from a TOML file.
type Config struct {
	SchemaVersion string                 `toml:"schema_version"`
	VM            VMConfig               `toml:"vm"`
	SSH           SSHConfig              `toml:"ssh"`
	Volumes       map[string]VolumeMount `toml:"volumes"`
	Environment   map[string]string      `toml:"environment"`
	OpenCode      OpenCodeConfig         `toml:"opencode"`
	Network       NetworkConfig          `toml:"network"`
}

// VMConfig describes the virtual machine resources.
type VMConfig struct {
	Name     string `toml:"name"`
	Image    string `toml:"image"`
	CPUs     int    `toml:"cpus"`
	Memory   string `toml:"memory"`
	DiskSize string `toml:"disk_size"`
}

// SSHConfig describes SSH connectivity parameters.
type SSHConfig struct {
	User           string `toml:"user"`
	Port           int    `toml:"port"`
	HostPort       int    `toml:"host_port"`
	IdentityFile   string `toml:"identity_file"`
	PublicKeyFile  string `toml:"public_key_file"`
	ConnectTimeout int    `toml:"connect_timeout"`
}

// VolumeMount maps a host directory to a guest mount point.
type VolumeMount struct {
	Host  string `toml:"host"`
	Guest string `toml:"guest"`
}

// OpenCodeConfig controls the OpenCode AI agent setup in the guest.
type OpenCodeConfig struct {
	AutoStart  bool   `toml:"auto_start"`
	ConfigFile string `toml:"config_file"`
	Model      string `toml:"model"`
}

// NetworkConfig controls guest networking mode.
type NetworkConfig struct {
	Mode string `toml:"mode"`
}

// Load reads and parses a TOML config file, applies defaults, expands
// environment variables and paths, and validates the result.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyDefaults(&cfg)
	expandPaths(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// applyDefaults fills in zero-value fields with sensible defaults.
func applyDefaults(cfg *Config) {
	if cfg.VM.Image == "" {
		cfg.VM.Image = "fedora-42"
	}
	if cfg.VM.CPUs == 0 {
		cfg.VM.CPUs = 4
	}
	if cfg.VM.Memory == "" {
		cfg.VM.Memory = "4G"
	}
	if cfg.VM.DiskSize == "" {
		cfg.VM.DiskSize = "10G"
	}

	if cfg.SSH.User == "" {
		cfg.SSH.User = "agent"
	}
	if cfg.SSH.Port == 0 {
		cfg.SSH.Port = 22
	}
	if cfg.SSH.HostPort == 0 {
		cfg.SSH.HostPort = 2222
	}
	if cfg.SSH.ConnectTimeout == 0 {
		cfg.SSH.ConnectTimeout = 120
	}

	if cfg.Network.Mode == "" {
		cfg.Network.Mode = "user"
	}

	if cfg.Environment == nil {
		cfg.Environment = make(map[string]string)
	}
	if cfg.Volumes == nil {
		cfg.Volumes = make(map[string]VolumeMount)
	}
}

// expandPaths resolves ~ and ${ENV} references in path and environment fields.
func expandPaths(cfg *Config) {
	cfg.SSH.IdentityFile = ExpandPath(cfg.SSH.IdentityFile)
	cfg.SSH.PublicKeyFile = ExpandPath(cfg.SSH.PublicKeyFile)
	cfg.OpenCode.ConfigFile = ExpandPath(cfg.OpenCode.ConfigFile)

	// Expand environment variable references in the environment map
	cfg.Environment = ExpandEnvMap(cfg.Environment)

	// Expand host paths in volume mounts
	expanded := make(map[string]VolumeMount, len(cfg.Volumes))
	for k, v := range cfg.Volumes {
		v.Host = ExpandPath(v.Host)
		expanded[k] = v
	}
	cfg.Volumes = expanded
}

// validate checks that required fields are present and values are sane.
func validate(cfg *Config) error {
	if cfg.VM.Name == "" {
		return fmt.Errorf("vm.name is required")
	}
	if cfg.VM.CPUs < 1 {
		return fmt.Errorf("vm.cpus must be at least 1")
	}
	if cfg.SSH.PublicKeyFile == "" {
		return fmt.Errorf("ssh.public_key_file is required")
	}
	if cfg.SSH.IdentityFile == "" {
		return fmt.Errorf("ssh.identity_file is required")
	}
	if cfg.Network.Mode != "user" && cfg.Network.Mode != "vmnet" {
		return fmt.Errorf("network.mode must be \"user\" or \"vmnet\", got %q", cfg.Network.Mode)
	}
	return nil
}

// ReadPublicKey reads an SSH public key file and returns its contents as a string.
func ReadPublicKey(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading SSH public key %q: %w", path, err)
	}
	return string(data), nil
}
