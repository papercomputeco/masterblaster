package ssh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/papercomputeco/masterblaster/pkg/ssh"
)

func TestGenerateKeyPair(t *testing.T) {
	dir := t.TempDir()

	privPath, pubKey, err := ssh.GenerateKeyPair(dir, "test-sandbox")
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	// Verify private key file exists with correct permissions
	info, err := os.Stat(privPath)
	if err != nil {
		t.Fatalf("private key file not found: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("private key permissions = %o, want 0600", info.Mode().Perm())
	}

	// Verify private key is in PEM format
	privData, err := os.ReadFile(privPath)
	if err != nil {
		t.Fatalf("reading private key: %v", err)
	}
	if !strings.Contains(string(privData), "BEGIN OPENSSH PRIVATE KEY") {
		t.Error("private key is not in OpenSSH PEM format")
	}

	// Verify public key file exists
	pubPath := filepath.Join(dir, ssh.SSHPubKeyFilename)
	pubInfo, err := os.Stat(pubPath)
	if err != nil {
		t.Fatalf("public key file not found: %v", err)
	}
	if pubInfo.Mode().Perm() != 0644 {
		t.Errorf("public key permissions = %o, want 0644", pubInfo.Mode().Perm())
	}

	// Verify public key format
	if !strings.HasPrefix(pubKey, "ssh-ed25519 ") {
		t.Errorf("public key does not start with 'ssh-ed25519 ': %s", pubKey)
	}

	// Verify public key file content matches returned string
	pubData, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatalf("reading public key: %v", err)
	}
	if string(pubData) != pubKey {
		t.Error("public key file content does not match returned public key")
	}
}

func TestSSHKeyPath(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		dir := t.TempDir()
		keyPath := filepath.Join(dir, ssh.SSHKeyFilename)
		if err := os.WriteFile(keyPath, []byte("dummy"), 0600); err != nil {
			t.Fatal(err)
		}
		got := ssh.SSHKeyPath(dir)
		if got != keyPath {
			t.Errorf("SSHKeyPath() = %q, want %q", got, keyPath)
		}
	})

	t.Run("missing", func(t *testing.T) {
		dir := t.TempDir()
		got := ssh.SSHKeyPath(dir)
		if got != "" {
			t.Errorf("SSHKeyPath() = %q, want empty", got)
		}
	})
}
