package telemetry

// SetTelemetryFilePath overrides the telemetry state file path for testing.
// It returns the previous path so callers can restore it.
func SetTelemetryFilePath(path string) string {
	prev := telemetryFilePathOverride
	telemetryFilePathOverride = path
	return prev
}

// GetOrCreateUniqueID is an exported alias for testing.
func GetOrCreateUniqueID() (string, bool, error) {
	return getOrCreateUniqueID()
}
