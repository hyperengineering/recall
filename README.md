# Recall

Recall is a Go library and CLI for managing experiential lore — discrete units of knowledge captured from AI agent workflows.

## Overview

Recall enables AI agents to:
- **Capture** structured insights during workflows
- **Store** lore locally for low-latency recall
- **Query** semantically similar lore based on current context
- **Synchronize** lore with Engram (central service) for cross-environment persistence
- **Reinforce** lore quality through feedback loops

## Installation

### Homebrew (macOS/Linux)

```bash
brew install hyperengineering/tap/recall
```

### Go Install

```bash
go install github.com/hyperengineering/recall/cmd/recall@latest
```

### Docker

```bash
docker pull ghcr.io/hyperengineering/recall:latest

# Run with local database mounted
docker run -v ./data:/data -e RECALL_DB_PATH=/data/lore.db ghcr.io/hyperengineering/recall:latest query "your search"
```

### Download Binary

Pre-built binaries for Linux, macOS, and Windows are available on the [Releases](https://github.com/hyperengineering/recall/releases) page.

### Library

```bash
go get github.com/hyperengineering/recall
```

## Quick Start

### 1. Configure

Set the required environment variable:

```bash
export RECALL_DB_PATH="./data/lore.db"
```

For Engram sync (optional):

```bash
export ENGRAM_URL="https://engram.example.com"
export ENGRAM_API_KEY="your-api-key"
export RECALL_SOURCE_ID="my-workstation"
```

### 2. Record Lore

```bash
# Record a new insight
recall record --content "Queue consumers benefit from idempotency checks" --category PATTERN_OUTCOME

# With optional context and confidence
recall record --content "ORM generates N+1 queries without eager loading" \
  --category DEPENDENCY_BEHAVIOR \
  --context "story-2.1-performance" \
  --confidence 0.7

# Output as JSON
recall record --content "Always validate input at API boundaries" \
  --category INTERFACE_LESSON --json
```

### 3. Query Lore

```bash
# Search for relevant lore
recall query "implementing message consumers"

# With filters
recall query "database performance" --top 10 --min-confidence 0.6

# Filter by category
recall query "testing" --category TESTING_STRATEGY,PATTERN_OUTCOME

# Output as JSON
recall query "error handling" --json
```

### 4. Provide Feedback

After querying, lore entries are assigned session references (L1, L2, ...).

```bash
# Single-item feedback
recall feedback --id L1 --type helpful
recall feedback --id L2 --type incorrect
recall feedback --id L3 --type not_relevant

# Batch feedback
recall feedback --helpful L1,L2 --incorrect L3

# By lore ID directly
recall feedback --id 01HXYZ123ABC --type helpful
```

### 5. Sync with Engram

```bash
# Push local changes to Engram
recall sync push

# Download full snapshot from Engram (replaces local data)
recall sync bootstrap
```

## CLI Reference

### Global Flags

| Flag | Environment Variable | Description |
|------|---------------------|-------------|
| `--lore-path` | `RECALL_DB_PATH` | Path to local SQLite database (required) |
| `--engram-url` | `ENGRAM_URL` | URL of Engram central service |
| `--api-key` | `ENGRAM_API_KEY` | API key for Engram authentication |
| `--source-id` | `RECALL_SOURCE_ID` | Client source identifier |
| `--json` | - | Output as JSON instead of human-readable |

### recall record

Record a new lore entry.

```
recall record --content <text> --category <category> [flags]
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--content` | - | (required) | Lore content (max 4000 chars) |
| `--category` | `-c` | (required) | Lore category |
| `--context` | - | - | Additional context (max 1000 chars) |
| `--confidence` | - | 0.5 | Initial confidence (0.0-1.0) |

**Examples:**

```bash
recall record --content "Pattern X works well for Y" -c PATTERN_OUTCOME
recall record --content "Edge case Z" -c EDGE_CASE_DISCOVERY --context "story-1.2" --confidence 0.8
```

### recall query

Search for relevant lore.

```
recall query <search terms> [flags]
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--top` | `-k` | 5 | Maximum number of results |
| `--min-confidence` | - | 0.0 | Minimum confidence threshold |
| `--category` | - | - | Comma-separated categories to filter |

**Examples:**

```bash
recall query "error handling patterns"
recall query "performance" --top 10 --min-confidence 0.7
recall query "testing" --category TESTING_STRATEGY,PATTERN_OUTCOME --json
```

### recall feedback

Provide feedback on recalled lore.

**Single-item mode:**

```
recall feedback --id <ref> --type <type>
```

| Flag | Description |
|------|-------------|
| `--id` | Lore ID or session reference (L1, L2, ...) |
| `--type` | Feedback type: `helpful`, `incorrect`, `not_relevant` |

**Batch mode:**

```
recall feedback [--helpful <refs>] [--incorrect <refs>] [--not-relevant <refs>]
```

| Flag | Description |
|------|-------------|
| `--helpful` | Comma-separated helpful refs (+0.08 confidence) |
| `--incorrect` | Comma-separated incorrect refs (-0.15 confidence) |
| `--not-relevant` | Comma-separated not-relevant refs (no change) |

**Examples:**

```bash
recall feedback --id L1 --type helpful
recall feedback --helpful L1,L2 --incorrect L3
recall feedback --id 01HXYZ123ABC --type incorrect --json
```

### recall sync push

Push pending local changes to Engram.

```
recall sync push [flags]
```

Requires `ENGRAM_URL` and `ENGRAM_API_KEY` to be configured.

**Example:**

```bash
recall sync push
recall sync push --json
```

### recall sync bootstrap

Download a full snapshot from Engram, replacing local data.

```
recall sync bootstrap [flags]
```

**Warning:** This replaces ALL local lore with the server snapshot.

**Example:**

```bash
recall sync bootstrap
recall sync bootstrap --json
```

### recall session

List lore surfaced during the current session.

```
recall session [flags]
```

**Note:** In CLI mode, each command invocation is a separate session. This command is more useful in scripted or interactive scenarios.

**Example:**

```bash
recall session
recall session --json
```

### recall version

Print version information.

```
recall version [flags]
```

**Example:**

```bash
recall version
recall version --json
```

```json
{
  "version": "1.0.0",
  "commit": "abc1234",
  "date": "2026-01-30T12:00:00Z",
  "go": "go1.23.12",
  "os": "darwin",
  "arch": "arm64"
}
```

### recall stats

Display store statistics.

```
recall stats [flags]
```

**Example:**

```bash
recall stats
recall stats --json
```

## Configuration

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `RECALL_DB_PATH` | Yes | Path to local SQLite database |
| `ENGRAM_URL` | No | URL of Engram central service |
| `ENGRAM_API_KEY` | No* | API key for Engram (*required if ENGRAM_URL is set) |
| `RECALL_SOURCE_ID` | No | Client identifier (defaults to hostname) |

### Config Struct (Library)

```go
type Config struct {
    LocalPath    string        // Path to local SQLite database (required)
    EngramURL    string        // Central service URL (optional, empty = offline mode)
    APIKey       string        // Engram API key (required if EngramURL is set)
    SourceID     string        // Client identifier (defaults to hostname)
    SyncInterval time.Duration // Auto-sync interval (default: 5m)
    AutoSync     bool          // Enable background sync (default: true)
}
```

You can also load configuration from environment variables:

```go
cfg := recall.ConfigFromEnv() // Reads from RECALL_DB_PATH, ENGRAM_URL, etc.
```

### Offline Mode

When `ENGRAM_URL` is not set, Recall operates in offline-only mode:
- All local operations work normally (record, query, feedback)
- Sync commands return an error
- No network calls are made

## Lore Categories

| Category | Description |
|----------|-------------|
| `ARCHITECTURAL_DECISION` | System-level choices and rationale |
| `PATTERN_OUTCOME` | Results of applying a design pattern |
| `INTERFACE_LESSON` | Contract/API design insights |
| `EDGE_CASE_DISCOVERY` | Scenarios found during implementation |
| `IMPLEMENTATION_FRICTION` | Design-to-code translation difficulties |
| `TESTING_STRATEGY` | Testing approach insights |
| `DEPENDENCY_BEHAVIOR` | Library/framework gotchas |
| `PERFORMANCE_INSIGHT` | Performance characteristics |

## Confidence Model

Confidence represents how validated a lore entry is (0.0 - 1.0):

| Score | Meaning |
|-------|---------|
| 0.0 - 0.3 | Hypothesis, unvalidated |
| 0.3 - 0.6 | Some evidence, limited validation |
| 0.6 - 0.8 | Validated in multiple contexts |
| 0.8 - 1.0 | Repeatedly confirmed, high reliability |

Feedback adjustments:
- `helpful`: +0.08 (capped at 1.0)
- `incorrect`: -0.15 (floored at 0.0)
- `not_relevant`: no change

## Library Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/hyperengineering/recall"
)

