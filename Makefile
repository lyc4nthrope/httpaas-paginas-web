# Shortcuts for building, running and checking the service. Everything is plain Go:
# `make` is a convenience, not a requirement (Windows equivalents: docs/PLAN.md §7, P5).

BIN := httpaas

.PHONY: build run simular test check vm-test clean

# One static binary with templates and static files embedded (go:embed).
build:
	CGO_ENABLED=0 go build -trimpath -o $(BIN) ./cmd/httpaas

run: build
	./$(BIN)

# Without VMs: every port answers through a fake (available from phase A5).
simular: build
	./$(BIN) -simular

test:
	go test -race -count=1 ./...

# What CI runs on every pull request: format, vet, tests and build.
check:
	@test -z "$$(gofmt -l .)" || { echo "Run gofmt on:"; gofmt -l .; exit 1; }
	go vet ./...
	go test -race -count=1 ./...
	go build ./cmd/httpaas

# Tests that need real VMs (files with //go:build vm).
vm-test:
	go test -tags vm -count=1 -v ./...

clean:
	rm -f $(BIN)
