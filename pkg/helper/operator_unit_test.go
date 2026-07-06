package helper

import (
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

// Mock logger that captures output
type mockLogger struct {
	messages []string
}

func (m *mockLogger) Init(info logr.RuntimeInfo) {}

func (m *mockLogger) Info(level int, msg string, keysAndValues ...interface{}) {
	m.messages = append(m.messages, msg)
}

func (m *mockLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	m.messages = append(m.messages, msg)
}

func (m *mockLogger) Enabled(level int) bool {
	return true
}

func (m *mockLogger) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return m
}

func (m *mockLogger) WithName(name string) logr.LogSink {
	return m
}

func TestPrintVersion(t *testing.T) {
	logger := &mockLogger{}

	PrintVersion(logr.New(logger), "metrics_addr", "probe_addr")

	// Check that logger received messages
	assert.Len(t, logger.messages, 4)
	assert.Contains(t, logger.messages, "Binary info ")
	assert.Contains(t, logger.messages, "Address ")
}

func TestGetWatchNamespaceFromEnvUnit(t *testing.T) {
	t.Run("namespace is set", func(t *testing.T) {
		t.Setenv("WATCH_NAMESPACES", "test-namespace")

		ns, err := GetWatchNamespaceFromEnv()
		assert.NoError(t, err)
		assert.Equal(t, "test-namespace", ns)
	})

	t.Run("namespace is not set", func(t *testing.T) {
		_ = os.Unsetenv("WATCH_NAMESPACES")

		_, err := GetWatchNamespaceFromEnv()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "WATCH_NAMESPACES must be set")
	})
}

func TestGetKubeClientTimeoutFromEnvUnit(t *testing.T) {
	t.Run("timeout is set with valid duration", func(t *testing.T) {
		t.Setenv("KUBE_CLIENT_TIMEOUT", "60s")

		timeout, err := GetKubeClientTimeoutFromEnv()
		assert.NoError(t, err)
		assert.Equal(t, 60*time.Second, timeout)
	})

	t.Run("timeout is not set - default value", func(t *testing.T) {
		_ = os.Unsetenv("KUBE_CLIENT_TIMEOUT")

		timeout, err := GetKubeClientTimeoutFromEnv()
		assert.NoError(t, err)
		assert.Equal(t, 30*time.Second, timeout)
	})

	t.Run("timeout is set with invalid duration", func(t *testing.T) {
		t.Setenv("KUBE_CLIENT_TIMEOUT", "invalid-duration")

		_, err := GetKubeClientTimeoutFromEnv()
		assert.Error(t, err)
	})
}
