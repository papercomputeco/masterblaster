// Package vm provides VM lifecycle management including the Backend interface
// abstraction, instance types, state persistence, and QMP communication.
package vm

import (
	"path/filepath"

	"github.com/papercomputeco/masterblaster/pkg/config"
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
	// Name is the unique human-readable name for this sandbox.
	Name string `json:"name"`

	// Dir is the path to this VM's resource directory (~/.mb/vms/<name>/).
	Dir string `json:"dir"`

	// PID is the hypervisor process ID (e.g. QEMU PID).
	PID int `json:"pid"`

	// QMPSocket is the path to the QMP unix socket (QEMU only).
	QMPSocket string `json:"qmp_socket"`

	// VsockPort is the host-side port for vsock communication with stereosd.
	// For QEMU with TCP fallback, this is a TCP port on localhost.
	VsockPort int `json:"vsock_port"`

	// SSHPort is the host port forwarded to guest port 22.
	SSHPort int `json:"ssh_port"`

	// VMState is the current lifecycle state.
	VMState State `json:"state"`

	// Config is the jcard configuration for this instance.
	Config *config.JcardConfig `json:"-"`
}

// DiskPath returns the path to this VM's disk image.
func (inst *Instance) DiskPath() string {
	return filepath.Join(inst.Dir, "disk.raw")
}

// QCOWDiskPath returns the path to this VM's qcow2 overlay disk.
func (inst *Instance) QCOWDiskPath() string {
	return filepath.Join(inst.Dir, "disk.qcow2")
}

// EFIVarsPath returns the path to this VM's writable EFI variable store.
func (inst *Instance) EFIVarsPath() string {
	return filepath.Join(inst.Dir, "efi-vars.fd")
}

// SerialLogPath returns the path to this VM's serial console log.
func (inst *Instance) SerialLogPath() string {
	return filepath.Join(inst.Dir, "serial.log")
}

// PIDFilePath returns the path to the hypervisor PID file.
func (inst *Instance) PIDFilePath() string {
	return filepath.Join(inst.Dir, "qemu.pid")
}

// JcardPath returns the path to the saved jcard.toml for this instance.
func (inst *Instance) JcardPath() string {
	return filepath.Join(inst.Dir, "jcard.toml")
}
