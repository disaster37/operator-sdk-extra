package helper

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetWatchNamespaceFromEnv(t *testing.T) {
	tests := []struct {
		name          string
		envValue      string
		expectedError bool
	}{
		{
			name:          "namespace set",
			envValue:      "my-namespace",
			expectedError: false,
		},
		{
			name:          "namespace not set",
			envValue:      "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				_ = os.Setenv("WATCH_NAMESPACES", tt.envValue)
			}

			ns, err := GetWatchNamespaceFromEnv()
			_ = os.Unsetenv("WATCH_NAMESPACES")

			if !tt.expectedError {
				assert.Equal(t, ns, tt.envValue)
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestGetKubeClientTimeoutFromEnv(t *testing.T) {
	tests := []struct {
		name            string
		envValue        string
		expectedTimeout time.Duration
		expectedError   bool
	}{
		{
			name:            "timeout set",
			envValue:        "30s",
			expectedTimeout: 30 * time.Second,
			expectedError:   false,
		},
		{
			name:            "timeout not set",
			envValue:        "",
			expectedTimeout: 30 * time.Second,
			expectedError:   false,
		},
		{
			name:            "invalid timeout",
			envValue:        " invalid",
			expectedTimeout: 0,
			expectedError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				_ = os.Setenv("KUBE_CLIENT_TIMEOUT", tt.envValue)
			}

			timeout, err := GetKubeClientTimeoutFromEnv()
			_ = os.Unsetenv("KUBE_CLIENT_TIMEOUT")

			if !tt.expectedError {
				assert.Equal(t, timeout, tt.expectedTimeout)
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
