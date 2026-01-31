# Ralph Makefile
# Build and development commands

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
BINARY_NAME=ralph
MAIN_PATH=./cmd/ralph

# Version info (overridden during release)
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-s -w \
	-X github.com/arvesolland/ralph/internal/cli.Version=$(VERSION) \
	-X github.com/arvesolland/ralph/internal/cli.Commit=$(COMMIT) \
	-X github.com/arvesolland/ralph/internal/cli.BuildDate=$(BUILD_DATE)

.PHONY: all build build-dev test test-short test-integration test-verbose clean deps lint release-snapshot release-dry-run help

# Default target
all: build

# Build the binary with version info
build:
	CGO_ENABLED=0 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) $(MAIN_PATH)

# Build for development (faster, no version info)
build-dev:
	$(GOBUILD) -o $(BINARY_NAME) $(MAIN_PATH)

# Run all tests
test:
	$(GOTEST) -v ./...

# Run tests without long-running integration tests
test-short:
	$(GOTEST) -v -short ./...

# Run tests with race detection
test-race:
	$(GOTEST) -v -race ./...

# Run tests with coverage
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Run integration tests (requires Claude CLI or RALPH_MOCK_CLAUDE=1)
test-integration: build
	$(GOTEST) -v -tags=integration ./internal/integration/...

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME).exe
	rm -rf dist/
	rm -f coverage.out coverage.html

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Run linter (requires golangci-lint)
lint:
	golangci-lint run ./...

# Create a snapshot release (for testing)
release-snapshot:
	goreleaser release --snapshot --clean

# Dry run release (no actual release)
release-dry-run:
	goreleaser release --skip=publish --clean

# Install the binary to GOPATH/bin
install:
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(GOPATH)/bin/$(BINARY_NAME) $(MAIN_PATH)

# Show help
help:
	@echo "Ralph Makefile targets:"
	@echo ""
	@echo "  build            Build the binary with version info"
	@echo "  build-dev        Build for development (faster)"
	@echo "  test             Run all tests"
	@echo "  test-short       Run tests (skip slow tests)"
	@echo "  test-race        Run tests with race detection"
	@echo "  test-coverage    Run tests with coverage report"
	@echo "  test-integration Run integration tests (requires claude CLI)"
	@echo "  clean            Remove build artifacts"
	@echo "  deps             Download and tidy dependencies"
	@echo "  lint             Run golangci-lint"
	@echo "  release-snapshot Create a snapshot release"
	@echo "  release-dry-run  Dry run release"
	@echo "  install          Install to GOPATH/bin"
	@echo "  help             Show this help"
