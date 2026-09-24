BINARY  := bluetooth-widget
PKG     := ./cmd/bluetooth-widget
GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
TAGS    ?= $(shell pkg-config --exists gtk4-x11 x11 2>/dev/null || echo nox11)
LDFLAGS := -s -w -X main.version=$(VERSION)
PURE    := ./internal/bluetooth/... ./internal/config/... ./internal/order/... ./internal/autostart/... ./internal/logging/... ./internal/cli/... ./internal/audio/...

.PHONY: all build run test test-race test-integration vet fmt install uninstall clean

all: build

build:
	$(GO) build -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $(BINARY) $(PKG)

run: build
	./$(BINARY)

test:
	$(GO) test -tags '$(TAGS)' ./...

test-race:
	$(GO) test -race $(PURE)

test-integration:
	$(GO) test -tags 'integration $(TAGS)' -v -count=1 ./tests/integration/

vet:
	$(GO) vet -tags '$(TAGS)' ./...

fmt:
	gofmt -s -w cmd internal tests

install:
	./scripts/install.sh

uninstall:
	./scripts/uninstall.sh

clean:
	rm -f $(BINARY)
