# Makefile for b3tty

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get

# Name of your binary
BINARY_NAME=b3tty

# Main package path
MAIN_PACKAGE=.

# Read version from VERSION file
PACKAGE_VERSION=$(shell cat VERSION)

# Build flags
BUILD_FLAGS=-v -ldflags="-X 'github.com/cmmorrow/b3tty/cmd.Version=$(PACKAGE_VERSION)'"
# BUILD_FLAGS=-v -ldflags="-X 'github.com/cmmorrow/b3tty/cmd.Version=test'"
.PHONY: all setup format format-check lint client build test test-race clean run deps tidy build-linux build-freebsd build-mac

all: test build

setup:
	cd src/client && bun install

format:
	cd src/client && bun run format

format-check:
	cd src/client && bun run format:check

lint:
	cd src/client && bun run lint

client:
	rm -rf src/dist
	bun build src/client/terminal.ts --outdir src/dist --target browser --splitting --minify \
		--entry-naming=terminal.min.js --chunk-naming="[name]-[hash].chunk.js"
	cp src/client/node_modules/@xterm/xterm/css/xterm.css src/dist/xterm.min.css

build: client
	@echo "Building $(BINARY_NAME) version $(PACKAGE_VERSION)"
	$(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME) $(MAIN_PACKAGE)

# src/dist is gitignored (generated build output), and the src package's
# go:embed dist directive requires that directory to exist at compile time —
# so test/test-race depend on client the same way build does, even though
# they don't otherwise touch the client bundle.
test: client
	$(GOTEST) -v ./...
	cd src/client && bun test

# Runs the Go test suite with the data race detector enabled. Not part of the
# default `test` target since -race adds noticeable overhead; run this
# separately (and in CI) to catch races like the one guarded by StateMu.
test-race: client
	$(GOTEST) -race -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf src/dist

run: build
	./$(BINARY_NAME)

deps:
	$(GOGET) ./...

tidy:
	$(GOCMD) mod tidy

# Cross compilation
build-linux: client
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME)_linux $(MAIN_PACKAGE)

build-freebsd: client
	CGO_ENABLED=0 GOOS=freebsd GOARCH=arm64 $(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME) $(MAIN_PACKAGE)

build-mac: client
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME)_mac $(MAIN_PACKAGE)
