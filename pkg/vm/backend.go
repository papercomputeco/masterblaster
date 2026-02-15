package vm

import (
	"context"
	"time"
)

// Backend abstracts the VM hypervisor. QEMU today, Apple Virtualization.framework
// and KVM/libvirt in the future. All VM operations go through this interface.
type Backend interface {
	// Up creates and starts a new sandbox VM from the given instance configuration.
	// It creates the VM directory, disk overlay, boots the hypervisor, waits for
	// stereosd to become ready, injects secrets, and mounts shared directories.
	Up(ctx context.Context, inst *Instance) error

	// Down gracefully stops the VM. Sends shutdown via vsock to stereosd first,
	// then falls back to ACPI shutdown via QMP, then force kill after timeout.
	Down(ctx context.Context, inst *Instance, timeout time.Duration) error

	// ForceDown immediately terminates the VM process.
	ForceDown(ctx context.Context, inst *Instance) error

	// Destroy stops the VM (if running) and removes all its on-disk resources.
	Destroy(ctx context.Context, inst *Instance) error

	// Status returns the current state of the VM.
	Status(ctx context.Context, inst *Instance) (State, error)

	// List returns all known VM instances by scanning the vms directory.
	List(ctx context.Context) ([]*Instance, error)

	// LoadInstance reads a persisted VM instance by name.
	LoadInstance(name string) (*Instance, error)
}
