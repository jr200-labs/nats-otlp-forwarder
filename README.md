# nats-otlp-forwarder

Forward OTLP telemetry from NATS subjects to an OpenTelemetry Collector.

## What it does

The forwarder subscribes to three NATS subjects (one per OTLP signal: traces, logs, metrics) and forwards each received payload to an OTel Collector's HTTP endpoint (`/v1/traces`, `/v1/logs`, `/v1/metrics`) as `application/x-protobuf`.

It performs **no deserialisation** — messages are forwarded as raw bytes. Clients are expected to publish OTLP protobuf payloads (the standard OTLP wire format).

## Configuration

Single YAML file (see `docker/config/config.yaml` for a template):

```yaml
service:
  name: nats-otlp-forwarder
  version: "0.1.0"

nats:
  url: nats://nats:4222
  creds_file: /secrets/nats-otlp-forwarder.creds
  connection_name: nats-otlp-forwarder
  subscriptions:
    traces:  "myapp.otlp.traces"
    logs:    "myapp.otlp.logs"
    metrics: "myapp.otlp.metrics"

collector:
  endpoint: http://otel-collector:4318
  timeout: 10s
  max_retries: 2
  retry_backoff: 100ms

server:
  log_level: info
  log_format: json
  metrics_port: 8080
```

Empty subscription subjects are skipped — you can run the forwarder with any subset of signals.

## Metrics

Prometheus metrics are exposed at `http://<host>:<metrics_port>/metrics`:

| Metric | Type | Labels |
|---|---|---|
| `nats_otlp_forwarder_messages_received_total` | counter | `signal` |
| `nats_otlp_forwarder_messages_forwarded_total` | counter | `signal, status` |
| `nats_otlp_forwarder_payload_bytes` | histogram | `signal` |
| `nats_otlp_forwarder_forward_duration_seconds` | histogram | `signal, status` |
| `nats_otlp_forwarder_http_errors_total` | counter | `signal, code` |
| `nats_otlp_forwarder_nats_connected` | gauge | — |
| `nats_otlp_forwarder_nats_reconnects_total` | counter | — |

## Failure behaviour

- **Network / 5xx**: retried up to `max_retries` with linear backoff. On final failure: logged and dropped, error counter incremented.
- **4xx**: dropped immediately (payload is malformed), `http_errors_total` incremented.
- **NATS disconnect**: standard nats.go auto-reconnect, infinite retries. Messages published during disconnect are lost (core pub/sub, at-most-once).
- **Collector unreachable at startup**: forwarder starts anyway; failures surface via metrics.

## Development

```sh
make test              # unit tests
make test-integration  # integration tests (embedded NATS + httptest collector)
make lint              # go vet + golangci-lint
make build             # static binary in ./build/
```

## Release

```sh
make bump PART=patch   # or minor/major
make release           # runs lint+test+integration, then tag + GH release
```

## License

See LICENSE.
