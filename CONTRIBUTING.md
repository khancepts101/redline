# Contributing

Run `go test ./...`, `go vet ./...`, and `go run ./cmd/promrule-lint ./alerts` before submitting a change. Keep labels bounded and add allocation-visible benchmarks for hot-path changes.

Good first issues: add a bundled Tempo trace backend; implement token-bucket adaptive sampling; compare the benchmark with `promhttp`; add OTLP batch metric export; create a sampling-rate panel; add multi-tenant isolation as an explicitly opt-in v2 design.
