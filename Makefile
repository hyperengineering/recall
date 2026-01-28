BIN_DIR ?= dist
CLI_BIN := $(BIN_DIR)/recall

.PHONY: build clean test lint install fmt vet ci

# Build CLI tool
build:
	@mkdir -p $(BIN_DIR)
	go build -o $(CLI_BIN) ./cmd/recall

# Install CLI to GOPATH
install:
	go install ./cmd/recall

# Run unit tests
test:
	go test -v ./...

# Run tests with coverage
test-cover:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run tests with race detector
test-race:
	go test -v -race ./...

# Run linter
lint:
	golangci-lint run ./...

# Format code
fmt:
	go fmt ./...

# Run go vet
vet:
	go vet ./...

# Clean build artifacts
clean:
	rm -rf $(BIN_DIR) coverage.out coverage.html data/*.db

# CI checks (format, vet, lint, test, build)
ci: fmt vet lint test build

# Download dependencies
deps:
	go mod download
	go mod tidy

# Generate mocks (placeholder for future use)
generate:
	go generate ./...

# Cross-compile for multiple platforms
release:
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 go build -o $(BIN_DIR)/recall-linux-amd64 ./cmd/recall
	GOOS=darwin GOARCH=amd64 go build -o $(BIN_DIR)/recall-darwin-amd64 ./cmd/recall
	GOOS=darwin GOARCH=arm64 go build -o $(BIN_DIR)/recall-darwin-arm64 ./cmd/recall
	GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/recall-windows-amd64.exe ./cmd/recall
