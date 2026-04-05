package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "nats-otlp-forwarder",
		Short: "Forward OTLP telemetry from NATS subjects to an OTel Collector",
		Long: `nats-otlp-forwarder subscribes to NATS subjects carrying OTLP protobuf
telemetry (traces, logs, metrics) and forwards each payload to an
OTel Collector via HTTP POST. Designed for bridging browser telemetry
that cannot directly reach the collector (e.g. due to ad blockers).`,
		SilenceUsage: true,
	}

	root.AddCommand(newServeCmd())
	root.AddCommand(newVersionCmd())

	return root
}
