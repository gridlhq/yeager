VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || cat version.txt 2>/dev/null || echo dev)
BINARY  := yg
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test lint clean race livefire livefire-offline

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BINARY) ./cmd/yeager/

test:
	go test ./...

race:
	go test -race ./...

lint:
	golangci-lint run

livefire: build
	YG_BINARY=$(CURDIR)/$(BINARY) go test -tags livefire -v -count=1 -timeout 30m ./test/livefire/...

livefire-offline: build
	YG_BINARY=$(CURDIR)/$(BINARY) LIVEFIRE_TAGS=@offline go test -tags livefire -v -count=1 -timeout 5m ./test/livefire/...

clean:
	rm -f $(BINARY)
	go clean -testcache
