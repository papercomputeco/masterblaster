// Package telemetry provides anonymous usage tracking for the mb CLI.
// Telemetry is opt-out via --disable-telemetry, MB_DISABLE_TELEMETRY=1,
// or automatic CI environment detection.
package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// telemetryFilePath is the path to the persistent telemetry state file.
// It is a var so tests can override it.
var telemetryFilePath = filepath.Join(os.Getenv("HOME"), ".mb", "telemetry.json")

// State is the persistent telemetry state stored in ~/.mb/telemetry.json.
type State struct {
	ID           string `json:"id"`
	FirstRunDate string `json:"first_run_date,omitempty"`
}

// getOrCreateUniqueID reads or creates the user's anonymous unique ID.
// Returns the ID, whether this is the first run, and any error.
func getOrCreateUniqueID() (string, bool, error) {
	if _, err := os.Stat(telemetryFilePath); os.IsNotExist(err) {
		return createTelemetryUUID()
	}

	data, err := os.ReadFile(telemetryFilePath)
	if err != nil {
		return createTelemetryUUID()
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil || state.ID == "" {
		return createTelemetryUUID()
	}

	return state.ID, false, nil
}

func createTelemetryUUID() (string, bool, error) {
	newUUID := uuid.New().String()

	state := State{
		ID:           newUUID,
		FirstRunDate: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(state)
	if err != nil {
		return "", true, fmt.Errorf("creating telemetry data: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(telemetryFilePath), 0755); err != nil {
		return "", true, fmt.Errorf("creating telemetry directory: %w", err)
	}

	if err := os.WriteFile(telemetryFilePath, data, 0600); err != nil {
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
