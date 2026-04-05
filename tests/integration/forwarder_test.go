//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/jr200-labs/nats-otlp-forwarder/internal/forwarder"
	"github.com/jr200-labs/nats-otlp-forwarder/internal/metrics"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startHandler returns a Handler configured against the fake collector,
// using an isolated Prometheus registry so tests don't collide.
func startHandler(t *testing.T, collectorURL string, maxRetries int) *forwarder.Handler {
	t.Helper()
	prev := prometheus.DefaultRegisterer
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
	t.Cleanup(func() { prometheus.DefaultRegisterer = prev })
	m := metrics.New()
	return forwarder.NewHandler(forwarder.CollectorConfig{
		Endpoint:     collectorURL,
		Timeout:      2 * time.Second,
		MaxRetries:   maxRetries,
		RetryBackoff: 20 * time.Millisecond,
	}, m)
}

// subscribeForwarder subscribes to the three subjects and wires them to the handler.
// Returns a subscribe func mirroring forwarder.subscribeAll logic (can't import unexported).
func subscribeForwarder(t *testing.T, nc *nats.Conn, h *forwarder.Handler, traces, logs, metricsSubj string) {
	t.Helper()
	_, err := nc.Subscribe(traces, func(msg *nats.Msg) {
		h.Forward(t.Context(), forwarder.SignalTraces, msg.Data)
	})
	require.NoError(t, err)
	_, err = nc.Subscribe(logs, func(msg *nats.Msg) {
		h.Forward(t.Context(), forwarder.SignalLogs, msg.Data)
	})
	require.NoError(t, err)
	_, err = nc.Subscribe(metricsSubj, func(msg *nats.Msg) {
		h.Forward(t.Context(), forwarder.SignalMetrics, msg.Data)
	})
	require.NoError(t, err)
	require.NoError(t, nc.Flush())
}

func TestForwarder_HappyPathAllSignals(t *testing.T) {
	ns := StartNATSServer(t)
	fc := NewFakeCollector(t)

	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	h := startHandler(t, fc.Server.URL, 2)
	subscribeForwarder(t, nc, h, "test.otlp.traces", "test.otlp.logs", "test.otlp.metrics")

	require.NoError(t, nc.Publish("test.otlp.traces", []byte("trace-bytes")))
	require.NoError(t, nc.Publish("test.otlp.logs", []byte("log-bytes")))
	require.NoError(t, nc.Publish("test.otlp.metrics", []byte("metric-bytes")))
	require.NoError(t, nc.Flush())

	got := map[string][]byte{}
	for i := 0; i < 3; i++ {
		select {
		case req := <-fc.Received:
			got[req.Path] = req.Body
			assert.Equal(t, "application/x-protobuf", req.ContentType)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for request %d", i+1)
		}
	}
	assert.Equal(t, []byte("trace-bytes"), got["/v1/traces"])
	assert.Equal(t, []byte("log-bytes"), got["/v1/logs"])
	assert.Equal(t, []byte("metric-bytes"), got["/v1/metrics"])
}

func TestForwarder_Collector500ThenRecovers(t *testing.T) {
	ns := StartNATSServer(t)
	fc := NewFakeCollector(t)
	// Fail first 2 attempts, succeed on 3rd
	fc.SetResponder(func(_ string, n int) int {
		if n < 3 {
			return http.StatusInternalServerError
		}
		return http.StatusOK
	})

	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	h := startHandler(t, fc.Server.URL, 5)
	subscribeForwarder(t, nc, h, "test.otlp.traces", "x.logs", "x.metrics")

	require.NoError(t, nc.Publish("test.otlp.traces", []byte("bytes")))
	require.NoError(t, nc.Flush())

	// Drain 3 requests (2 x 500 + 1 x 200)
	for i := 0; i < 3; i++ {
		select {
		case <-fc.Received:
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout on attempt %d", i+1)
		}
	}
	assert.Equal(t, 3, fc.CallCount("/v1/traces"))
}

func TestForwarder_Collector400NoRetry(t *testing.T) {
	ns := StartNATSServer(t)
	fc := NewFakeCollector(t)
	fc.SetResponder(func(string, int) int { return http.StatusBadRequest })

	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	h := startHandler(t, fc.Server.URL, 5)
	subscribeForwarder(t, nc, h, "test.otlp.traces", "x.logs", "x.metrics")

	require.NoError(t, nc.Publish("test.otlp.traces", []byte("bytes")))
	require.NoError(t, nc.Flush())

	select {
	case <-fc.Received:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	// Give a moment for any spurious retries
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 1, fc.CallCount("/v1/traces"))
}
