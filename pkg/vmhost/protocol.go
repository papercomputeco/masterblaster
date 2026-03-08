// Package vmhost implements the control protocol between the mb daemon and
// per-VM vmhost child processes. Each vmhost process manages exactly one VM
// and exposes a JSON-RPC control socket (vmhost.sock) for lifecycle operations.
package vmhost

// Method identifies an RPC call from the daemon to a vmhost process.
type Method string

const (
	MethodStatus    Method = "status"
	MethodStop      Method = "stop"
	MethodForceStop Method = "force_stop"
	MethodInfo      Method = "info"
	MethodApply     Method = "apply"
)

// Request is the wire format for daemon -> vmhost RPC calls.
type Request struct {
	Method Method `json:"method"`

	// Stop parameters
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`

	// Apply parameters
	ConfigContent string            `json:"config_content,omitempty"` // serialized jcard.toml
	Secrets       map[string]string `json:"secrets,omitempty"`        // name -> value
}

// Response is the wire format for vmhost -> daemon responses.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`

	// Status/Info fields
	State   string `json:"state,omitempty"`
	SSHPort int    `json:"ssh_port,omitempty"`
	Backend string `json:"backend,omitempty"`
}
