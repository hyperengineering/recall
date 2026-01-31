#!/bin/bash
# post-start.sh - Runs every time the container starts

# Fix Go module cache permissions
# The /go/pkg/mod/cache directory is owned by root but Go tries to write there
if [ -d "/go/pkg/mod" ]; then
    sudo chown -R vscode:vscode /go/pkg/mod 2>/dev/null || true
fi

# Ensure workspace cache directories exist and are writable
mkdir -p /workspaces/recall/.gopath/pkg/mod/cache
mkdir -p /workspaces/recall/.gocache
