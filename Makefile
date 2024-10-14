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

all: test build

build:
	@echo "Building $(BINARY_NAME) version $(PACKAGE_VERSION)"
	$(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME) $(MAIN_PACKAGE)

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

run: build
	./$(BINARY_NAME)

deps:
	$(GOGET) ./...

tidy:
	$(GOCMD) mod tidy

# Cross compilation
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME)_linux $(MAIN_PACKAGE)

build-freebsd:
	CGO_ENABLED=0 GOOS=freebsd GOARCH=arm64 $(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME) $(MAIN_PACKAGE)

build-mac:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME)_mac $(MAIN_PACKAGE)

.PHONY: all build test clean run deps tidy build-linux build-freebsd build-mac
