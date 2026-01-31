#!/bin/bash
set -e

echo "=== Setting up Recall development environment ==="

# Create Go directories in workspace (avoids /go permission issues)
mkdir -p /workspaces/recall/.gopath
mkdir -p /workspaces/recall/.gocache
export GOPATH=/workspaces/recall/.gopath
export GOCACHE=/workspaces/recall/.gocache

# Install GitHub CLI
if ! command -v gh &> /dev/null; then
    echo "Installing GitHub CLI..."
    (type -p wget >/dev/null || (sudo apt update && sudo apt-get install wget -y)) \
        && sudo mkdir -p -m 755 /etc/apt/keyrings \
        && out=$(mktemp) && wget -nv -O$out https://cli.github.com/packages/githubcli-archive-keyring.gpg \
        && cat $out | sudo tee /etc/apt/keyrings/githubcli-archive-keyring.gpg > /dev/null \
        && sudo chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg \
        && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
        && sudo apt update \
        && sudo apt install gh -y
fi

# Download Go dependencies
go mod download

# Create data directory
mkdir -p /workspaces/recall/data

# Install development tools
echo "Installing development tools..."
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
curl -fsSL https://claude.ai/install.sh | bash

# Build the CLI
go build -o /workspaces/recall/bin/recall ./cmd/recall

echo "=== Recall dev environment ready ==="
echo ""
echo "Available commands:"
echo "  make build    - Build the CLI"
echo "  make test     - Run tests"
echo "  make lint     - Run linter"
echo "  make install  - Install CLI to GOPATH"
