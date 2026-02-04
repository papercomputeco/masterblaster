package config

import (
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

// envVarPattern matches ${VAR_NAME} patterns in strings.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// ExpandEnvVars replaces ${VAR_NAME} references with their values from the
// host environment. Unknown variables expand to empty string.
func ExpandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := envVarPattern.FindStringSubmatch(match)[1]
		return os.Getenv(varName)
	})
}

// ExpandPath resolves ~ to the user's home directory and cleans the path.
func ExpandPath(path string) string {
	if path == "" {
		return path
	}

	if strings.HasPrefix(path, "~/") || path == "~" {
		u, err := user.Current()
		if err != nil {
			return path
		}
		if path == "~" {
			return u.HomeDir
		}
		return filepath.Join(u.HomeDir, path[2:])
	}

	return filepath.Clean(path)
}

// ExpandEnvMap expands environment variable references in a string map's values.
func ExpandEnvMap(m map[string]string) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = ExpandEnvVars(v)
	}
	return result
}
