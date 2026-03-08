// Package telemetry provides anonymous usage tracking for the mb CLI.
// Telemetry is opt-out via --disable-telemetry flag, config, or automatic
// CI environment detection.
package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// telemetryFileName is the state file name within ~/.mb/.
const telemetryFileName = "telemetry.json"

// telemetryDir returns the directory for the telemetry state file.
// Resolved at call time so $HOME changes and os.UserHomeDir work correctly.
func telemetryDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".mb")
	}
	return filepath.Join(home, ".mb")
}

// telemetryFilePathOverride allows tests to override the state file path.
var telemetryFilePathOverride string

func resolvedTelemetryFilePath() string {
	if telemetryFilePathOverride != "" {
		return telemetryFilePathOverride
	}
	return filepath.Join(telemetryDir(), telemetryFileName)
}

// State is the persistent telemetry state stored in ~/.mb/telemetry.json.
type State struct {
	ID           string `json:"id"`
	FirstRunDate string `json:"first_run_date,omitempty"`
}

// getOrCreateUniqueID reads or creates the user's anonymous unique ID.
// Returns the ID, whether this is the first run, and any error.
func getOrCreateUniqueID() (string, bool, error) {
	fp := resolvedTelemetryFilePath()

	if _, err := os.Stat(fp); os.IsNotExist(err) {
		return createTelemetryUUID(fp)
	}

	data, err := os.ReadFile(fp)
	if err != nil {
		return createTelemetryUUID(fp)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil || state.ID == "" {
		return createTelemetryUUID(fp)
	}

	return state.ID, false, nil
}

func createTelemetryUUID(fp string) (string, bool, error) {
	newUUID := uuid.New().String()

	state := State{
		ID:           newUUID,
		FirstRunDate: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(state)
	if err != nil {
		return "", true, fmt.Errorf("creating telemetry data: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(fp), 0755); err != nil {
		return "", true, fmt.Errorf("creating telemetry directory: %w", err)
	}

	if err := os.WriteFile(fp, data, 0600); err != nil {
		return "", true, fmt.Errorf("writing telemetry file: %w", err)
	}

	return newUUID, true, nil
}

// ciEnvVars is the list of environment variables used to detect CI environments.
var ciEnvVars = []string{
	"CI",
	"GITHUB_ACTIONS",
	"GITLAB_CI",
	"CIRCLECI",
	"TRAVIS",
	"JENKINS_URL",
	"BUILDKITE",
	"CODEBUILD_BUILD_ID",
}

// IsCI returns true if the process appears to be running in a CI environment.
func IsCI() bool {
	for _, env := range ciEnvVars {
		if os.Getenv(env) != "" {
			return true
		}
	}
	return false
}
