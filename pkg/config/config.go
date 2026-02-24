// Package config handles parsing and validation of jcard.toml configuration
// files. The jcard.toml format is the primary configuration mechanism for
// Masterblaster sandboxes, defining the mixtape, resources, networking,
// shared directories, secrets, and agent configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// JcardConfig is the top-level configuration parsed from a jcard.toml file.
type JcardConfig struct {
	// Mixtape is the StereOS image to boot, in "name:tag" format.
	// The tag defaults to "latest" when omitted.
	//
	// Examples:
	//   "opencode-mixtape"       -> opencode-mixtape:latest
	//   "opencode-mixtape:0.1.0" -> pinned to tag 0.1.0
	//   "base"                   -> base:latest
	//
	// Images are resolved from ~/.config/mb/mixtapes/{name}/{tag}/.
	// Pull with: mb pull opencode-mixtape:0.1.0
	Mixtape string `toml:"mixtape"`

	// MixtapeDigest pins to an exact digest for reproducible builds.
	// When set, takes precedence over the tag in Mixtape.
	MixtapeDigest string `toml:"mixtape_digest"`

	// Name is a human-readable name for this sandbox. Defaults to the
	// parent directory name. Must be unique across running sandboxes.
	Name string `toml:"name"`

	// Resources for the sandbox VM.
	Resources ResourcesConfig `toml:"resources"`

	// Network configuration for the sandbox.
	Network NetworkConfig `toml:"network"`

	// Shared directories mounted from host into sandbox.
	Shared []SharedMount `toml:"shared"`

	// Secrets injected into the sandbox at runtime via stereosd.
	Secrets map[string]string `toml:"secrets"`

	// Agent runtime configuration (passed to agentd).
	Agent AgentConfig `toml:"agent"`
}

// ResourcesConfig describes the VM resource allocation.
type ResourcesConfig struct {
	CPUs   int    `toml:"cpus"`
	Memory string `toml:"memory"`
	Disk   string `toml:"disk"`
}

// NetworkConfig describes sandbox networking.
type NetworkConfig struct {
	// Mode is the network mode: "nat" (default), "bridged", or "none".
	Mode string `toml:"mode"`

	// Forwards are port forwards from host to sandbox (nat mode only).
	Forwards []PortForward `toml:"forwards"`

	// EgressAllow is an allowlist of domains/CIDRs reachable from the
	// sandbox. Empty means no restrictions.
	EgressAllow []string `toml:"egress_allow"`
}

// PortForward maps a host port to a guest port.
type PortForward struct {
	Host  int    `toml:"host"`
	Guest int    `toml:"guest"`
	Proto string `toml:"proto"` // "tcp" or "udp"
}

// SharedMount maps a host directory to a guest mount point.
type SharedMount struct {
	Host     string `toml:"host"`
	Guest    string `toml:"guest"`
	ReadOnly bool   `toml:"readonly"`
}

// AgentConfig defines what agent harness to run and how agentd manages it.
// This section is passed through to agentd on the guest.
type AgentConfig struct {
	// Harness is the agent harness to use: "claude-code", "opencode",
	// "gemini-cli", or "custom".
	Harness string `toml:"harness"`

	// Prompt is the prompt or command to give the agent on boot.
	Prompt string `toml:"prompt"`

	// PromptFile is a path to a prompt file (relative to jcard.toml).
	// Takes precedence over Prompt.
	PromptFile string `toml:"prompt_file"`

	// Workdir is the working directory inside the sandbox.
	// Defaults to the first shared mount guest path, or /workspace.
	Workdir string `toml:"workdir"`

	// Restart policy: "no" (default), "on-failure", "always".
	Restart string `toml:"restart"`

	// MaxRestarts is the maximum restart attempts (0 = unlimited).
	MaxRestarts int `toml:"max_restarts"`

	// Timeout for the agent to complete (Go duration string, e.g. "2h").
	Timeout string `toml:"timeout"`

	// GracePeriod for SIGTERM before SIGKILL (Go duration, default "30s").
	GracePeriod string `toml:"grace_period"`

	// Session is the tmux session name. Defaults to the harness name.
	Session string `toml:"session"`

	// Env are environment variables set only for the agent process.
	Env map[string]string `toml:"env"`
}

