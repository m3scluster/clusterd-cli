SHELL:=bash
BIN:=mesos-cli
VERSION:=$(shell git describe --tags --abbrev=0 2>/dev/null || { if [ -f .version ]; then cat .version; else echo dev; fi; })
BUILD_ID:=$(shell git rev-parse --short HEAD 2>/dev/null || echo "$(shell date +%s)")
GO ?=go
PLUGIN_DIR:=$(shell pwd)/plugins

.PHONY: all
all: $(BIN)

.PHONY: $(BIN)
$(BIN): FORCE
	$(GO) build -o $(BIN) ./cmd/mesos-cli

.PHONY: test
test: FORCE
	$(GO) test ./...

.PHONY: lint
lint: FORCE
	$(GO) vet ./...

.PHONY: fmt
fmt: FORCE
	$(GO) fmt ./...

.PHONY: clean
clean: FORCE
	rm -f $(BIN)

.PHONY: deps
deps: FORCE
	$(GO) mod tidy

.PHONY: version
version:
	@echo "Version: ($(VERSION)) Build: ($(BUILD_ID))"

FORCE:
