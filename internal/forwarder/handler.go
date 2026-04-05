package forwarder

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jr200-labs/nats-otlp-forwarder/internal/metrics"
	"go.uber.org/zap"
)

// Signal is the OTLP signal type carried on a NATS message.
type Signal string

const (
	SignalTraces  Signal = "traces"
	SignalLogs    Signal = "logs"
	SignalMetrics Signal = "metrics"
)

// Handler forwards a single OTLP payload from NATS to the collector via HTTP POST.
type Handler struct {
	client       *http.Client
	collectorURL string
	maxRetries   int
	retryBackoff time.Duration
	metrics      *metrics.Metrics
	logger       *zap.Logger
}

// NewHandler builds a handler from resolved config.
func NewHandler(cfg CollectorConfig, m *metrics.Metrics) *Handler {
	return &Handler{
		client:       &http.Client{Timeout: cfg.Timeout},
		collectorURL: cfg.Endpoint,
		maxRetries:   cfg.MaxRetries,
		retryBackoff: cfg.RetryBackoff,
		metrics:      m,
		logger:       zap.L(),
	}
}

// Forward sends the given OTLP protobuf bytes to the collector's HTTP endpoint
// for the given signal, retrying on 5xx/network errors up to MaxRetries.
func (h *Handler) Forward(ctx context.Context, signal Signal, data []byte) {
	signalLabel := string(signal)
	h.metrics.MessagesReceived.WithLabelValues(signalLabel).Inc()
	h.metrics.PayloadBytes.WithLabelValues(signalLabel).Observe(float64(len(data)))

	start := time.Now()
	url := h.collectorURL + "/v1/" + signalLabel

	var lastErr error
	for attempt := 0; attempt <= h.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				h.recordError(signalLabel, start, ctx.Err())
				return
			case <-time.After(h.retryBackoff * time.Duration(attempt)):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			h.recordError(signalLabel, start, err)
			return
		}
		req.Header.Set("Content-Type", "application/x-protobuf")

		resp, err := h.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_ = resp.Body.Close()

		switch {
		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("collector returned %d", resp.StatusCode)
			continue
		case resp.StatusCode >= 400:
			// Client error: don't retry, log and drop.
			h.metrics.HTTPErrors.WithLabelValues(signalLabel, strconv.Itoa(resp.StatusCode)).Inc()
			h.metrics.MessagesForwarded.WithLabelValues(signalLabel, metrics.StatusError).Inc()
			h.metrics.ForwardDuration.WithLabelValues(signalLabel, metrics.StatusError).Observe(time.Since(start).Seconds())
			h.logger.Warn("collector rejected payload",
				zap.String("signal", signalLabel),
				zap.Int("status", resp.StatusCode))
			return
		default:
			// 2xx
			h.metrics.MessagesForwarded.WithLabelValues(signalLabel, metrics.StatusSuccess).Inc()
			h.metrics.ForwardDuration.WithLabelValues(signalLabel, metrics.StatusSuccess).Observe(time.Since(start).Seconds())
			return
		}
	}

	h.recordError(signalLabel, start, lastErr)
}

func (h *Handler) recordError(signalLabel string, start time.Time, err error) {
	h.metrics.MessagesForwarded.WithLabelValues(signalLabel, metrics.StatusError).Inc()
	h.metrics.ForwardDuration.WithLabelValues(signalLabel, metrics.StatusError).Observe(time.Since(start).Seconds())
	h.logger.Warn("forward failed after retries",
		zap.String("signal", signalLabel),
		zap.Error(err))
}
