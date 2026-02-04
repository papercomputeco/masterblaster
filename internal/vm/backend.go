package vm

import (
	"context"
	"time"
)

// Backend abstracts the VM hypervisor. QEMU today, Firecracker/Cloud Hypervisor/
// Apple Virtualization.framework tomorrow.
type Backend interface {
	// Create prepares VM resources (overlay disk, cloud-init ISO) but does not start.
	Create(ctx context.Context, opts CreateOpts) (*Instance, error)

	// Start boots the VM. Returns once QEMU process is running (not once guest is ready).
	Start(ctx context.Context, inst *Instance) error

	// Stop sends ACPI shutdown (graceful). Falls back to kill after timeout.
	Stop(ctx context.Context, inst *Instance, timeout time.Duration) error

	// Kill force-terminates the VM process.
	Kill(ctx context.Context, inst *Instance) error

	// Remove deletes all VM resources (disk overlay, cloud-init ISO, state, QMP socket).
	Remove(ctx context.Context, inst *Instance) error

	// Status returns the current state of the VM.
	Status(ctx context.Context, inst *Instance) (State, error)

	// List returns all known VM instances.
	List(ctx context.Context) ([]*Instance, error)

	// LoadInstance reads a persisted VM instance by name.
	LoadInstance(name string) (*Instance, error)

	// EnsureBaseImage validates that the base image exists and returns its path.
	EnsureBaseImage(ctx context.Context, image string) (string, error)
}
