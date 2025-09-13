# Variables
BINARY_NAME=dddns
VERSION=$(shell cat internal/version/VERSION 2>/dev/null || echo "dev")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X github.com/descoped/dddns/internal/version.Version=${VERSION} -X github.com/descoped/dddns/internal/version.BuildDate=${BUILD_DATE} -s -w"
GO_BUILD=CGO_ENABLED=0 go build ${LDFLAGS}

# Default target
.PHONY: all
all: clean test build

# Build for current platform
.PHONY: build
build:
	${GO_BUILD} -o bin/${BINARY_NAME} .

# Run the application
.PHONY: run
run:
	go run main.go

# Run tests
.PHONY: test
test:
	go test -v ./...

# Run integration tests
.PHONY: test-integration
test-integration:
	go test -v ./tests/...

# Clean build artifacts
.PHONY: clean
clean:
	rm -rf bin/ dist/
	go clean

# Build for specific platforms
.PHONY: build-linux
build-linux:
	@echo "Building for Linux platforms..."
	@mkdir -p dist
	GOOS=linux GOARCH=amd64 ${GO_BUILD} -o dist/${BINARY_NAME}-linux-amd64 .
	@echo "✓ Built for linux/amd64"
	GOOS=linux GOARCH=arm64 ${GO_BUILD} -o dist/${BINARY_NAME}-linux-arm64 .
	@echo "✓ Built for linux/arm64"
	GOOS=linux GOARCH=arm ${GO_BUILD} -o dist/${BINARY_NAME}-linux-arm .
	@echo "✓ Built for linux/arm (32-bit ARM)"

.PHONY: build-windows
build-windows:
	@echo "Building for Windows platforms..."
	@mkdir -p dist
	GOOS=windows GOARCH=amd64 ${GO_BUILD} -o dist/${BINARY_NAME}-windows-amd64.exe .
	@echo "✓ Built for windows/amd64"
	GOOS=windows GOARCH=arm64 ${GO_BUILD} -o dist/${BINARY_NAME}-windows-arm64.exe .
	@echo "✓ Built for windows/arm64"
	GOOS=windows GOARCH=386 ${GO_BUILD} -o dist/${BINARY_NAME}-windows-386.exe .
	@echo "✓ Built for windows/386 (32-bit)"

.PHONY: build-darwin
build-darwin:
	@echo "Building for macOS platforms..."
	@mkdir -p dist
	GOOS=darwin GOARCH=amd64 ${GO_BUILD} -o dist/${BINARY_NAME}-darwin-amd64 .
	@echo "✓ Built for darwin/amd64 (Intel Mac)"
	GOOS=darwin GOARCH=arm64 ${GO_BUILD} -o dist/${BINARY_NAME}-darwin-arm64 .
	@echo "✓ Built for darwin/arm64 (Apple Silicon)"

.PHONY: build-udm
build-udm:
	@echo "Building for UniFi Dream Machine..."
	@mkdir -p dist
	GOOS=linux GOARCH=arm64 ${GO_BUILD} -o dist/${BINARY_NAME}-linux-arm64 .
	@echo "✓ Built for linux/arm64 (UDM/UDR/UDM-Pro)"

# Build for all platforms
.PHONY: build-all
build-all: clean
	@echo "Building for all platforms..."
	@mkdir -p dist

	# Linux
	GOOS=linux GOARCH=amd64 ${GO_BUILD} -o dist/${BINARY_NAME}-linux-amd64 .
	@echo "✓ Built for linux/amd64"
	GOOS=linux GOARCH=arm64 ${GO_BUILD} -o dist/${BINARY_NAME}-linux-arm64 .
	@echo "✓ Built for linux/arm64 (including UDM/Raspberry Pi)"
	GOOS=linux GOARCH=arm ${GO_BUILD} -o dist/${BINARY_NAME}-linux-arm .
	@echo "✓ Built for linux/arm (32-bit ARM)"

	# Windows
	GOOS=windows GOARCH=amd64 ${GO_BUILD} -o dist/${BINARY_NAME}-windows-amd64.exe .
	@echo "✓ Built for windows/amd64"
	GOOS=windows GOARCH=arm64 ${GO_BUILD} -o dist/${BINARY_NAME}-windows-arm64.exe .
	@echo "✓ Built for windows/arm64"
	GOOS=windows GOARCH=386 ${GO_BUILD} -o dist/${BINARY_NAME}-windows-386.exe .
	@echo "✓ Built for windows/386 (32-bit)"

	# macOS
	GOOS=darwin GOARCH=amd64 ${GO_BUILD} -o dist/${BINARY_NAME}-darwin-amd64 .
	@echo "✓ Built for darwin/amd64 (Intel Mac)"
	GOOS=darwin GOARCH=arm64 ${GO_BUILD} -o dist/${BINARY_NAME}-darwin-arm64 .
	@echo "✓ Built for darwin/arm64 (Apple Silicon)"

	# FreeBSD (bonus)
	GOOS=freebsd GOARCH=amd64 ${GO_BUILD} -o dist/${BINARY_NAME}-freebsd-amd64 .
	@echo "✓ Built for freebsd/amd64"

	# Generate checksums
	@cd dist && sha256sum * > checksums.txt
	@echo "✓ Generated checksums"

	@echo ""
	@echo "Build complete! Binaries available in dist/"
	@ls -lh dist/

