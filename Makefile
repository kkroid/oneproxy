# Makefile for OneProxy

# Variables
BINARY_NAME=oneproxy
BINARY_WINDOWS=$(BINARY_NAME).exe
CMD_PATH=./cmd/oneproxy
BUILD_DIR=build
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

.PHONY: all build clean test run deps help download-singbox

# Default target
all: deps build

# Build the project
build:
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_WINDOWS) $(CMD_PATH)
	@echo "Build complete: $(BINARY_WINDOWS)"

# Build for Windows
build-windows:
	@echo "Building for Windows..."
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_WINDOWS) $(CMD_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_WINDOWS)"

# Build for Linux
build-linux:
	@echo "Building for Linux..."
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux $(CMD_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux"

# Build for macOS
build-darwin:
	@echo "Building for macOS..."
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin $(CMD_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-darwin"

# Build for all platforms
build-all: build-windows build-linux build-darwin
	@echo "All builds complete!"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Dependencies installed."

# Clean build files
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -f $(BINARY_WINDOWS)
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)
	rm -f singbox_generated.json
	@echo "Clean complete."

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_WINDOWS)

# Download sing-box binary
download-singbox:
	@echo "Downloading sing-box..."
ifeq ($(OS),Windows_NT)
	@powershell -ExecutionPolicy Bypass -File download-singbox.bat
else
	@bash download-singbox.sh
endif

# Initialize project (for first-time setup)
init: deps download-singbox
	@echo "Initializing project..."
	@if [ ! -f config.json ]; then \
		cp configs/config.example.json config.json; \
		echo "Created config.json from example. Please update with your settings."; \
	fi
	@mkdir -p logs bin
	@echo "Project initialized!"

# Show help
help:
	@echo "OneProxy Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  make build            - Build for current platform"
	@echo "  make build-windows    - Build for Windows"
	@echo "  make build-linux      - Build for Linux"
	@echo "  make build-darwin     - Build for macOS"
	@echo "  make build-all        - Build for all platforms"
	@echo "  make deps             - Install Go dependencies"
	@echo "  make clean            - Clean build artifacts"
	@echo "  make test             - Run tests"
	@echo "  make run              - Build and run"
	@echo "  make download-singbox - Download sing-box binary"
	@echo "  make init             - Initialize project (first-time setup)"
	@echo "  make help             - Show this help"
	@echo ""
	@echo "Quick start:"
	@echo "  make init    # First time only"
	@echo "  make build   # Build the application"
	@echo "  make run     # Run the application"
