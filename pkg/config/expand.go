package config

import (
	"os"
	"regexp"
)

// envVarPattern matches ${VAR_NAME} patterns in strings.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// expandEnvVars replaces ${VAR_NAME} references with their values from the
// host environment. Unknown variables expand to empty string.
func expandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := envVarPattern.FindStringSubmatch(match)[1]
		return os.Getenv(varName)
	})
}

// expandEnvMap expands environment variable references in a string map's values.
func expandEnvMap(m map[string]string) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = expandEnvVars(v)
	}
	return result
}
