package helper

import (
	"bytes"
	"os"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestGetZapLogLevelFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		expectedLog zapcore.Level
	}{
		{
			name:        "debug level",
			envValue:    "debug",
			expectedLog: zapcore.DebugLevel,
		},
		{
			name:        "info level",
			envValue:    "info",
			expectedLog: zapcore.InfoLevel,
		},
		{
			name:        "warn level",
			envValue:    "warn",
			expectedLog: zapcore.WarnLevel,
		},
		{
			name:        "error level",
			envValue:    "error",
			expectedLog: zapcore.ErrorLevel,
		},
		{
			name:        "panic level",
			envValue:    "panic",
			expectedLog: zapcore.PanicLevel,
		},
		{
			name:        "unknown level",
			envValue:    "unknown",
			expectedLog: zapcore.InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("LOG_LEVEL", tt.envValue)
			defer os.Unsetenv("LOG_LEVEL")

			logLevel := GetZapLogLevelFromEnv()
			if logLevel != tt.expectedLog {
				t.Errorf("GetZapLogLevelFromEnv() = %v, want %v", logLevel, tt.expectedLog)
			}
		})
	}
}

func TestGetZapFormatterFromDev(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		expectedFormat zapcore.Encoder
	}{
		{
			name:           "json format",
			envValue:       "json",
			expectedFormat: zapcore.NewJSONEncoder(zapcore.EncoderConfig{}),
		},
		{
			name:           "console format",
			envValue:       "console",
			expectedFormat: zapcore.NewConsoleEncoder(zapcore.EncoderConfig{}),
		},
		{
			name:           "unknown format",
			envValue:       "unknown",
			expectedFormat: zapcore.NewConsoleEncoder(zapcore.EncoderConfig{}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("LOG_FORMATTER", tt.envValue)
			defer os.Unsetenv("LOG_FORMATTER")

			formatter := GetZapFormatterFromDev()
			actualBuff := bytes.NewBufferString("")
			zActual := zap.New(zapcore.NewCore(formatter, zapcore.AddSync(actualBuff), zap.DebugLevel))

			expectedBuff := bytes.NewBufferString("")
			zExpected := zap.New(zapcore.NewCore(tt.expectedFormat, zapcore.AddSync(expectedBuff), zap.DebugLevel))
			zActual.Info("test")
			zExpected.Info("test")
			zActual.Sync()
			zExpected.Sync()

			if actualBuff.String() != expectedBuff.String() {
				t.Errorf("GetZapFormatterFromDev() = %v, want %v", formatter, tt.expectedFormat)
			}
		})
	}
}
