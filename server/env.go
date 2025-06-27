package server

import (
	"os"
	"strings"
)

// getEnvByPrefix returns a map of environment variables that start with the given prefix
// The prefix is removed from the keys in the returned map
func getEnvByPrefix(prefix string) map[string]string {
	result := make(map[string]string)

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], prefix) {
			key := strings.TrimPrefix(parts[0], prefix)
			result[key] = parts[1]
		}
	}

	return result
}
