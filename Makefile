.PHONY: test build tidy

test:
	go test ./...

build:
	go build -o dune-compute ./cmd/dune-compute

tidy:
	go mod tidy
