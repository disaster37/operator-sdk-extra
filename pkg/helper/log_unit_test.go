package helper

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetLogrusLogLevelFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected logrus.Level
	}{
		{"trace level", "trace", logrus.TraceLevel},
		{"debug level", "debug", logrus.DebugLevel},
		{"info level", "info", logrus.InfoLevel},
		{"warn level", "warn", logrus.WarnLevel},
		{"error level", "error", logrus.ErrorLevel},
		{"panic level", "panic", logrus.PanicLevel},
		{"fatal level", "fatal", logrus.FatalLevel},
		{"unknown level", "unknown", logrus.InfoLevel},
		{"empty level", "", logrus.InfoLevel},
		{"case insensitive", "DEBUG", logrus.DebugLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("LOG_LEVEL", tt.envValue)
			} else {
				_ = os.Unsetenv("LOG_LEVEL")
			}

			result := GetLogrusLogLevelFromEnv()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetLogrusFormatterFromEnv(t *testing.T) {
	t.Run("json formatter", func(t *testing.T) {
		t.Setenv("LOG_FORMATTER", "json")

		result := GetLogrusFormatterFromEnv()
		_, isJSONFormatter := result.(*logrus.JSONFormatter)
		assert.True(t, isJSONFormatter)
	})

	t.Run("text formatter (default)", func(t *testing.T) {
		// Test with empty/missing env var
		_ = os.Unsetenv("LOG_FORMATTER")

		result := GetLogrusFormatterFromEnv()
		_, isTextFormatter := result.(*logrus.TextFormatter)
		assert.True(t, isTextFormatter)
	})

	t.Run("case insensitive json formatter", func(t *testing.T) {
		t.Setenv("LOG_FORMATTER", "JSON")

		result := GetLogrusFormatterFromEnv()
		_, isJSONFormatter := result.(*logrus.JSONFormatter)
		assert.True(t, isJSONFormatter)
	})
}
