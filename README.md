# redline

[![CI](https://github.com/khancepts101/redline/actions/workflows/ci.yml/badge.svg)](https://github.com/khancepts101/redline/actions/workflows/ci.yml) [![Go Report Card](https://goreportcard.com/badge/github.com/khancepts101/redline)](https://goreportcard.com/report/github.com/khancepts101/redline) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**redline** is one Go SDK for request **R**ate, **E**rrors, and **D**uration, Sentry-compatible GlitchTip error capture, and provisioned Grafana dashboards. It adds HTTP middleware, gRPC interceptors, and batch-job instrumentation without locking applications to a hosted observability vendor. Its differentiator is a built-in cardinality watchdog: it catches label explosions before Prometheus runs out of memory.

```text
 HTTP / gRPC service ── redline SDK ─┬─ Prometheus ── Grafana
        batch job ─── Pushgateway ───┤       │ exemplars (trace_id)
                                    └─ GlitchTip (same trace_id)
```

## 60-second demo

```sh
git clone https://github.com/khancepts101/redline.git
cd redline
docker compose up --build
```

Open [Grafana](http://localhost:3000) (`admin` / `admin`) and choose **Redline / RED Overview**. The load generator starts automatically, so rate, errors, duration, and middleware-overhead panels populate within a minute. Prometheus is at `:9090`, the demo at `:8080`, Pushgateway at `:9091`, and GlitchTip at `:8000`.

GlitchTip needs its normal first-user/project setup. Copy the project's DSN into `GLITCHTIP_DSN` and restart the demo containers. The official Sentry Go client speaks GlitchTip's Sentry-compatible ingestion protocol; no custom error client is involved.

## Import and go

```go
r, _ := redline.New(redline.Config{Service: "billing", DSN: os.Getenv("GLITCHTIP_DSN")})
http.Handle("/invoices/{id}", r.HTTP("/invoices/{id}", invoices))
```

Always pass a route template, not a raw path containing user IDs. HTTP status, gRPC code, method, route, and transport become bounded labels. For gRPC, pass `r.UnaryServerInterceptor()` and `r.StreamServerInterceptor()` to `grpc.NewServer`.

### Metrics → trace → error

When an active OpenTelemetry span is present, duration observations carry its trace ID as a Prometheus exemplar. Grafana can display that exemplar beside a latency spike. Panic events sent to GlitchTip carry the identical `trace_id` tag, letting an operator search the exact error. The SDK consumes standard OTel context and is backend-independent; an application may export spans through any OTLP-compatible pipeline. A demo Tempo collector is intentionally a v2 item—the datasource provision includes the exemplar mapping so it can be connected without changing application code.

## Batch jobs

```go
job := r.Job("nightly-rollup", func(ctx context.Context) error { return rollup(ctx) })
err := job.Run(ctx)
```

Set `PushgatewayURL` to push runs, failures, duration, and `redline_job_last_run_timestamp_seconds` when the process exits. Pushgateway retains metrics forever: absence does **not** mean a job is healthy or removed. Alert on the timestamp (the included rule does), and call `job.Delete()` during intentional decommissioning. OTLP metric export is the planned v2 path because it avoids Pushgateway persistence semantics.

## Cardinality guard

`cardinality-guard` polls Prometheus's TSDB status endpoint, emits series count and growth per metric, and logs warning/hard threshold breaches with a relabel-drop suggestion. Configure `-warn`, `-hard`, and `-interval`. This targets a common failure mode that generic middleware leaves entirely to operators.

## Adaptive capture and overhead

Below `Sampling.FullCaptureRPS`, every panic is captured. Above it, capture changes to `1/N`, driven by the SDK's measured one-second request rate. Request metrics always remain complete. `redline_middleware_overhead_seconds` separately measures recording work after the handler returns.

Reproduce performance with `go test ./benchmarks -bench . -benchmem`. We do not publish invented latency percentiles: CI records Go benchmark time/op and allocations, while live p50/p99 come from the supplied overhead histogram. Release notes should record hardware, Go version, and benchmark output before claiming a number.

## Why not just …?

- **Self-hosted Sentry?** GlitchTip offers the compatible error-ingestion features used here with a smaller operational footprint. Sentry remains more capable when session replay or its full APM UI matters.
- **Grafana Cloud / AWS AMP+AMG?** Managed products reduce maintenance and are usually the right choice at larger scale. Redline prioritizes low cost, data ownership, and a transparent stack for small teams, at the honest cost of operating it yourself.

## Scope

Redline is not full Sentry: it has no session replay or complete APM UI. It is single-workspace/single-team by design, not multi-tenant. It targets roughly 1–100 services, not installations with millions of active series. See [ARCHITECTURE.md](ARCHITECTURE.md) and [CONTRIBUTING.md](CONTRIBUTING.md).

Licensed under MIT.
