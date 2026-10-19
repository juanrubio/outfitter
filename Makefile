GO ?= go
BIN := outfitter

.PHONY: all build test vet lint docs-check check run clean race

all: check build

build:
	$(GO) build -o $(BIN) ./cmd/outfitter

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

lint:
	$(GO) tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint run

docs-check:
	$(GO) run ./internal/tools/docscheck .

check: vet lint docs-check test

run:
	$(GO) run ./cmd/outfitter $(ARGS)

clean:
	rm -f $(BIN)
