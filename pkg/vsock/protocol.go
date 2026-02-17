// Package vsock implements the host-side client for communicating with
// stereosd inside a StereOS guest VM over the vsock control plane.
//
// The wire format is newline-delimited JSON (ndjson). Each message has
// a "type" field that determines how the payload is interpreted. This
// matches the protocol defined in the stereosd project.
package vsock

import "encoding/json"

// MessageType identifies the kind of control plane message.
type MessageType string

const (
	// Host -> Guest messages
	MsgPing         MessageType = "ping"
	MsgInjectSecret MessageType = "inject_secret"
	MsgMount        MessageType = "mount"
	MsgShutdown     MessageType = "shutdown"
	MsgGetHealth    MessageType = "get_health"
	MsgSetConfig    MessageType = "set_config"

	// Guest -> Host messages
	MsgPong      MessageType = "pong"
	MsgLifecycle MessageType = "lifecycle"
	MsgAck       MessageType = "ack"
	MsgHealth    MessageType = "health"
)

// Envelope is the top-level wire format for all messages.
type Envelope struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// NewEnvelope creates an envelope with the given type and payload.
func NewEnvelope(msgType MessageType, payload any) (*Envelope, error) {
	env := &Envelope{Type: msgType}
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		env.Payload = data
	}
	return env, nil
}

// DecodePayload unmarshals the envelope's payload into the given target.
func (e *Envelope) DecodePayload(target any) error {
	if e.Payload == nil {
		return nil
	}
	return json.Unmarshal(e.Payload, target)
}

// LifecycleState represents the current state of the StereOS instance.
type LifecycleState string

const (
	StateBooting  LifecycleState = "booting"
	StateReady    LifecycleState = "ready"
	StateHealthy  LifecycleState = "healthy"
	StateDegraded LifecycleState = "degraded"
	StateShutdown LifecycleState = "shutdown"
)

// SecretPayload is the payload for inject_secret messages.
type SecretPayload struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Mode  uint32 `json:"mode,omitempty"`
}

// MountPayload is the payload for mount messages.
type MountPayload struct {
	Tag       string `json:"tag"`
	GuestPath string `json:"guest_path"`
	FSType    string `json:"fs_type"`
	ReadOnly  bool   `json:"read_only,omitempty"`
}

// ShutdownPayload is the payload for shutdown messages.
type ShutdownPayload struct {
	Reason string `json:"reason,omitempty"`
}

// AckPayload is the payload for ack messages.
type AckPayload struct {
	ReplyTo MessageType `json:"reply_to"`
	OK      bool        `json:"ok"`
	Error   string      `json:"error,omitempty"`
}

// LifecyclePayload is the payload for lifecycle messages.
type LifecyclePayload struct {
	State   LifecycleState `json:"state"`
	Message string         `json:"message,omitempty"`
}

// AgentStatusPayload represents the runtime state of a single agent harness.
type AgentStatusPayload struct {
	Name     string `json:"name"`
	Running  bool   `json:"running"`
	Session  string `json:"session,omitempty"`
	Restarts int    `json:"restarts"`
	Error    string `json:"error,omitempty"`
}

// HealthPayload is the payload for health messages.
type HealthPayload struct {
	State  LifecycleState       `json:"state"`
	Agents []AgentStatusPayload `json:"agents,omitempty"`
	Uptime int64                `json:"uptime_seconds"`
}

// ConfigPayload is the payload for set_config messages.
type ConfigPayload struct {
	// Content is the raw jcard.toml content to write inside the guest.
	Content string `json:"content"`
}
