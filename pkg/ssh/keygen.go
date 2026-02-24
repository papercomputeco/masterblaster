package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	gossh "golang.org/x/crypto/ssh"
)

const (
	// SSHKeyFilename is the name of the ephemeral private key file
	// stored in the VM directory.
	SSHKeyFilename = "ssh_key"

	// SSHPubKeyFilename is the name of the ephemeral public key file
	// stored in the VM directory.
	SSHPubKeyFilename = "ssh_key.pub"
)

// GenerateKeyPair creates a fresh Ed25519 SSH keypair in the given directory.
// It writes the private key (OpenSSH PEM format, mode 0600) and the public
// key (authorized_keys format, mode 0644) to <dir>/ssh_key and
// <dir>/ssh_key.pub respectively.
//
// Returns the absolute path to the private key and the public key string
// (suitable for injection into authorized_keys).
func GenerateKeyPair(dir, comment string) (privateKeyPath, publicKey string, err error) {
	// Generate Ed25519 keypair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generating Ed25519 key: %w", err)
	}

	// Marshal private key to OpenSSH PEM format
	privPEM, err := gossh.MarshalPrivateKey(privKey, comment)
	if err != nil {
		return "", "", fmt.Errorf("marshaling private key: %w", err)
	}
	privPEMBytes := pem.EncodeToMemory(privPEM)

	// Marshal public key to authorized_keys format
	sshPubKey, err := gossh.NewPublicKey(pubKey)
	if err != nil {
		return "", "", fmt.Errorf("creating SSH public key: %w", err)
	}
	pubKeyStr := string(gossh.MarshalAuthorizedKey(sshPubKey))

	// Write private key (mode 0600 — owner-only read/write)
	privateKeyPath = filepath.Join(dir, SSHKeyFilename)
	if err := os.WriteFile(privateKeyPath, privPEMBytes, 0600); err != nil {
		return "", "", fmt.Errorf("writing private key: %w", err)
	}

	// Write public key (mode 0644)
	pubKeyPath := filepath.Join(dir, SSHPubKeyFilename)
	if err := os.WriteFile(pubKeyPath, []byte(pubKeyStr), 0644); err != nil {
		// Clean up private key on failure
		_ = os.Remove(privateKeyPath)
		return "", "", fmt.Errorf("writing public key: %w", err)
	}

	return privateKeyPath, pubKeyStr, nil
}

// SSHKeyPath returns the path to the ephemeral SSH private key for a VM
// directory, or empty string if it doesn't exist.
func SSHKeyPath(vmDir string) string {
	path := filepath.Join(vmDir, SSHKeyFilename)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}
