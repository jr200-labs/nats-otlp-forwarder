package forwarder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestLoadConfig_Valid(t *testing.T) {
	path := writeConfig(t, `
service:
  name: test
  version: "1.0.0"
nats:
  url: nats://localhost:4222
  subscriptions:
    traces: test.otlp.traces
    logs: test.otlp.logs
    metrics: test.otlp.metrics
collector:
  endpoint: http://localhost:4318
  timeout: 5s
  max_retries: 3
  retry_backoff: 200ms
server:
  metrics_port: 9090
`)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "nats://localhost:4222", cfg.NATS.URL)
	assert.Equal(t, "test.otlp.traces", cfg.NATS.Subscriptions.Traces)
	assert.Equal(t, "http://localhost:4318", cfg.Collector.Endpoint)
	assert.Equal(t, 3, cfg.Collector.MaxRetries)
	assert.Equal(t, 9090, cfg.Server.MetricsPort)
}

func TestLoadConfig_DefaultsApplied(t *testing.T) {
	path := writeConfig(t, `
nats:
  url: nats://localhost:4222
  subscriptions:
    traces: test.otlp.traces
collector:
  endpoint: http://localhost:4318
`)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Server.MetricsPort)
	assert.Equal(t, 2, cfg.Collector.MaxRetries)
	assert.Equal(t, "info", cfg.Server.LogLevel)
}

func TestLoadConfig_MissingNATSURL(t *testing.T) {
	path := writeConfig(t, `
nats:
  subscriptions:
    traces: test.otlp.traces
collector:
  endpoint: http://localhost:4318
`)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nats.url")
}

func TestLoadConfig_NoSubscriptions(t *testing.T) {
	path := writeConfig(t, `
nats:
  url: nats://localhost:4222
collector:
  endpoint: http://localhost:4318
`)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one")
}

func TestLoadConfig_MissingCollectorEndpoint(t *testing.T) {
	path := writeConfig(t, `
nats:
  url: nats://localhost:4222
  subscriptions:
    traces: test.otlp.traces
`)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collector.endpoint")
}

func TestLoadConfig_NegativeRetries(t *testing.T) {
	path := writeConfig(t, `
nats:
  url: nats://localhost:4222
  subscriptions:
    traces: test.otlp.traces
collector:
  endpoint: http://localhost:4318
  max_retries: -1
`)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_retries")
}
