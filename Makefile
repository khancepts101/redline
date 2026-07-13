.PHONY: test lint bench build demo

test:
	go test ./...

lint:
	go vet ./...
	go run ./cmd/promrule-lint ./alerts

bench:
	go test ./benchmarks -run '^$$' -bench . -benchmem

build:
	go build ./cmd/...

demo:
	docker compose up --build
