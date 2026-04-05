package main

import (
	"fmt"
	"os"

	"github.com/jr200/nats-otlp-forwarder/internal/forwarder"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func newServeCmd() *cobra.Command {
	opts := forwarder.DefaultOptions()

	cmd := &cobra.Command{
		Use:   "serve [flags] config.yaml",
		Short: "Start the OTLP-over-NATS forwarder service",
		Long: `Start the forwarder service. Reads a single YAML config file defining
NATS connection details, subscription subjects, and the OTel Collector
endpoint. Subscribes to each configured subject and forwards received
OTLP protobuf payloads to the collector via HTTP POST.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			defer func() { _ = zap.L().Sync() }()

			if err := forwarder.Start(args[0], opts); err != nil {
				fmt.Fprintf(os.Stderr, "[service stderr]: %v\n", err)
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.LogLevel, "log-level", opts.LogLevel, "log level: debug, info, warn, error")
	flags.StringVar(&opts.LogFormat, "log-format", opts.LogFormat, "log format: json, human")

	return cmd
}
