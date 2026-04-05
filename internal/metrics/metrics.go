package metrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

const (
	namespace = "nats_otlp_forwarder"

	// Label names
	labelSignal = "signal"
	labelStatus = "status"
	labelCode   = "code"

	// Status values for forward result
	StatusSuccess = "success"
	StatusError   = "error"
)

// Metrics holds all Prometheus metrics for the forwarder.
type Metrics struct {
	MessagesReceived  *prometheus.CounterVec
	MessagesForwarded *prometheus.CounterVec
	PayloadBytes      *prometheus.HistogramVec
	ForwardDuration   *prometheus.HistogramVec
	HTTPErrors        *prometheus.CounterVec
	NATSConnected     prometheus.Gauge
	NATSReconnects    prometheus.Counter
}

// New creates and registers all Prometheus metrics.
func New() *Metrics {
	m := &Metrics{
		MessagesReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "messages_received_total",
				Help:      "Total OTLP messages received from NATS, by signal.",
			},
			[]string{labelSignal},
		),
		MessagesForwarded: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "messages_forwarded_total",
				Help:      "Total OTLP messages forwarded to the collector, by signal and status.",
			},
			[]string{labelSignal, labelStatus},
		),
		PayloadBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "payload_bytes",
				Help:      "OTLP message payload size in bytes, by signal.",
				Buckets:   []float64{1024, 4096, 16384, 65536, 262144, 1048576},
			},
			[]string{labelSignal},
		),
		ForwardDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "forward_duration_seconds",
				Help:      "Duration of forwarding a message to the collector, by signal and status.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{labelSignal, labelStatus},
		),
		HTTPErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_errors_total",
				Help:      "HTTP errors returned by the collector, by signal and status code.",
			},
			[]string{labelSignal, labelCode},
		),
		NATSConnected: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "nats_connected",
				Help:      "Current NATS connection state (1 = connected, 0 = disconnected).",
			},
		),
		NATSReconnects: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "nats_reconnects_total",
				Help:      "Total NATS reconnection events.",
			},
		),
	}

	prometheus.MustRegister(
		m.MessagesReceived,
		m.MessagesForwarded,
		m.PayloadBytes,
		m.ForwardDuration,
		m.HTTPErrors,
		m.NATSConnected,
		m.NATSReconnects,
	)

	return m
}

// Server runs an HTTP server exposing /metrics, /healthz, /readyz.
type Server struct {
	httpServer *http.Server
}

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 5 * time.Second
)

// NewServer creates the metrics HTTP server on the given port.
func NewServer(port int, health *HealthChecker) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", health.LivenessHandler())
	mux.HandleFunc("/readyz", health.ReadinessHandler())

	return &Server{
		httpServer: &http.Server{
			Addr:              fmt.Sprintf(":%d", port),
			Handler:           mux,
			ReadHeaderTimeout: readHeaderTimeout,
		},
	}
}

// Start begins listening in a goroutine. Returns immediately.
func (s *Server) Start() {
	go func() {
		zap.L().Info("metrics server listening", zap.String("addr", s.httpServer.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Error("metrics server error", zap.Error(err))
		}
	}()
}

// Stop gracefully shuts down the metrics server.
func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := s.httpServer.Shutdown(ctx); err != nil {
		zap.L().Error("metrics server shutdown error", zap.Error(err))
	}
}
