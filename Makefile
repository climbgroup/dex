.PHONY: build build-all release release-archives checksums test test-cover fmt vet lint clean install install-user help

BINARY_NAME := dex
GOPATH := $(shell go env GOPATH)
BIN_DIR := bin
DIST_DIR := dist

VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w \
  -X github.com/climbgroup/dex/internal/cli.Version=$(VERSION) \
  -X github.com/climbgroup/dex/internal/cli.Commit=$(COMMIT) \
  -X github.com/climbgroup/dex/internal/cli.Date=$(DATE)

# Cross-compile target matrix (os/arch).
PLATFORMS := \
  linux/amd64 \
  linux/arm64 \
  darwin/amd64 \
  darwin/arm64 \
  windows/amd64 \
  windows/arm64

# Default target
all: build

## build: Build the CLI binary to bin/dex
build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/dex

## build-all: Cross-compile binaries for all supported platforms into dist/
build-all:
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
	  os=$${platform%/*}; arch=$${platform#*/}; \
	  ext=""; if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
	  out="$(DIST_DIR)/$(BINARY_NAME)_$${os}_$${arch}$${ext}"; \
	  echo "  -> $$out"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
	    go build -trimpath -ldflags "$(LDFLAGS)" -o "$$out" ./cmd/dex || exit 1; \
	done

## release-archives: Package per-platform archives (.tar.gz / .zip) into dist/
release-archives: build-all
	@cd $(DIST_DIR) && for platform in $(PLATFORMS); do \
	  os=$${platform%/*}; arch=$${platform#*/}; \
	  base="$(BINARY_NAME)_$(VERSION)_$${os}_$${arch}"; \
	  if [ "$$os" = "windows" ]; then \
	    bin="$(BINARY_NAME)_$${os}_$${arch}.exe"; \
	    cp "$$bin" "$(BINARY_NAME).exe"; \
	    zip -q "$$base.zip" "$(BINARY_NAME).exe"; \
	    rm "$(BINARY_NAME).exe"; \
	    echo "  -> $(DIST_DIR)/$$base.zip"; \
	  else \
	    bin="$(BINARY_NAME)_$${os}_$${arch}"; \
	    cp "$$bin" "$(BINARY_NAME)"; \
	    tar -czf "$$base.tar.gz" "$(BINARY_NAME)"; \
	    rm "$(BINARY_NAME)"; \
	    echo "  -> $(DIST_DIR)/$$base.tar.gz"; \
	  fi; \
	done

## checksums: Generate SHA256SUMS for archives in dist/
checksums:
	@cd $(DIST_DIR) && shasum -a 256 *.tar.gz *.zip > SHA256SUMS 2>/dev/null || true
	@echo "Wrote $(DIST_DIR)/SHA256SUMS"

## release: Cross-compile, archive, and checksum everything in dist/
release: release-archives checksums

## test: Run all tests
test:
	go test ./...

## test-cover: Run tests with coverage report
test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report written to coverage.html"

## fmt: Format code with go fmt
fmt:
	go fmt ./...

## vet: Run go vet
vet:
	go vet ./...

## lint: Run fmt + vet
lint: fmt vet

## clean: Remove built binary, dist artifacts, and coverage files
clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)
	rm -f coverage.out coverage.html

## install: Install binary to ~/.bin
install: build
	@mkdir -p $(HOME)/.local/bin
	cp $(BIN_DIR)/$(BINARY_NAME) $(HOME)/.bin/
	@echo "Installed to $(HOME)/.local/bin/$(BINARY_NAME)"
	@echo "Make sure $(HOME)/.local/bin is in your PATH"

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed 's/^/  /'
