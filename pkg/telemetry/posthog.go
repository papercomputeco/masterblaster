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

// WithContext returns a copy of ctx with the telemetry client attached.
func WithContext(ctx context.Context, c *PosthogClient) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

// FromContext retrieves the PosthogClient from a context. Returns nil if absent.
func FromContext(ctx context.Context) *PosthogClient {
	c, _ := ctx.Value(contextKey{}).(*PosthogClient)
	return c
}

// PosthogClient wraps the PostHog SDK for anonymous CLI telemetry.
// All capture methods are nil-safe: calling them on a nil *PosthogClient is a no-op.
type PosthogClient struct {
	client     posthog.Client
	uniqueID   string
	isFirstRun bool
	version    string
}

// NewPosthogClient creates a new telemetry client.
// Returns nil when activated is false, skipping the PostHog connection and
// UUID file creation entirely. Nil-safe methods make this transparent to callers.
func NewPosthogClient(activated bool, version string) *PosthogClient {
	if !activated {
		return nil
	}

	client, err := posthog.NewWithConfig(
		writeOnlyPublicPosthogKey,
		posthog.Config{
			Endpoint: posthogEndpoint,
		},
	)
	if err != nil {
		return nil
	}

	uniqueID, isFirstRun, _ := getOrCreateUniqueID()

	return &PosthogClient{
		client:     client,
		uniqueID:   uniqueID,
		isFirstRun: isFirstRun,
		version:    version,
	}
}

// Done flushes pending events and closes the client.
func (p *PosthogClient) Done() {
	if p == nil {
		return
	}
	_ = p.client.Close()
}

func (p *PosthogClient) baseProperties() posthog.Properties {
	return posthog.NewProperties().
		Set("version", p.version).
		Set("os", runtime.GOOS).
		Set("arch", runtime.GOARCH)
}

// CaptureInstall tracks first-time installs.
func (p *PosthogClient) CaptureInstall() {
	if p == nil || !p.isFirstRun {
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
	if p == nil {
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
	if p == nil {
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
	if p == nil {
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
	if p == nil {
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
	if p == nil {
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
	if p == nil {
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
