// Package daemon implements the long-lived Masterblaster daemon service.
// The daemon manages VM sandbox lifecycles via a Backend, exposes a JSON-RPC
// API over a unix domain socket (~/.mb/mb.sock), and handles PID file
// management and liveness probes.
package daemon

// RPCMethod identifies an RPC call from the CLI to the daemon.
type RPCMethod string

const (
	MethodUp      RPCMethod = "up"
	MethodDown    RPCMethod = "down"
	MethodStatus  RPCMethod = "status"
	MethodDestroy RPCMethod = "destroy"
	MethodList    RPCMethod = "list"
	MethodPing    RPCMethod = "ping"
)

// Request is the wire format for CLI -> daemon RPC calls.
type Request struct {
	Method RPCMethod `json:"method"`

	// Up parameters
	Name       string `json:"name,omitempty"`
	ConfigPath string `json:"config_path,omitempty"`

	// Down/Destroy parameters
	Force bool `json:"force,omitempty"`

	// Status parameters
	All bool `json:"all,omitempty"`
}

// Response is the wire format for daemon -> CLI responses.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`

	// For status/list responses
	Sandboxes []SandboxInfo `json:"sandboxes,omitempty"`
}

// SandboxInfo is the public representation of a sandbox for CLI display.
type SandboxInfo struct {
	Name        string `json:"name"`
	State       string `json:"state"`
	Mixtape     string `json:"mixtape"`
	CPUs        int    `json:"cpus"`
	Memory      string `json:"memory"`
	SSHPort     int    `json:"ssh_port"`
	SSHAddress  string `json:"ssh_address"`
	VsockPort   int    `json:"vsock_port"`
	NetworkMode string `json:"network_mode"`
}
