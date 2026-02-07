# Remnawave Node Go - Makefile

# Load environment variables from .env file if it exists
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

# Variables
BINARY_NAME=remnawave-node
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Directories
CMD_DIR=./cmd/node
BUILD_DIR=./build
DIST_DIR=./dist

# Platforms
PLATFORMS=linux/amd64 linux/arm64 linux/arm darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all build clean test deps lint run help release

# Default target
all: clean deps build

# Build for current platform
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for all platforms
release:
	@echo "Building releases..."
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} ; \
		output_name=$(BINARY_NAME)_$${GOOS}_$${GOARCH} ; \
		if [ $${GOOS} = "windows" ]; then \
			output_name=$${output_name}.exe ; \
		fi ; \
		echo "Building $${output_name}..." ; \
		GOOS=$${GOOS} GOARCH=$${GOARCH} $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$${output_name} $(CMD_DIR) ; \
	done
	@echo "Release binaries built in $(DIST_DIR)"

# Create release archives
package: release
	@echo "Creating release archives..."
	@cd $(DIST_DIR) && \
	for f in $(BINARY_NAME)_*; do \
		if [ "$${f##*.}" = "exe" ]; then \
			base=$${f%.exe} ; \
			zip $${base}.zip $$f ; \
		else \
			tar -czvf $${f}.tar.gz $$f ; \
		fi ; \
	done
	@echo "Archives created in $(DIST_DIR)"

# Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	$(BUILD_DIR)/$(BINARY_NAME)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -rf $(DIST_DIR)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -cover ./...

# Run tests with coverage
coverage:
	@echo "Running tests with coverage..."
	@mkdir -p $(BUILD_DIR)
	$(GOTEST) -coverprofile=$(BUILD_DIR)/coverage.out ./...
	$(GOCMD) tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "Coverage report: $(BUILD_DIR)/coverage.html"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Lint code
lint:
	@echo "Linting code..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

# Verify dependencies
verify:
	@echo "Verifying dependencies..."
	$(GOMOD) verify

# Update dependencies
update:
	@echo "Updating dependencies..."
	$(GOMOD) tidy
	$(GOGET) -u ./...

# Update Xray-core to latest version
update-xray:
	@echo "Updating Xray-core to latest version..."
	@LATEST=$$(curl -fsSL "https://api.github.com/repos/XTLS/Xray-core/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'); \
	if [ -z "$$LATEST" ]; then \
		echo "Failed to get latest Xray-core version"; \
		exit 1; \
	fi; \
	echo "Latest Xray-core version: $$LATEST"; \
	$(GOGET) github.com/xtls/xray-core@$$LATEST; \
	$(GOMOD) tidy; \
	echo "Xray-core updated to $$LATEST"

# Update Xray-core to specific version
update-xray-version:
	@if [ -z "$(XRAY_VERSION)" ]; then \
		echo "Usage: make update-xray-version XRAY_VERSION=v1.x.x"; \
		exit 1; \
	fi
	@echo "Updating Xray-core to $(XRAY_VERSION)..."
	$(GOGET) github.com/xtls/xray-core@$(XRAY_VERSION)
	$(GOMOD) tidy
	@echo "Xray-core updated to $(XRAY_VERSION)"

# Show current Xray-core version
xray-version:
	@echo "Current Xray-core version in go.mod:"
	@grep "github.com/xtls/xray-core" go.mod | head -1
	@echo ""
	@echo "Fetching latest Xray-core version..."
	@LATEST=$$(curl -fsSL "https://api.github.com/repos/XTLS/Xray-core/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'); \
	echo "Latest available: $$LATEST"

# Install to system (Linux only)
install: build
	@echo "Installing $(BINARY_NAME)..."
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	@sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	@echo "Installed to /usr/local/bin/$(BINARY_NAME)"

# Uninstall from system
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	@sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "Uninstalled"

# Docker build
docker-build:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(VERSION) .

# Docker run
docker-run: docker-build
	docker run --rm -it \
		-e SECRET_KEY=$${SECRET_KEY} \
		-e NODE_PORT=3000 \
		-p 3000:3000 \
		$(BINARY_NAME):$(VERSION)

# Show help
help:
	@echo "Remnawave Node Go - Build Commands"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Build Targets:"
	@echo "  build       - Build binary for current platform"
	@echo "  release     - Build binaries for all platforms"
	@echo "  package     - Create release archives"
	@echo "  run         - Build and run the application"
	@echo "  clean       - Clean build artifacts"
	@echo ""
	@echo "Test Targets:"
	@echo "  test        - Run tests"
	@echo "  coverage    - Run tests with coverage report"
	@echo "  lint        - Run linter"
	@echo "  fmt         - Format code"
	@echo ""
	@echo "Dependency Targets:"
	@echo "  deps        - Download dependencies"
	@echo "  update      - Update all dependencies"
	@echo "  update-xray - Update Xray-core to latest version"
	@echo "  update-xray-version XRAY_VERSION=vX.X.X - Update Xray-core to specific version"
	@echo "  xray-version - Show current and latest Xray-core version"
	@echo ""
	@echo "Install Targets:"
	@echo "  install     - Install to /usr/local/bin (requires sudo)"
	@echo "  uninstall   - Remove from /usr/local/bin"
	@echo ""
	@echo "Docker Targets:"
	@echo "  docker-build - Build Docker image"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION      - Set version string (default: git tag or 'dev')"
	@echo "  XRAY_VERSION - Xray-core version for update-xray-version target"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make VERSION=v1.0.0 release"
	@echo "  make update-xray"
	@echo "  make update-xray-version XRAY_VERSION=v1.8.24"
	@echo "  make xray-version"
