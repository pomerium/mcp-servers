package server

import (
	"os"
	"strings"
	"testing"
)

func TestGetEnvByPrefix(t *testing.T) {
	testEnvs := map[string]string{
		"TEST_VAR1":     "value1",
		"TEST_VAR2":     "value2",
		"OTHER_VAR":     "othervalue",
		"TEST_NESTED_X": "nestedvalue",
		"NOTTEST_VAR":   "nottest",
	}

	// Set environment variables
	for k, v := range testEnvs {
		t.Setenv(k, v)
	}

	tests := []struct {
		name     string
		prefix   string
		expected map[string]string
	}{
		{
			name:   "TEST_ prefix",
			prefix: "TEST_",
			expected: map[string]string{
				"VAR1":     "value1",
				"VAR2":     "value2",
				"NESTED_X": "nestedvalue",
			},
		},
		{
			name:   "OTHER_ prefix",
			prefix: "OTHER_",
			expected: map[string]string{
				"VAR": "othervalue",
			},
		},
		{
			name:     "nonexistent prefix",
			prefix:   "NONEXISTENT_",
			expected: map[string]string{},
		},
		{
			name:   "empty prefix returns all vars",
			prefix: "",
			expected: func() map[string]string {
				result := make(map[string]string)
				for _, env := range os.Environ() {
					parts := strings.SplitN(env, "=", 2)
					if len(parts) == 2 {
						result[parts[0]] = parts[1]
					}
				}
				return result
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getEnvByPrefix(tt.prefix)

			// For empty prefix test, just check that our test vars are present
			if tt.prefix == "" {
				for expectedKey, expectedValue := range testEnvs {
					if result[expectedKey] != expectedValue {
						t.Errorf("expected %s=%s, got %s", expectedKey, expectedValue, result[expectedKey])
					}
				}
				return
			}

			// For other tests, check exact match
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d items, got %d", len(tt.expected), len(result))
			}

			for expectedKey, expectedValue := range tt.expected {
				if result[expectedKey] != expectedValue {
					t.Errorf("expected %s=%s, got %s", expectedKey, expectedValue, result[expectedKey])
				}
			}

			// Verify prefix is actually removed
			for key := range result {
				if strings.HasPrefix(key, tt.prefix) && tt.prefix != "" {
					t.Errorf("prefix %s was not removed from key %s", tt.prefix, key)
				}
			}
		})
	}
}
