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
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
	BaseImage    string    `json:"base_image"`
	SSHUser      string    `json:"ssh_user"`
	SSHHostPort  int       `json:"ssh_host_port"`
	CPUs         int       `json:"cpus"`
	Memory       string    `json:"memory"`
	ConfigPath   string    `json:"config_path,omitempty"`
	IdentityFile string    `json:"identity_file,omitempty"`
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
