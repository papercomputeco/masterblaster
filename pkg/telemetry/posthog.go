package telemetry

import (
	"context"
	"runtime"

	"github.com/posthog/posthog-go"
)

var (
	// Write-only key, safe to embed in source.
	writeOnlyPublicPosthogKey = "phc_xCBFT1jetPLJIRGTqJ9Q0YuG5I1jhXtUkxYkNBEAXRY"
	posthogEndpoint           = "https://us.i.posthog.com"
)

// contextKey is an unexported type for context keys in this package.
type contextKey struct{}

// Key is the context key used to store and retrieve the PosthogClient.
var Key = contextKey{}

// FromContext retrieves the PosthogClient from a context. Returns nil if absent.
func FromContext(ctx context.Context) *PosthogClient {
	if t, ok := ctx.Value(Key).(*PosthogClient); ok {
		return t
	}
	return nil
}

// PosthogClient wraps the PostHog SDK for anonymous CLI telemetry.
type PosthogClient struct {
	client     posthog.Client
	activated  bool
	uniqueID   string
	isFirstRun bool
	version    string
}

// NewPosthogClient creates a new telemetry client.
// If activated is false, all capture methods are no-ops.
func NewPosthogClient(activated bool, version string) *PosthogClient {
	client, err := posthog.NewWithConfig(
		writeOnlyPublicPosthogKey,
		posthog.Config{
			Endpoint: posthogEndpoint,
		},
	)
	if err != nil {
		// Config is static; this should never happen.
		return &PosthogClient{activated: false}
	}

	uniqueID, isFirstRun, _ := getOrCreateUniqueID()

	return &PosthogClient{
		client:     client,
		activated:  activated,
		uniqueID:   uniqueID,
		isFirstRun: isFirstRun,
		version:    version,
	}
}

// Done flushes pending events and closes the client.
func (p *PosthogClient) Done() {
	if p.client != nil {
		_ = p.client.Close()
	}
}

func (p *PosthogClient) baseProperties() posthog.Properties {
	return posthog.NewProperties().
		Set("version", p.version).
		Set("os", runtime.GOOS).
		Set("arch", runtime.GOARCH)
}

// CaptureInstall tracks first-time installs.
func (p *PosthogClient) CaptureInstall() {
	if !p.activated || !p.isFirstRun {
		return
	}
	props := p.baseProperties().Set("event_type", "install")
	_ = p.client.Enqueue(posthog.Capture{
		DistinctId: p.uniqueID,
		Event:      "mb_cli_installed",
		Properties: props,
	})
}

// CaptureCommandRun tracks command usage for DAU calculation.
func (p *PosthogClient) CaptureCommandRun(command string) {
	if !p.activated {
		return
	}
	props := p.baseProperties().Set("command", command)
	_ = p.client.Enqueue(posthog.Capture{
		DistinctId: p.uniqueID,
		Event:      "mb_cli_command_run",
		Properties: props,
	})
}

// CaptureUp tracks sandbox creation.
func (p *PosthogClient) CaptureUp(mixtape string, success bool) {
	if !p.activated {
		return
	}
	props := p.baseProperties().
		Set("mixtape", mixtape).
		Set("success", success)
	_ = p.client.Enqueue(posthog.Capture{
		DistinctId: p.uniqueID,
		Event:      "mb_cli_sandbox_created",
		Properties: props,
	})
}

// CaptureDown tracks sandbox shutdown.
func (p *PosthogClient) CaptureDown(success bool) {
	if !p.activated {
		return
	}
	props := p.baseProperties().Set("success", success)
	_ = p.client.Enqueue(posthog.Capture{
		DistinctId: p.uniqueID,
		Event:      "mb_cli_sandbox_stopped",
		Properties: props,
	})
}

// CaptureSSH tracks SSH connections.
func (p *PosthogClient) CaptureSSH() {
	if !p.activated {
		return
	}
	_ = p.client.Enqueue(posthog.Capture{
		DistinctId: p.uniqueID,
		Event:      "mb_cli_ssh_connected",
		Properties: p.baseProperties(),
	})
}

// CapturePull tracks mixtape pulls.
func (p *PosthogClient) CapturePull(mixtape string, success bool) {
	if !p.activated {
		return
	}
	props := p.baseProperties().
		Set("mixtape", mixtape).
		Set("success", success)
	_ = p.client.Enqueue(posthog.Capture{
		DistinctId: p.uniqueID,
		Event:      "mb_cli_mixtape_pulled",
		Properties: props,
	})
}

// CaptureError tracks errors anonymously.
func (p *PosthogClient) CaptureError(command string, errType string) {
	if !p.activated {
		return
	}
	props := p.baseProperties().
		Set("command", command).
		Set("error_type", errType)
	_ = p.client.Enqueue(posthog.Capture{
		DistinctId: p.uniqueID,
		Event:      "mb_cli_error",
		Properties: props,
	})
}
