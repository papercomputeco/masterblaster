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

// AgentType defines how the agent process is executed inside the guest.
type AgentType string

const (
	// AgentTypeSandboxed runs the agent in a gVisor (runsc) sandbox with
	// read-only /nix/store bind mounts. This is the default.
	AgentTypeSandboxed AgentType = "sandboxed"

	// AgentTypeNative runs the agent directly on the host in a tmux
	// session as the agent user (the original agentd behavior).
	AgentTypeNative AgentType = "native"
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

	// Agents defines the agent harnesses to run inside this sandbox.
	// Each entry is an independent agent managed by agentd.
	Agents []AgentConfig `toml:"agents"`
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
	// Type selects the agent execution mode.
	// "sandboxed" (default) runs in a gVisor container with /nix/store sharing.
	// "native" runs directly on the host in a tmux session.
	Type AgentType `toml:"type,omitempty"`

	// Name is a unique identifier for this agent. If omitted, a name is
	// auto-generated from the harness name (e.g. "claude-code", "claude-code-1").
	Name string `toml:"name"`

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
	// Only used for native agents.
	Session string `toml:"session"`

	// ExtraPackages is a list of additional Nix package attribute names
	// to install into the sandbox (e.g. ["ripgrep", "fd", "python311"]).
	// These are resolved against the system's nixpkgs and materialized
	// into /nix/store at agent launch time. Only used for sandboxed agents.
	ExtraPackages []string `toml:"extra_packages,omitempty"`

	// Replicas is the number of identical agents to launch from this
	// spec. Defaults to 1. When > 1, each replica gets a unique name
	// suffixed with its index (e.g. "reviewer-0", "reviewer-1").
	// Useful for launching swarms of agents performing the same task.
	Replicas int `toml:"replicas"`

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
		Agents: []AgentConfig{
			{
				Harness: "claude-code",
				Workdir: "/workspace",
				Restart: "no",
			},
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

	// Apply per-agent defaults.
	for i := range cfg.Agents {
		a := &cfg.Agents[i]
		if a.Type == "" {
			a.Type = AgentTypeSandboxed
		}
		if a.Replicas <= 0 {
			a.Replicas = 1
		}
		if a.Restart == "" {
			a.Restart = "no"
		}
		if a.GracePeriod == "" {
			a.GracePeriod = "30s"
		}
		if a.Workdir == "" {
			if len(cfg.Shared) > 0 {
				a.Workdir = cfg.Shared[0].Guest
			} else {
				a.Workdir = "/workspace"
			}
		}
		if a.Env == nil {
			a.Env = make(map[string]string)
		}
	}

	// Expand replicas before name assignment: a single [[agents]] entry
	// with replicas=5 becomes 5 individual agent entries.
	cfg.Agents = expandReplicas(cfg.Agents)

	// Auto-generate agent names for agents without explicit names.
	assignAgentNames(cfg.Agents)

	if cfg.Secrets == nil {
		cfg.Secrets = make(map[string]string)
	}
}

// expandReplicas expands agent entries with Replicas > 1 into individual
// agent entries. Each replica is a copy of the original with a unique
// name suffix. For replicas=1, the entry is left unchanged.
//
// Naming rules:
//   - replicas=1, name="rev"   -> "rev" (unchanged)
//   - replicas=3, name="rev"   -> "rev-0", "rev-1", "rev-2"
//   - replicas=3, name=""      -> name left empty (assignAgentNames handles it later)
//     but since there are now 3 unnamed entries with the same harness,
//     assignAgentNames will produce "claude-code-0", "claude-code-1", "claude-code-2"
func expandReplicas(agents []AgentConfig) []AgentConfig {
	// Fast path: if all agents have replicas=1, return as-is.
	needsExpansion := false
	total := 0
	for i := range agents {
		if agents[i].Replicas > 1 {
			needsExpansion = true
		}
		total += agents[i].Replicas
	}
	if !needsExpansion {
		return agents
	}

	expanded := make([]AgentConfig, 0, total)
	for _, a := range agents {
		if a.Replicas <= 1 {
			expanded = append(expanded, a)
			continue
		}

		baseName := a.Name
		for j := 0; j < a.Replicas; j++ {
			replica := a
			replica.Replicas = 1
			if baseName != "" {
				replica.Name = fmt.Sprintf("%s-%d", baseName, j)
			}
			// If baseName is empty, leave Name empty — assignAgentNames
			// will handle it and produce unique names from the harness.
			// Session is also left empty so it defaults to the final name.
			replica.Session = ""
			// Deep-copy the env map so replicas don't share a reference.
			if a.Env != nil {
				replica.Env = make(map[string]string, len(a.Env))
				for k, v := range a.Env {
					replica.Env[k] = v
				}
			}
			expanded = append(expanded, replica)
		}
	}
	return expanded
}

// assignAgentNames fills in Name for agents that don't have one set.
// The first agent with a given harness gets the harness name (e.g. "claude-code").
// Subsequent agents with the same harness get "<harness>-1", "<harness>-2", etc.
func assignAgentNames(agents []AgentConfig) {
	// Count how many times each harness appears (for unnamed agents).
	harnessCount := make(map[string]int)
	for i := range agents {
		if agents[i].Name == "" {
			harnessCount[agents[i].Harness]++
		}
	}

	// Track how many of each harness we've assigned so far.
	harnessIdx := make(map[string]int)
	for i := range agents {
		if agents[i].Name != "" {
			continue
		}
		h := agents[i].Harness
		idx := harnessIdx[h]
		harnessIdx[h]++

		if harnessCount[h] == 1 {
			// Only one unnamed agent with this harness — use harness name directly.
			agents[i].Name = h
		} else {
			// Multiple unnamed agents — suffix with index.
			agents[i].Name = fmt.Sprintf("%s-%d", h, idx)
		}
	}
}

// expandPaths resolves ~ and ${ENV} references in path fields.
// baseDir is the directory containing the jcard.toml for resolving relative paths.
func expandPaths(cfg *JcardConfig, baseDir string) {
	// Expand shared mount host paths
	for i := range cfg.Shared {
		cfg.Shared[i].Host = expandPath(cfg.Shared[i].Host, baseDir)
	}

	// Expand per-agent paths
	for i := range cfg.Agents {
		if cfg.Agents[i].PromptFile != "" {
			cfg.Agents[i].PromptFile = expandPath(cfg.Agents[i].PromptFile, baseDir)
		}
		cfg.Agents[i].Env = expandEnvMap(cfg.Agents[i].Env)
	}

	// Expand environment variable references in secrets
	cfg.Secrets = expandEnvMap(cfg.Secrets)
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

	// Validate each agent.
	validHarnesses := map[string]bool{
		"claude-code": true,
		"opencode":    true,
		"gemini-cli":  true,
		"custom":      true,
	}
	validRestart := map[string]bool{"no": true, "on-failure": true, "always": true}
	validAgentTypes := map[string]AgentType{
		"sandboxed": AgentTypeSandboxed,
		"native":    AgentTypeNative,
	}
	namesSeen := make(map[string]bool, len(cfg.Agents))

	for i, a := range cfg.Agents {
		// Validate agent type.
		if _, ok := validAgentTypes[string(a.Type)]; !ok {
			return fmt.Errorf("agents[%d].type must be \"sandboxed\" or \"native\", got %q", i, a.Type)
		}

		// Validate unique names.
		if namesSeen[a.Name] {
			return fmt.Errorf("agents[%d]: duplicate agent name %q", i, a.Name)
		}
		namesSeen[a.Name] = true

		if a.Harness != "" {
			if !validHarnesses[a.Harness] {
				return fmt.Errorf("agents[%d].harness must be \"claude-code\", \"opencode\", \"gemini-cli\", or \"custom\", got %q", i, a.Harness)
			}
		}

		if !validRestart[a.Restart] {
			return fmt.Errorf("agents[%d].restart must be \"no\", \"on-failure\", or \"always\", got %q", i, a.Restart)
		}

		if a.MaxRestarts < 0 {
			return fmt.Errorf("agents[%d].max_restarts must be >= 0, got %d", i, a.MaxRestarts)
		}

		if a.Replicas < 1 {
			return fmt.Errorf("agents[%d].replicas must be >= 1, got %d", i, a.Replicas)
		}

		// Validate extra_packages entries are non-empty strings.
		for j, pkg := range a.ExtraPackages {
			if strings.TrimSpace(pkg) == "" {
				return fmt.Errorf("agents[%d].extra_packages[%d] is empty", i, j)
			}
		}

		// extra_packages is only valid for sandboxed agents.
		if a.Type != AgentTypeSandboxed && len(a.ExtraPackages) > 0 {
			return fmt.Errorf("agents[%d].extra_packages is only supported for type=\"sandboxed\"", i)
		}
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