// Load reads and parses a jcard.toml config file, applies defaults,
// expands environment variables and paths, and validates the result.
func Load(path string) (*JcardConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg JcardConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Infer name from directory if not set
	if cfg.Name == "" {
		dir := filepath.Dir(path)
		cfg.Name = filepath.Base(dir)
		// If it's just "." use a better default
		if cfg.Name == "." || cfg.Name == "/" {
			cfg.Name = "sandbox"
		}
	}

	applyDefaults(&cfg)
	expandPaths(&cfg, filepath.Dir(path))

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// DefaultJcard returns a default jcard.toml configuration suitable for
// scaffolding with `mb init`.
func DefaultJcard() *JcardConfig {
	cfg := &JcardConfig{
		Mixtape: "base",
		Resources: ResourcesConfig{
			CPUs:   2,
			Memory: "4GiB",
			Disk:   "20GiB",
		},
		Network: NetworkConfig{
			Mode: "nat",
		},
		Agent: AgentConfig{
			Harness: "claude-code",
			Workdir: "/workspace",
			Restart: "no",
		},
	}
	return cfg
}

// applyDefaults fills in zero-value fields with sensible defaults.
func applyDefaults(cfg *JcardConfig) {
	if cfg.Mixtape == "" {
		cfg.Mixtape = "base:latest"
	}

	if cfg.Resources.CPUs == 0 {
		cfg.Resources.CPUs = 2
	}
	if cfg.Resources.Memory == "" {
		cfg.Resources.Memory = "4GiB"
	}
	if cfg.Resources.Disk == "" {
		cfg.Resources.Disk = "20GiB"
	}

	if cfg.Network.Mode == "" {
		cfg.Network.Mode = "nat"
	}

	// Default port forward proto
	for i := range cfg.Network.Forwards {
		if cfg.Network.Forwards[i].Proto == "" {
			cfg.Network.Forwards[i].Proto = "tcp"
		}
	}

	if cfg.Agent.Restart == "" {
		cfg.Agent.Restart = "no"
	}
	if cfg.Agent.GracePeriod == "" {
		cfg.Agent.GracePeriod = "30s"
	}
	if cfg.Agent.Workdir == "" {
		// Default to first shared mount, or /workspace
		if len(cfg.Shared) > 0 {
			cfg.Agent.Workdir = cfg.Shared[0].Guest
		} else {
			cfg.Agent.Workdir = "/workspace"
		}
	}

	if cfg.Secrets == nil {
		cfg.Secrets = make(map[string]string)
	}
	if cfg.Agent.Env == nil {
		cfg.Agent.Env = make(map[string]string)
	}
}

// expandPaths resolves ~ and ${ENV} references in path fields.
// baseDir is the directory containing the jcard.toml for resolving relative paths.
func expandPaths(cfg *JcardConfig, baseDir string) {
	// Expand shared mount host paths
	for i := range cfg.Shared {
		cfg.Shared[i].Host = expandPath(cfg.Shared[i].Host, baseDir)
	}

	// Expand prompt_file relative to jcard.toml
	if cfg.Agent.PromptFile != "" {
		cfg.Agent.PromptFile = expandPath(cfg.Agent.PromptFile, baseDir)
	}

	// Expand environment variable references in secrets
	cfg.Secrets = expandEnvMap(cfg.Secrets)

	// Expand environment variable references in agent env
	cfg.Agent.Env = expandEnvMap(cfg.Agent.Env)
}

// validate checks that required fields are present and values are sane.
func validate(cfg *JcardConfig) error {
	if cfg.Resources.CPUs < 1 {
		return fmt.Errorf("resources.cpus must be at least 1")
	}

	validNetModes := map[string]bool{"nat": true, "bridged": true, "none": true}
	if !validNetModes[cfg.Network.Mode] {
		return fmt.Errorf("network.mode must be \"nat\", \"bridged\", or \"none\", got %q", cfg.Network.Mode)
	}

	for i, fwd := range cfg.Network.Forwards {
		if fwd.Host <= 0 || fwd.Host > 65535 {
			return fmt.Errorf("forwards[%d].host must be 1-65535, got %d", i, fwd.Host)
		}
		if fwd.Guest <= 0 || fwd.Guest > 65535 {
			return fmt.Errorf("forwards[%d].guest must be 1-65535, got %d", i, fwd.Guest)
		}
		if fwd.Proto != "tcp" && fwd.Proto != "udp" {
			return fmt.Errorf("forwards[%d].proto must be \"tcp\" or \"udp\", got %q", i, fwd.Proto)
		}
	}

	if cfg.Agent.Harness != "" {
		validHarnesses := map[string]bool{
			"claude-code": true,
			"opencode":    true,
			"gemini-cli":  true,
			"custom":      true,
		}
		if !validHarnesses[cfg.Agent.Harness] {
			return fmt.Errorf("agent.harness must be \"claude-code\", \"opencode\", \"gemini-cli\", or \"custom\", got %q", cfg.Agent.Harness)
		}
	}

	validRestart := map[string]bool{"no": true, "on-failure": true, "always": true}
	if !validRestart[cfg.Agent.Restart] {
		return fmt.Errorf("agent.restart must be \"no\", \"on-failure\", or \"always\", got %q", cfg.Agent.Restart)
	}

	if cfg.Agent.MaxRestarts < 0 {
		return fmt.Errorf("agent.max_restarts must be >= 0, got %d", cfg.Agent.MaxRestarts)
	}

	return nil
}

// expandPath resolves ~ and relative paths. Relative paths are resolved
// against baseDir (the directory containing jcard.toml).
func expandPath(path, baseDir string) string {
	if path == "" {
		return path
	}

	// Expand environment variables
	path = expandEnvVars(path)

	// Expand ~
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		path = filepath.Join(home, path[2:])
	}

	// Make relative paths absolute relative to baseDir
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}

	return filepath.Clean(path)
}
