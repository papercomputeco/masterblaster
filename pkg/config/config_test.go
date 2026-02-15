package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMinimalConfig(t *testing.T) {
	dir := t.TempDir()
	tomlContent := `
mixtape = "base"
`
	cfgPath := filepath.Join(dir, "jcard.toml")
	if err := os.WriteFile(cfgPath, []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Mixtape != "base" {
		t.Errorf("Mixtape = %q, want %q", cfg.Mixtape, "base")
	}
	// Check defaults
	if cfg.Resources.CPUs != 2 {
		t.Errorf("Resources.CPUs = %d, want 2", cfg.Resources.CPUs)
	}
	if cfg.Resources.Memory != "4GiB" {
		t.Errorf("Resources.Memory = %q, want %q", cfg.Resources.Memory, "4GiB")
	}
	if cfg.Resources.Disk != "20GiB" {
		t.Errorf("Resources.Disk = %q, want %q", cfg.Resources.Disk, "20GiB")
	}
	if cfg.Network.Mode != "nat" {
		t.Errorf("Network.Mode = %q, want %q", cfg.Network.Mode, "nat")
	}
	if cfg.Agent.Restart != "no" {
		t.Errorf("Agent.Restart = %q, want %q", cfg.Agent.Restart, "no")
	}
	if cfg.Agent.GracePeriod != "30s" {
		t.Errorf("Agent.GracePeriod = %q, want %q", cfg.Agent.GracePeriod, "30s")
	}
	if cfg.Agent.Workdir != "/workspace" {
		t.Errorf("Agent.Workdir = %q, want %q", cfg.Agent.Workdir, "/workspace")
	}
}

func TestLoadFullConfig(t *testing.T) {
	dir := t.TempDir()

	// Create a prompt file
	promptPath := filepath.Join(dir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte("fix the tests"), 0644); err != nil {
		t.Fatal(err)
	}

	tomlContent := `
mixtape = "openclaw"
name = "my-agent"

[resources]
cpus = 8
memory = "16GiB"
disk = "100GiB"

[network]
mode = "nat"
forwards = [
    { host = 8080, guest = 8080, proto = "tcp" },
    { host = 9090, guest = 9090, proto = "udp" },
]
egress_allow = ["api.anthropic.com"]

[[shared]]
host = "./"
guest = "/workspace"
readonly = false

[[shared]]
host = "/tmp/data"
guest = "/data"
readonly = true

[secrets]
MY_SECRET = "secret-value"

[agent]
harness = "claude-code"
prompt_file = "./prompt.md"
workdir = "/workspace"
restart = "on-failure"
max_restarts = 5
timeout = "2h"
grace_period = "60s"
session = "my-session"

[agent.env]
FOO = "bar"
`
	cfgPath := filepath.Join(dir, "jcard.toml")
	if err := os.WriteFile(cfgPath, []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Mixtape != "openclaw" {
		t.Errorf("Mixtape = %q, want %q", cfg.Mixtape, "openclaw")
	}
	if cfg.Name != "my-agent" {
		t.Errorf("Name = %q, want %q", cfg.Name, "my-agent")
	}
	if cfg.Resources.CPUs != 8 {
		t.Errorf("Resources.CPUs = %d, want 8", cfg.Resources.CPUs)
	}
	if cfg.Resources.Memory != "16GiB" {
		t.Errorf("Resources.Memory = %q, want %q", cfg.Resources.Memory, "16GiB")
	}
	if cfg.Resources.Disk != "100GiB" {
		t.Errorf("Resources.Disk = %q, want %q", cfg.Resources.Disk, "100GiB")
	}

	// Network
	if len(cfg.Network.Forwards) != 2 {
		t.Fatalf("len(Network.Forwards) = %d, want 2", len(cfg.Network.Forwards))
	}
	if cfg.Network.Forwards[0].Host != 8080 {
		t.Errorf("Forwards[0].Host = %d, want 8080", cfg.Network.Forwards[0].Host)
	}
	if cfg.Network.Forwards[1].Proto != "udp" {
		t.Errorf("Forwards[1].Proto = %q, want %q", cfg.Network.Forwards[1].Proto, "udp")
	}
	if len(cfg.Network.EgressAllow) != 1 || cfg.Network.EgressAllow[0] != "api.anthropic.com" {
		t.Errorf("Network.EgressAllow = %v, want [api.anthropic.com]", cfg.Network.EgressAllow)
	}

	// Shared
	if len(cfg.Shared) != 2 {
		t.Fatalf("len(Shared) = %d, want 2", len(cfg.Shared))
	}
	if cfg.Shared[0].Guest != "/workspace" {
		t.Errorf("Shared[0].Guest = %q, want %q", cfg.Shared[0].Guest, "/workspace")
	}
	if cfg.Shared[1].ReadOnly != true {
		t.Error("Shared[1].ReadOnly = false, want true")
	}

	// Secrets
	if cfg.Secrets["MY_SECRET"] != "secret-value" {
		t.Errorf("Secrets[MY_SECRET] = %q, want %q", cfg.Secrets["MY_SECRET"], "secret-value")
	}

	// Agent
	if cfg.Agent.Harness != "claude-code" {
		t.Errorf("Agent.Harness = %q, want %q", cfg.Agent.Harness, "claude-code")
	}
	if cfg.Agent.PromptFile != filepath.Join(dir, "prompt.md") {
		t.Errorf("Agent.PromptFile = %q, want %q", cfg.Agent.PromptFile, filepath.Join(dir, "prompt.md"))
	}
	if cfg.Agent.Restart != "on-failure" {
		t.Errorf("Agent.Restart = %q, want %q", cfg.Agent.Restart, "on-failure")
	}
	if cfg.Agent.MaxRestarts != 5 {
		t.Errorf("Agent.MaxRestarts = %d, want 5", cfg.Agent.MaxRestarts)
	}
	if cfg.Agent.Timeout != "2h" {
		t.Errorf("Agent.Timeout = %q, want %q", cfg.Agent.Timeout, "2h")
	}
	if cfg.Agent.GracePeriod != "60s" {
		t.Errorf("Agent.GracePeriod = %q, want %q", cfg.Agent.GracePeriod, "60s")
	}
	if cfg.Agent.Session != "my-session" {
		t.Errorf("Agent.Session = %q, want %q", cfg.Agent.Session, "my-session")
	}
	if cfg.Agent.Env["FOO"] != "bar" {
		t.Errorf("Agent.Env[FOO] = %q, want %q", cfg.Agent.Env["FOO"], "bar")
	}
}

func TestValidateInvalidNetworkMode(t *testing.T) {
	dir := t.TempDir()
	tomlContent := `
mixtape = "base"

[network]
mode = "invalid"
`
	cfgPath := filepath.Join(dir, "jcard.toml")
	os.WriteFile(cfgPath, []byte(tomlContent), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid network mode, got nil")
	}
}

func TestValidateInvalidHarness(t *testing.T) {
	dir := t.TempDir()
	tomlContent := `
mixtape = "base"

[agent]
harness = "invalid-harness"
`
	cfgPath := filepath.Join(dir, "jcard.toml")
	os.WriteFile(cfgPath, []byte(tomlContent), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid harness, got nil")
	}
}

func TestValidateInvalidRestart(t *testing.T) {
	dir := t.TempDir()
	tomlContent := `
mixtape = "base"

[agent]
restart = "invalid"
`
	cfgPath := filepath.Join(dir, "jcard.toml")
	os.WriteFile(cfgPath, []byte(tomlContent), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid restart policy, got nil")
	}
}

func TestValidateInvalidPortForward(t *testing.T) {
	dir := t.TempDir()
	tomlContent := `
mixtape = "base"

[network]
mode = "nat"
forwards = [
    { host = 0, guest = 80, proto = "tcp" },
]
`
	cfgPath := filepath.Join(dir, "jcard.toml")
	os.WriteFile(cfgPath, []byte(tomlContent), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid port forward, got nil")
	}
}

func TestNameDefaultsToDirectoryName(t *testing.T) {
	dir := t.TempDir()
	tomlContent := `mixtape = "base"`
	cfgPath := filepath.Join(dir, "jcard.toml")
	os.WriteFile(cfgPath, []byte(tomlContent), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	expected := filepath.Base(dir)
	if cfg.Name != expected {
		t.Errorf("Name = %q, want %q (directory name)", cfg.Name, expected)
	}
}

func TestExpandEnvVarsInSecrets(t *testing.T) {
	os.Setenv("MB_TEST_KEY", "test-api-key-123")
	defer os.Unsetenv("MB_TEST_KEY")

	dir := t.TempDir()
	tomlContent := `
mixtape = "base"

[secrets]
API_KEY = "${MB_TEST_KEY}"
`
	cfgPath := filepath.Join(dir, "jcard.toml")
	os.WriteFile(cfgPath, []byte(tomlContent), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Secrets["API_KEY"] != "test-api-key-123" {
		t.Errorf("Secrets[API_KEY] = %q, want %q", cfg.Secrets["API_KEY"], "test-api-key-123")
	}
}

func TestDefaultJcardTOML(t *testing.T) {
	content := DefaultJcardTOML()
	if content == "" {
		t.Error("DefaultJcardTOML() returned empty string")
	}

	// Verify it's valid TOML
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "jcard.toml")
	os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("DefaultJcardTOML() generated invalid TOML: %v", err)
	}
}

func TestWorkdirDefaultsToFirstSharedMount(t *testing.T) {
	dir := t.TempDir()
	tomlContent := `
mixtape = "base"

[[shared]]
host = "./"
guest = "/code"
`
	cfgPath := filepath.Join(dir, "jcard.toml")
	os.WriteFile(cfgPath, []byte(tomlContent), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Agent.Workdir != "/code" {
		t.Errorf("Agent.Workdir = %q, want %q (first shared mount)", cfg.Agent.Workdir, "/code")
	}
}
