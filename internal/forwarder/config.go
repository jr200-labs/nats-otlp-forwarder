package forwarder

import (
	"fmt"
	"os"
	"time"

	"github.com/goccy/go-yaml"
)

// Config is the on-disk YAML configuration for the forwarder.
type Config struct {
	Service   ServiceConfig   `yaml:"service"`
	NATS      NATSConfig      `yaml:"nats"`
	Collector CollectorConfig `yaml:"collector"`
	Server    ServerConfig    `yaml:"server"`
}

type ServiceConfig struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type NATSConfig struct {
	URL            string            `yaml:"url"`
	CredsFile      string            `yaml:"creds_file"`
	ConnectionName string            `yaml:"connection_name"`
	Subscriptions  SubscriptionsConf `yaml:"subscriptions"`
}

// SubscriptionsConf holds the NATS subjects to subscribe to, one per signal.
type SubscriptionsConf struct {
	Traces  string `yaml:"traces"`
	Logs    string `yaml:"logs"`
	Metrics string `yaml:"metrics"`
}

type CollectorConfig struct {
	Endpoint     string        `yaml:"endpoint"`
	Timeout      time.Duration `yaml:"timeout"`
	MaxRetries   int           `yaml:"max_retries"`
	RetryBackoff time.Duration `yaml:"retry_backoff"`
}

type ServerConfig struct {
	LogLevel    string `yaml:"log_level"`
	LogFormat   string `yaml:"log_format"`
	MetricsPort int    `yaml:"metrics_port"`
}

// LoadConfig reads and validates a YAML config from disk.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config YAML: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Service: ServiceConfig{
			Name:    "nats-otlp-forwarder",
			Version: "dev",
		},
		Collector: CollectorConfig{
			Timeout:      10 * time.Second,
			MaxRetries:   2,
			RetryBackoff: 100 * time.Millisecond,
		},
		Server: ServerConfig{
			LogLevel:    "info",
			LogFormat:   "json",
			MetricsPort: 8080,
		},
	}
}

func (c *Config) validate() error {
	if c.NATS.URL == "" {
		return fmt.Errorf("nats.url is required")
	}
	if c.NATS.Subscriptions.Traces == "" && c.NATS.Subscriptions.Logs == "" && c.NATS.Subscriptions.Metrics == "" {
		return fmt.Errorf("nats.subscriptions: at least one of traces/logs/metrics must be set")
	}
	if c.Collector.Endpoint == "" {
		return fmt.Errorf("collector.endpoint is required")
	}
	if c.Collector.Timeout <= 0 {
		return fmt.Errorf("collector.timeout must be positive")
	}
	if c.Collector.MaxRetries < 0 {
		return fmt.Errorf("collector.max_retries must be non-negative")
	}
	return nil
}
