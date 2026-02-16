VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || cat version.txt 2>/dev/null || echo dev)
BINARY  := yg
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test lint clean race

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BINARY) ./cmd/yeager/

test:
	go test ./...

race:
	go test -race ./...

lint:
	golangci-lint run

clean:
	rm -f $(BINARY)
	go clean -testcache
