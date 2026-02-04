package vm

import (
	"path/filepath"

	"github.com/paper-compute-co/masterblaster/internal/config"
)

// State represents the lifecycle state of a VM.
type State string

const (
	StateCreated State = "created"
	StateRunning State = "running"
	StateStopped State = "stopped"
	StateError   State = "error"
)

// Instance represents a single VM and its on-disk resources.
type Instance struct {
	Name       string `json:"name"`
	Dir        string `json:"dir"`         // ~/.mb/vms/<name>/
	PID        int    `json:"pid"`         // QEMU process ID
	QMPSocket  string `json:"qmp_socket"`  // Path to QMP unix socket
	SSHAddress string `json:"ssh_address"` // host:port for SSH
	VMState    State  `json:"state"`
}

// DiskPath returns the path to this VM's qcow2 overlay disk.
func (inst *Instance) DiskPath() string {
	return filepath.Join(inst.Dir, "disk.qcow2")
}

// EFIVarsPath returns the path to this VM's writable EFI variable store.
func (inst *Instance) EFIVarsPath() string {
	return filepath.Join(inst.Dir, "efi-vars.fd")
}

// CloudInitISO returns the path to this VM's cloud-init NoCloud ISO.
func (inst *Instance) CloudInitISO() string {
	return filepath.Join(inst.Dir, "cloud-init.iso")
}

// SerialLogPath returns the path to this VM's serial console log.
func (inst *Instance) SerialLogPath() string {
	return filepath.Join(inst.Dir, "serial.log")
}

// PIDFilePath returns the path to the QEMU PID file.
func (inst *Instance) PIDFilePath() string {
	return filepath.Join(inst.Dir, "qemu.pid")
}

// CreateOpts holds the parameters needed to create a new VM.
type CreateOpts struct {
	Name        string
	BaseImage   string // Path to base qcow2
	DiskSize    string // e.g. "40G"
	CPUs        int
	Memory      string // e.g. "8G"
	CloudInit   CloudInitData
	Volumes     map[string]config.VolumeMount
	SSHHostPort int    // Host port to forward to guest:22
	Network     string // "user" or "vmnet"
}

// CloudInitData holds the values rendered into cloud-init templates.
type CloudInitData struct {
	InstanceID     string
	Hostname       string
	User           string
	SSHPublicKey   string
	Packages       []string
	Environment    map[string]string
	OpenCode       config.OpenCodeConfig
	ConfigJSON     string // Rendered OpenCode config.json content, if any
	OpenCodeBinary []byte // Pre-downloaded OpenCode binary to embed on the ISO
}
