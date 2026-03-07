// Package telemetry provides anonymous usage tracking for the mb CLI.
// Telemetry is opt-out via --disable-telemetry or MB_DISABLE_TELEMETRY=1.
package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

var telemetryFilePath = filepath.Join(os.Getenv("HOME"), ".mb", "telemetry.json")

type userTelemetryConfig struct {
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

	var teleData userTelemetryConfig
	if err := json.Unmarshal(data, &teleData); err != nil || teleData.ID == "" {
		return createTelemetryUUID()
	}

	return teleData.ID, false, nil
}

func createTelemetryUUID() (string, bool, error) {
	newUUID := uuid.New().String()

	teleData := userTelemetryConfig{
		ID:           newUUID,
		FirstRunDate: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(teleData)
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
