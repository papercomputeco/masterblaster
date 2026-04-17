package vm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StateFile is the persistent metadata for a VM stored in state.json.
type StateFile struct {
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"created_at"`
	Mixtape     string    `json:"mixtape"`
	CPUs        int       `json:"cpus"`
	Memory      string    `json:"memory"`
	Disk        string    `json:"disk"`
	NetworkMode string    `json:"network_mode"`
	SSHPort     int       `json:"ssh_port"`
	VsockPort   int       `json:"vsock_port"`
	// VsockCID is the guest context ID for native AF_VSOCK control plane.
	// Set when ControlPlaneMode resolves to "vsock" at boot; zero when the
	// VM uses TCP hostfwd. Persisted so `mb down` after a daemon restart
	// can still address the guest.
	VsockCID   uint32 `json:"vsock_cid,omitempty"`
	ConfigPath string `json:"config_path,omitempty"`

	// SSHKeyPath is the path to the ephemeral SSH private key, relative
	// to the VM directory or absolute. Persisted so the key survives
	// daemon restarts.
	SSHKeyPath string `json:"ssh_key_path,omitempty"`

	// Backend identifies which Backend implementation owns this VM.
	Backend string `json:"backend,omitempty"`

	// PlatformData holds opaque backend-specific persistent state.
	// Each backend can use this to store identity or configuration data
	// that must survive across daemon restarts and up/down cycles.
	//
	// Examples:
	//   - Apple Virt: serialized vz.GenericMachineIdentifier (gives the
	//     guest a stable MAC address, EFI NVRAM, etc.)
	//   - QEMU: could store a generated machine UUID for SMBIOS identity
	//     (-smbios type=1,uuid=...) if needed in the future.
	PlatformData []byte `json:"platform_data,omitempty"`
}

// stateFilePath returns the path to state.json for a given VM directory.
func stateFilePath(vmDir string) string {
	return filepath.Join(vmDir, "state.json")
}

// saveState writes the state file to the VM directory.
func saveState(vmDir string, state *StateFile) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	path := stateFilePath(vmDir)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}
	return nil
}

// loadState reads the state file from the VM directory.
func loadState(vmDir string) (*StateFile, error) {
	data, err := os.ReadFile(stateFilePath(vmDir))
	if err != nil {
		return nil, fmt.Errorf("reading state file: %w", err)
	}
	var state StateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	return &state, nil
}

// LoadState reads the state file for this instance.
func (inst *Instance) LoadState() (*StateFile, error) {
	return loadState(inst.Dir)
}
