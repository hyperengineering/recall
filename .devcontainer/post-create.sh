#!/bin/bash
set -e

echo "=== Setting up Recall development environment ==="

# Download Go dependencies
go mod download

# Create data directory
mkdir -p /workspaces/recall/data

# Install development tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Build the CLI
go build -o /workspaces/recall/bin/recall ./cmd/recall

echo "=== Recall dev environment ready ==="
echo ""
echo "Available commands:"
echo "  make build    - Build the CLI"
echo "  make test     - Run tests"
echo "  make lint     - Run linter"
echo "  make install  - Install CLI to GOPATH"
