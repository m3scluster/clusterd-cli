SHELL:=bash
BIN:=mesos-cli
VERSION:=$(shell git describe --tags --abbrev=0 2>/dev/null || cat .version)
BUILD_ID:=$(shell git rev-parse --short HEAD 2>/dev/null || echo "$(shell date +%s)")
GO ?=go
PLUGIN_DIR:=$(shell pwd)/plugins

.PHONY: all
all: $(BIN)

.PHONY: $(BIN)
$(BIN): FORCE
	$(GO) build -o $(BIN) ./cmd/mesos-cli

.PHONY: test

.PHONY: lint
lint: FORCE
	$(GO) vet ./... 2>&1 | grep -v 'undefined: .*_test' > lint.out || true
	if [ -s lint.out ]; then echo "Linter warnings:"; cat lint.out; fi

.PHONY: fmt
fmt: FORCE
	$(GO) fmt ./...

.PHONY: clean
clean: FORCE
	rm -f $(BIN)

.PHONY: deps

.PHONY: version
version:
	@echo "Version: ($(VERSION)) Build: ($(BUILD_ID))"

FORCE:
