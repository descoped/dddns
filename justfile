# dddns recipes — `just --list` for the index.
#
# Cross-platform binaries are GoReleaser's job (it runs on every tag push in CI
# via .github/workflows/goreleaser.yml). Local recipes here cover the inner
# dev loop — build, test, lint, install — plus the Lambda-specific build
# that GoReleaser doesn't ship.

binary     := "dddns"
version    := `git describe --tags --always --dirty 2>/dev/null || echo "dev"`
build_date := `date -u +"%Y-%m-%dT%H:%M:%SZ"`
ldflags    := "-s -w -X github.com/descoped/dddns/internal/version.Version=" + version + " -X github.com/descoped/dddns/internal/version.BuildDate=" + build_date

# Default: show the list.
default:
    @just --list

# Build for current platform into bin/.
build:
    CGO_ENABLED=0 go build -ldflags '{{ldflags}}' -o bin/{{binary}} .
    @echo "✓ bin/{{binary}} ({{version}})"

# Race-detector build for local dev.
dev:
    go build -race -o bin/{{binary}}-dev .
    @echo "✓ bin/{{binary}}-dev (race)"

# Run the binary from source (pass subcommands / flags as *args).
run *args:
    go run main.go {{args}}

# Full test suite with the race detector.
test:
    go test -race ./...

# Integration tests only (when present under tests/).
test-integration:
    go test -race -v ./tests/...

# Format, vet, lint.
fmt:
    go fmt ./...

vet:
    go vet ./...

lint:
    @command -v golangci-lint >/dev/null || { echo "golangci-lint missing — install from https://golangci-lint.run"; exit 1; }
    golangci-lint run

# Quick gate — run before committing.
check: fmt vet test lint

# Dependency hygiene.
deps-update:
    go get -u ./...
    go mod tidy

# Remove build artefacts.
clean:
    rm -rf bin/ dist/ deploy/aws-lambda/dist/
    go clean

# Install the current-platform binary into /usr/local/bin (requires sudo).
install: build
    sudo cp bin/{{binary}} /usr/local/bin/{{binary}}
    @echo "✓ installed /usr/local/bin/{{binary}}"

uninstall:
    sudo rm -f /usr/local/bin/{{binary}}
    @echo "✓ removed /usr/local/bin/{{binary}}"

# Build deploy/aws-lambda/dist/lambda.zip (Linux arm64 static, provided.al2023).
build-aws-lambda:
    @mkdir -p deploy/aws-lambda/dist
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
        -ldflags '{{ldflags}}' -tags lambda.norpc \
        -o deploy/aws-lambda/dist/bootstrap ./deploy/aws-lambda
    cd deploy/aws-lambda/dist && zip -j lambda.zip bootstrap
    @echo "✓ deploy/aws-lambda/dist/lambda.zip"
