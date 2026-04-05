package forwarder

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jr200-labs/nats-otlp-forwarder/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// freshMetrics returns a Metrics with a fresh (non-global) registry so tests
// don't collide via the default prometheus.MustRegister.
func freshMetrics(t *testing.T) *metrics.Metrics {
	t.Helper()
	// Replace the default registerer for the duration of the test
	prev := prometheus.DefaultRegisterer
	reg := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = reg
	t.Cleanup(func() { prometheus.DefaultRegisterer = prev })
	return metrics.New()
}

func newHandler(t *testing.T, url string, maxRetries int) *Handler {
	return NewHandler(CollectorConfig{
		Endpoint:     url,
		Timeout:      2 * time.Second,
		MaxRetries:   maxRetries,
		RetryBackoff: 10 * time.Millisecond,
	}, freshMetrics(t))
}

func TestForward_HappyPath(t *testing.T) {
	received := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/traces", r.URL.Path)
		assert.Equal(t, "application/x-protobuf", r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := newHandler(t, srv.URL, 2)
	payload := []byte("fake-protobuf-bytes")

	h.Forward(context.Background(), SignalTraces, payload)

	select {
	case got := <-received:
		assert.Equal(t, payload, got)
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not receive payload")
	}
}

func TestForward_4xxNoRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	h := newHandler(t, srv.URL, 3)
	h.Forward(context.Background(), SignalTraces, []byte("x"))

	// 4xx => single attempt, no retries
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestForward_5xxRetriesThenFails(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := newHandler(t, srv.URL, 2)
	h.Forward(context.Background(), SignalTraces, []byte("x"))

	// maxRetries=2 => 1 initial attempt + 2 retries = 3 calls total
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestForward_5xxThenRecovers(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := newHandler(t, srv.URL, 5)
	h.Forward(context.Background(), SignalTraces, []byte("x"))

	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestForward_NetworkError(t *testing.T) {
	// Start a server and close it to ensure the URL dials fail.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	h := newHandler(t, url, 1)
	h.Forward(context.Background(), SignalTraces, []byte("x"))
	// We don't count exact HTTP calls here (connection refused is a transport error),
	// just assert it doesn't panic and completes in time.
}

func TestForward_ContextCancelled(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := newHandler(t, srv.URL, 5)
	h.retryBackoff = 200 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	h.Forward(ctx, SignalTraces, []byte("x"))
	// Should give up part-way through retries
	require.LessOrEqual(t, atomic.LoadInt32(&calls), int32(2))
}

func TestForward_CorrectURLPerSignal(t *testing.T) {
	urls := make(chan string, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urls <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := newHandler(t, srv.URL, 0)
	h.Forward(context.Background(), SignalTraces, []byte("x"))
	h.Forward(context.Background(), SignalLogs, []byte("x"))
	h.Forward(context.Background(), SignalMetrics, []byte("x"))

	close(urls)
	var got []string
	for u := range urls {
		got = append(got, u)
	}
	assert.ElementsMatch(t, []string{"/v1/traces", "/v1/logs", "/v1/metrics"}, got)
}
