package forwarder

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/jr200-labs/nats-otlp-forwarder/internal/logging"
	"github.com/jr200-labs/nats-otlp-forwarder/internal/metrics"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// Options holds runtime options overridable via CLI flags.
type Options struct {
	LogLevel  string
	LogFormat string
}

func DefaultOptions() *Options {
	return &Options{
		LogLevel:  "info",
		LogFormat: "json",
	}
}

// Start reads the config file and runs the forwarder until an OS interrupt.
func Start(configPath string, opts *Options) error {
	// CLI log flags take precedence over config-file values at this stage.
	logging.Setup(opts.LogLevel, opts.LogFormat == "human")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Re-apply logging from config if CLI used defaults
	if opts.LogLevel == "info" && cfg.Server.LogLevel != "" {
		logging.Setup(cfg.Server.LogLevel, cfg.Server.LogFormat == "human")
	}

	zap.L().Info("starting nats-otlp-forwarder",
		zap.String("service", cfg.Service.Name),
		zap.String("version", cfg.Service.Version))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return runWithContext(ctx, cfg)
}

// runWithContext wires up NATS, metrics, subscriptions, and blocks until ctx is cancelled.
func runWithContext(ctx context.Context, cfg *Config) error {
	m := metrics.New()
	health := metrics.NewHealthChecker()

	metricsSrv := metrics.NewServer(cfg.Server.MetricsPort, health)
	metricsSrv.Start()
	defer metricsSrv.Stop()

	nc, err := connectNATS(cfg, m, health)
	if err != nil {
		return fmt.Errorf("connect NATS: %w", err)
	}
	defer drainNATS(nc)

	health.SetNATSConnected(true)
	m.NATSConnected.Set(1)

	handler := NewHandler(cfg.Collector, m)

	if err := subscribeAll(ctx, nc, cfg.NATS.Subscriptions, handler); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	health.SetSubscribed(true)

	zap.L().Info("forwarder ready; waiting for messages")
	<-ctx.Done()
	zap.L().Info("shutting down")
	return nil
}

func connectNATS(cfg *Config, m *metrics.Metrics, health *metrics.HealthChecker) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			health.SetNATSConnected(false)
			m.NATSConnected.Set(0)
			if err != nil {
				zap.L().Warn("NATS disconnected", zap.Error(err))
			}
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			health.SetNATSConnected(true)
			m.NATSConnected.Set(1)
			m.NATSReconnects.Inc()
			zap.L().Info("NATS reconnected", zap.String("addr", c.ConnectedAddr()))
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			health.SetNATSConnected(false)
			m.NATSConnected.Set(0)
			zap.L().Info("NATS connection closed")
		}),
		nats.ErrorHandler(func(_ *nats.Conn, sub *nats.Subscription, err error) {
			if sub != nil {
				zap.L().Error("NATS async error", zap.String("subject", sub.Subject), zap.Error(err))
			} else {
				zap.L().Error("NATS async error", zap.Error(err))
			}
		}),
	}

	if cfg.NATS.ConnectionName != "" {
		opts = append(opts, nats.Name(cfg.NATS.ConnectionName))
	}
	if cfg.NATS.CredsFile != "" {
		opts = append(opts, nats.UserCredentials(cfg.NATS.CredsFile))
	}

	zap.L().Info("connecting to NATS", zap.String("url", cfg.NATS.URL))
	return nats.Connect(cfg.NATS.URL, opts...)
}

func drainNATS(nc *nats.Conn) {
	if nc == nil {
		return
	}
	if err := nc.Drain(); err != nil {
		zap.L().Warn("error draining NATS connection", zap.Error(err))
	}
}

// subscribeAll subscribes the handler to each configured subject. Missing
// (empty) subjects are skipped — the forwarder can operate on any subset
// of the three signals.
func subscribeAll(ctx context.Context, nc *nats.Conn, subs SubscriptionsConf, h *Handler) error {
	subscribe := func(subject string, signal Signal) error {
		if subject == "" {
			return nil
		}
		_, err := nc.Subscribe(subject, func(msg *nats.Msg) {
			h.Forward(ctx, signal, msg.Data)
		})
		if err != nil {
			return fmt.Errorf("subscribe %s: %w", subject, err)
		}
		zap.L().Info("subscribed", zap.String("signal", string(signal)), zap.String("subject", subject))
		return nil
	}

	if err := subscribe(subs.Traces, SignalTraces); err != nil {
		return err
	}
	if err := subscribe(subs.Logs, SignalLogs); err != nil {
		return err
	}
	if err := subscribe(subs.Metrics, SignalMetrics); err != nil {
		return err
	}
	return nil
}