func main() {
    // Create client - offline mode when EngramURL is not set
    client, err := recall.New(recall.Config{
        LocalPath: "./data/lore.db",
        // Optional: EngramURL and APIKey for sync with Engram
        // EngramURL: "https://engram.example.com",
        // APIKey:    "your-api-key",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Record lore using functional options
    lore, err := client.Record(
        "ORM generates N+1 queries without eager loading",
        recall.CategoryDependencyBehavior,
        recall.WithContext("performance-investigation"),
        recall.WithConfidence(0.7),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Recorded: %s\n", lore.ID)

    // Query for relevant lore
    ctx := context.Background()
    minConf := 0.5
    result, err := client.Query(ctx, recall.QueryParams{
        Query:         "database performance",
        K:             5,
        MinConfidence: &minConf,
    })
    if err != nil {
        log.Fatal(err)
    }

    for _, l := range result.Lore {
        fmt.Printf("[%s] %s (confidence: %.2f)\n", l.Category, l.Content, l.Confidence)
    }

    // Provide feedback using session reference (L1, L2, etc.)
    if len(result.Lore) > 0 {
        updated, err := client.Feedback("L1", recall.Helpful)
        if err != nil {
            log.Fatal(err)
        }
        fmt.Printf("Updated confidence: %.2f\n", updated.Confidence)
    }

    // Sync with Engram (only if configured)
    if err := client.Sync(ctx); err != nil {
        // Returns ErrOffline if EngramURL not configured
        fmt.Printf("Sync: %v\n", err)
    }
}
```

## JSON Output Mode

All CLI commands support `--json` for structured output:

```bash
# Record with JSON output
recall record --content "Test" -c PATTERN_OUTCOME --json
```

```json
{
  "id": "01HXYZ123ABC456DEF789GHI",
  "content": "Test",
  "category": "PATTERN_OUTCOME",
  "confidence": 0.5,
  "validation_count": 0,
  "source_id": "my-workstation",
  "created_at": "2026-01-29T10:30:00Z",
  "updated_at": "2026-01-29T10:30:00Z"
}
```

All JSON output uses `snake_case` field names.

## Security

### API Key Protection

Recall follows security best practices for API key handling:

- **Never logged**: API keys are never written to logs or debug output
- **Never in errors**: Error messages never include API key values
- **Never in CLI output**: The `--json` flag and human-readable output never expose keys
- **Environment variables**: Configure keys via `ENGRAM_API_KEY` to avoid hardcoding

**Recommended practices:**

```bash
# Good: Use environment variables
export ENGRAM_API_KEY="sk-..."
recall sync push

# Avoid: Passing keys as flags (visible in shell history)
recall --api-key "sk-..." sync push  # Not recommended
```

## Troubleshooting

### Missing Configuration

**Error:** `configuration: LocalPath: required: path to SQLite database — set RECALL_DB_PATH or use --lore-path`

**Solution:** Set the required environment variable:

```bash
export RECALL_DB_PATH="./data/lore.db"
```

### Offline Mode Errors

**Error:** `sync unavailable: ENGRAM_URL not configured (offline-only mode)`

**Solution:** Configure Engram URL and API key for sync operations:

```bash
export ENGRAM_URL="https://engram.example.com"
export ENGRAM_API_KEY="your-api-key"
```

Or accept offline-only mode — local operations (record, query, feedback) work without Engram.

### Invalid Input Errors

**Error:** `record lore: validation: Category: invalid: must be one of ARCHITECTURAL_DECISION, PATTERN_OUTCOME, ...`

**Solution:** Use a valid category from the list above.

**Error:** `record lore: validation: Content: exceeds 4000 character limit`

**Solution:** Shorten the content to 4000 characters or less.

**Error:** `invalid feedback type "wrong": valid types are helpful, incorrect, not_relevant`

**Solution:** Use one of the valid feedback types.

### Sync Failures

**Error:** `push: sync: push_lore failed (status 401): Unauthorized`

**Solution:** Check that `ENGRAM_API_KEY` is set correctly.

**Error:** `bootstrap: sync: download_snapshot failed (status 503): Service Unavailable`

**Solution:** The Engram service may be temporarily unavailable. Retry later.

## MCP Integration

Recall provides an MCP (Model Context Protocol) server for integration with AI coding agents like Claude Code.

**Available tools:**
- `recall_query` - Retrieve relevant lore with session references (L1, L2, ...)
- `recall_record` - Capture new lore with content, category, and optional context
- `recall_feedback` - Provide feedback using session references
- `recall_sync` - Push pending changes to Engram

### MCP Server Quick Start

Add Recall to your Claude Code configuration (`~/.claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "recall": {
      "command": "recall",
      "args": ["mcp"],
      "env": {
        "RECALL_DB_PATH": "/path/to/your/lore.db",
        "RECALL_SOURCE_ID": "claude-code"
      }
    }
  }
}
```

The MCP server maintains session state, so L1, L2, etc. references persist throughout your coding session.

### CLI-based Workflow (Alternative)

```bash
# Set environment variables
export RECALL_DB_PATH="$HOME/.recall/lore.db"
export RECALL_SOURCE_ID="claude-code"

# Query for relevant context before starting work
recall query "implementing message queue consumer" --json

# Record insights during implementation
recall record --content "Queue consumers benefit from idempotency checks" \
  --category PATTERN_OUTCOME --context "story-3.1" --json

# Provide feedback on recalled lore
recall feedback --helpful L1,L3 --not-relevant L2
```

See [docs/mcp-integration.md](docs/mcp-integration.md) for comprehensive configuration and usage patterns.

## Development

```bash
# Download dependencies
make deps

# Run tests
make test

# Run tests with coverage report
make test-cover

# Run tests with race detector
make test-race

# Run linter
make lint

# Format code
make fmt

# Run go vet
make vet

# Build CLI
make build

# Install to GOPATH
make install

# Run all CI checks (fmt, vet, lint, test, build)
make ci

# Cross-compile for all platforms
make release

# Generate code (mocks, etc.)
make generate

# Clean build artifacts
make clean
```

## Architecture

Recall is part of the Engram system:

- **Engram** (central service): Where lore is stored and synchronized
- **Recall** (this library): How agents retrieve and contribute lore
- **Lore**: Individual learnings — the substance itself

See [docs/engram-recall.md](docs/engram-recall.md) for the full technical design.

## License

MIT
