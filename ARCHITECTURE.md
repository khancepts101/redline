# Architecture

The SDK owns bounded RED labels and correlation. Prometheus remains the metric system of record, standard OpenTelemetry span context remains the tracing boundary, and the official Sentry client sends errors to GlitchTip. This separation keeps each sink replaceable.

The HTTP wrapper records response status without buffering bodies. Its defer path recovers panics, optionally reports a 500 or re-panics, then records counters and histograms. Duration exemplars and GlitchTip tags derive from the same OTel span context. gRPC unary and stream interceptors follow the same model.

Short-lived jobs use an isolated registry and Pushgateway. A last-run timestamp makes retained stale state detectable. The guard polls Prometheus rather than reading its storage, so it deploys independently.

Future work: OTLP metric push, Tempo in the bundled demo, token-bucket sampling, multi-instance rate aggregation, and automated relabel configuration generation.
