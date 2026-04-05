#!/bin/sh
set -e

CONFIG_FILE="${CONFIG_FILE:-/config/config.yaml}"

# If the first argument is a known subcommand, pass through directly.
case "${1:-}" in
    serve|version)
        echo "[CMD] nats-otlp-forwarder $*"
        exec nats-otlp-forwarder "$@"
        ;;
esac

# Default: run serve with the configured config file
echo "[CMD] nats-otlp-forwarder serve ${CONFIG_FILE}"
exec nats-otlp-forwarder serve "${CONFIG_FILE}" "$@"