# Build for production release
.PHONY: release
release: clean test build-all
	@echo "Creating release archives..."
	@mkdir -p releases

	# Create tar.gz for Linux platforms
	@cd dist && tar -czf ../releases/${BINARY_NAME}-${VERSION}-linux-amd64.tar.gz ${BINARY_NAME}-linux-amd64
	@cd dist && tar -czf ../releases/${BINARY_NAME}-${VERSION}-linux-arm64.tar.gz ${BINARY_NAME}-linux-arm64
	@cd dist && tar -czf ../releases/${BINARY_NAME}-${VERSION}-linux-arm.tar.gz ${BINARY_NAME}-linux-arm

	# Create zip for Windows platforms
	@cd dist && zip -q ../releases/${BINARY_NAME}-${VERSION}-windows-amd64.zip ${BINARY_NAME}-windows-amd64.exe
	@cd dist && zip -q ../releases/${BINARY_NAME}-${VERSION}-windows-arm64.zip ${BINARY_NAME}-windows-arm64.exe
	@cd dist && zip -q ../releases/${BINARY_NAME}-${VERSION}-windows-386.zip ${BINARY_NAME}-windows-386.exe

	# Create tar.gz for macOS platforms
	@cd dist && tar -czf ../releases/${BINARY_NAME}-${VERSION}-darwin-amd64.tar.gz ${BINARY_NAME}-darwin-amd64
	@cd dist && tar -czf ../releases/${BINARY_NAME}-${VERSION}-darwin-arm64.tar.gz ${BINARY_NAME}-darwin-arm64

	# Create tar.gz for FreeBSD
	@cd dist && tar -czf ../releases/${BINARY_NAME}-${VERSION}-freebsd-amd64.tar.gz ${BINARY_NAME}-freebsd-amd64

	# Copy checksums
	@cp dist/checksums.txt releases/checksums-${VERSION}.txt

	@echo "✓ Release archives created in releases/"
	@ls -lh releases/

# Install locally (for development)
.PHONY: install
install: build
	@echo "Installing ${BINARY_NAME} to /usr/local/bin..."
	@sudo cp bin/${BINARY_NAME} /usr/local/bin/
	@echo "✓ Installed successfully"

# Uninstall from local system
.PHONY: uninstall
uninstall:
	@echo "Removing ${BINARY_NAME} from /usr/local/bin..."
	@sudo rm -f /usr/local/bin/${BINARY_NAME}
	@echo "✓ Uninstalled successfully"

# Update dependencies
.PHONY: deps-update
deps-update:
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy
	@echo "✓ Dependencies updated"

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...
	@echo "✓ Code formatted"

# Lint code
.PHONY: lint
lint:
	@echo "Linting code..."
	@if ! which golangci-lint > /dev/null; then \
		echo "Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	golangci-lint run
	@echo "✓ Linting complete"

# Development build with race detector
.PHONY: dev
dev:
	go build -race -o bin/${BINARY_NAME}-dev .
	@echo "✓ Development build complete (with race detector)"

# Help target
.PHONY: help
help:
	@echo "dddns Makefile targets:"
	@echo ""
	@echo "Building:"
	@echo "  make build         - Build for current platform"
	@echo "  make build-all     - Build for all supported platforms"
	@echo "  make build-linux   - Build for Linux (amd64, arm64, arm)"
	@echo "  make build-windows - Build for Windows (amd64, arm64, 386)"
	@echo "  make build-darwin  - Build for macOS (amd64, arm64)"
	@echo "  make build-udm     - Build for UniFi Dream Machine (linux/arm64)"
	@echo ""
	@echo "Development:"
	@echo "  make test          - Run tests"
	@echo "  make test-integration - Run integration tests only"
	@echo "  make dev           - Build with race detector"
	@echo "  make fmt           - Format code"
	@echo "  make lint          - Run linter"
	@echo "  make clean         - Clean build artifacts"
	@echo ""
	@echo "Installation:"
	@echo "  make install       - Install locally to /usr/local/bin"
	@echo "  make uninstall     - Remove from /usr/local/bin"
	@echo "  make deps-update   - Update Go dependencies"
	@echo ""
	@echo "Release:"
	@echo "  make release       - Build all platforms and create release archives"
	@echo ""
	@echo "Environment variables:"
	@echo "  VERSION  - Set version string (default: git tag or 'dev')"
	@echo ""
	@echo "Examples:"
	@echo "  make build-linux                  # Build for Linux platforms"
	@echo "  make build-windows                # Build for Windows platforms"
	@echo "  make build-udm                    # Build for UDM routers"
	@echo "  VERSION=v1.0.0 make release       # Create release with specific version"