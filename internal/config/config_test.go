package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMinimalConfig(t *testing.T) {
	dir := t.TempDir()

	// Create a minimal SSH key for the test
	pubKeyPath := filepath.Join(dir, "id_test.pub")
	if err := os.WriteFile(pubKeyPath, []byte("ssh-ed25519 AAAA... test@test"), 0644); err != nil {
		t.Fatal(err)
	}
	identityPath := filepath.Join(dir, "id_test")
	if err := os.WriteFile(identityPath, []byte("PRIVATE KEY"), 0600); err != nil {
		t.Fatal(err)
	}

	tomlContent := `
schema_version = "0"

[vm]
name = "test-vm"

[ssh]
public_key_file = "` + pubKeyPath + `"
identity_file = "` + identityPath + `"
`
	cfgPath := filepath.Join(dir, "test.toml")
	if err := os.WriteFile(cfgPath, []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Check defaults were applied
	if cfg.VM.Name != "test-vm" {
		t.Errorf("VM.Name = %q, want %q", cfg.VM.Name, "test-vm")
	}
	if cfg.VM.CPUs != 4 {
		t.Errorf("VM.CPUs = %d, want 4", cfg.VM.CPUs)
	}
	if cfg.VM.Memory != "4G" {
		t.Errorf("VM.Memory = %q, want %q", cfg.VM.Memory, "4G")
	}
	if cfg.VM.DiskSize != "10G" {
		t.Errorf("VM.DiskSize = %q, want %q", cfg.VM.DiskSize, "10G")
	}
	if cfg.VM.Image != "fedora-42" {
		t.Errorf("VM.Image = %q, want %q", cfg.VM.Image, "fedora-42")
	}
	if cfg.SSH.User != "agent" {
		t.Errorf("SSH.User = %q, want %q", cfg.SSH.User, "agent")
	}
	if cfg.SSH.Port != 22 {
		t.Errorf("SSH.Port = %d, want 22", cfg.SSH.Port)
	}
	if cfg.SSH.HostPort != 2222 {
		t.Errorf("SSH.HostPort = %d, want 2222", cfg.SSH.HostPort)
	}
	if cfg.SSH.ConnectTimeout != 120 {
		t.Errorf("SSH.ConnectTimeout = %d, want 120", cfg.SSH.ConnectTimeout)
	}
	if cfg.Network.Mode != "user" {
		t.Errorf("Network.Mode = %q, want %q", cfg.Network.Mode, "user")
	}
}

func TestLoadFullConfig(t *testing.T) {
	dir := t.TempDir()

	pubKeyPath := filepath.Join(dir, "id_test.pub")
	if err := os.WriteFile(pubKeyPath, []byte("ssh-ed25519 AAAA..."), 0644); err != nil {
		t.Fatal(err)
	}
	identityPath := filepath.Join(dir, "id_test")
	if err := os.WriteFile(identityPath, []byte("PRIVATE KEY"), 0600); err != nil {
		t.Fatal(err)
	}

	tomlContent := `
schema_version = "0"

[vm]
name = "my-agent"
image = "fedora-42"
cpus = 8
memory = "16G"
disk_size = "100G"

[ssh]
user = "dev"
port = 22
host_port = 3333
identity_file = "` + identityPath + `"
public_key_file = "` + pubKeyPath + `"
connect_timeout = 60

[volumes]
workspace = { host = "/tmp/workspace", guest = "/workspace" }

[environment]
MY_VAR = "hello"

[opencode]
auto_start = true
model = "anthropic/claude-sonnet-4-5"

[network]
mode = "user"
`
	cfgPath := filepath.Join(dir, "full.toml")
	if err := os.WriteFile(cfgPath, []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.VM.CPUs != 8 {
		t.Errorf("VM.CPUs = %d, want 8", cfg.VM.CPUs)
	}
	if cfg.VM.Memory != "16G" {
		t.Errorf("VM.Memory = %q, want %q", cfg.VM.Memory, "16G")
	}
	if cfg.VM.DiskSize != "100G" {
		t.Errorf("VM.DiskSize = %q, want %q", cfg.VM.DiskSize, "100G")
	}
	if cfg.SSH.User != "dev" {
		t.Errorf("SSH.User = %q, want %q", cfg.SSH.User, "dev")
	}
	if cfg.SSH.HostPort != 3333 {
		t.Errorf("SSH.HostPort = %d, want 3333", cfg.SSH.HostPort)
	}
	if cfg.SSH.ConnectTimeout != 60 {
		t.Errorf("SSH.ConnectTimeout = %d, want 60", cfg.SSH.ConnectTimeout)
	}

	vol, ok := cfg.Volumes["workspace"]
	if !ok {
		t.Fatal("missing workspace volume")
	}
	if vol.Host != "/tmp/workspace" {
		t.Errorf("Volume.Host = %q, want %q", vol.Host, "/tmp/workspace")
	}
	if vol.Guest != "/workspace" {
		t.Errorf("Volume.Guest = %q, want %q", vol.Guest, "/workspace")
	}

	if cfg.Environment["MY_VAR"] != "hello" {
		t.Errorf("Environment[MY_VAR] = %q, want %q", cfg.Environment["MY_VAR"], "hello")
	}

	if !cfg.OpenCode.AutoStart {
		t.Error("OpenCode.AutoStart = false, want true")
	}
	if cfg.OpenCode.Model != "anthropic/claude-sonnet-4-5" {
		t.Errorf("OpenCode.Model = %q, want %q", cfg.OpenCode.Model, "anthropic/claude-sonnet-4-5")
	}
}

func TestValidateMissingName(t *testing.T) {
	dir := t.TempDir()
	pubKeyPath := filepath.Join(dir, "id_test.pub")
	os.WriteFile(pubKeyPath, []byte("ssh-ed25519 AAAA..."), 0644)
	identityPath := filepath.Join(dir, "id_test")
	os.WriteFile(identityPath, []byte("PRIVATE KEY"), 0600)

	tomlContent := `
schema_version = "0"

[vm]
# name is missing

[ssh]
public_key_file = "` + pubKeyPath + `"
identity_file = "` + identityPath + `"
`
	cfgPath := filepath.Join(dir, "bad.toml")
	os.WriteFile(cfgPath, []byte(tomlContent), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing vm.name, got nil")
	}
}

func TestValidateMissingPublicKey(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "id_test")
	os.WriteFile(identityPath, []byte("PRIVATE KEY"), 0600)

	tomlContent := `
schema_version = "0"

[vm]
name = "test"

[ssh]
identity_file = "` + identityPath + `"
`
	cfgPath := filepath.Join(dir, "bad.toml")
	os.WriteFile(cfgPath, []byte(tomlContent), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing ssh.public_key_file, got nil")
	}
}

func TestExpandEnvVars(t *testing.T) {
	os.Setenv("MB_TEST_VAR", "hello-world")
	defer os.Unsetenv("MB_TEST_VAR")

	result := ExpandEnvVars("value=${MB_TEST_VAR}")
	if result != "value=hello-world" {
		t.Errorf("ExpandEnvVars = %q, want %q", result, "value=hello-world")
	}
}

func TestExpandEnvVarsMissing(t *testing.T) {
	os.Unsetenv("MB_NONEXISTENT_VAR")

	result := ExpandEnvVars("value=${MB_NONEXISTENT_VAR}")
	if result != "value=" {
		t.Errorf("ExpandEnvVars = %q, want %q", result, "value=")
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input string
		want  string
	}{
		{"~/foo/bar", filepath.Join(home, "foo/bar")},
		{"/absolute/path", "/absolute/path"},
		{"", ""},
	}

	for _, tt := range tests {
		got := ExpandPath(tt.input)
		if got != tt.want {
			t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestReadPublicKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pub")
	content := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... test@machine"
	os.WriteFile(path, []byte(content), 0644)

	got, err := ReadPublicKey(path)
	if err != nil {
		t.Fatalf("ReadPublicKey() error: %v", err)
	}
	if got != content {
		t.Errorf("ReadPublicKey() = %q, want %q", got, content)
	}
}

func TestReadPublicKeyMissing(t *testing.T) {
	_, err := ReadPublicKey("/nonexistent/path/key.pub")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestExpandEnvMap(t *testing.T) {
	os.Setenv("MB_KEY1", "val1")
	os.Setenv("MB_KEY2", "val2")
	defer os.Unsetenv("MB_KEY1")
	defer os.Unsetenv("MB_KEY2")

	input := map[string]string{
		"A": "${MB_KEY1}",
		"B": "${MB_KEY2}",
		"C": "literal",
	}

	result := ExpandEnvMap(input)
	if result["A"] != "val1" {
		t.Errorf("A = %q, want %q", result["A"], "val1")
	}
	if result["B"] != "val2" {
		t.Errorf("B = %q, want %q", result["B"], "val2")
	}
	if result["C"] != "literal" {
		t.Errorf("C = %q, want %q", result["C"], "literal")
	}
}
