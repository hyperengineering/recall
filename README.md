# Recall

**Persistent memory for AI agents.** Recall captures, stores, and retrieves experiential knowledge (lore) across sessions—so your AI agents learn from the past instead of starting fresh every time.

## What It Does

```
Session 1: Agent discovers "ORM generates N+1 queries without eager loading"
           → Recall stores this insight

Session 2: Agent starts similar database work
           → Recall surfaces: "Watch out for N+1 queries..."
           → Agent avoids the same mistake
```

Recall enables AI agents to:
- **Remember** insights across sessions (architectural decisions, gotchas, patterns)
- **Recall** relevant knowledge when starting new work
- **Reinforce** what works through feedback loops
- **Sync** knowledge across environments via Engram (optional)

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
```

### Binary Download

Pre-built binaries available on the [Releases](https://github.com/hyperengineering/recall/releases) page.

### Go Library

```bash
go get github.com/hyperengineering/recall
```

## Quick Start

Recall works immediately with zero configuration—data stores in `./data/lore.db` by default.

### Record an insight

```bash
recall record --content "Queue consumers need idempotency checks" --category PATTERN_OUTCOME
```

### Search for relevant knowledge

```bash
recall query "implementing message consumers"
```

### Provide feedback on what helped

```bash
recall feedback --helpful L1 --not-relevant L2
```

That's it. Recall now remembers that insight for future sessions.

## Using with Claude Code (MCP)

The most common use case is integrating Recall with AI coding assistants via MCP (Model Context Protocol).

Add to `~/.claude/claude_desktop_config.json`:

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

This gives Claude Code four tools:
- `recall_query` — Search for relevant lore before starting work
- `recall_record` — Capture insights during implementation
- `recall_feedback` — Mark what helped (or didn't)
- `recall_sync` — Push to Engram for team sharing

See [MCP Integration Guide](docs/mcp-integration.md) for detailed configuration and usage patterns.

## How It Works

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Your Workflows                              │
│  ┌───────────┐    ┌───────────┐    ┌───────────┐    ┌───────────┐  │
│  │  Claude   │    │  Custom   │    │   CLI     │    │    Go     │  │
│  │   Code    │    │   Agent   │    │  Scripts  │    │  Library  │  │
│  └─────┬─────┘    └─────┬─────┘    └─────┬─────┘    └─────┬─────┘  │
│        │                │                │                │         │
│        └────────────────┴────────────────┴────────────────┘         │
│                                  │                                   │
│                           ┌──────▼──────┐                           │
│                           │   Recall    │  ← Local client           │
│                           │   Client    │    (this library)         │
│                           └──────┬──────┘                           │
│                                  │                                   │
│                           ┌──────▼──────┐                           │
│                           │   SQLite    │  ← Fast local storage     │
│                           │   (local)   │    (<20ms queries)        │
│                           └──────┬──────┘                           │
└──────────────────────────────────┼──────────────────────────────────┘
                                   │
                            (optional sync)
                                   │
                           ┌───────▼───────┐
                           │    Engram     │  ← Central service
                           │   (remote)    │    (team sharing)
                           └───────────────┘
```

**Recall** is the local client—it handles storage, search, and the agent-facing API.

**Engram** is the optional central service—it syncs lore across environments so your whole team benefits.

## Engram Sync (Optional)

Without Engram, Recall works entirely offline—perfect for personal use.

With Engram, lore syncs across all your environments:

```bash
# Configure Engram connection
export ENGRAM_URL="https://engram.example.com"
export ENGRAM_API_KEY="your-api-key"

# Push your insights to the team
recall sync push

# Pull latest from Engram (replaces local data)
recall sync bootstrap
```

## CLI Reference

### Global Options

| Option | Environment Variable | Default | Description |
|--------|---------------------|---------|-------------|
| `--lore-path` | `RECALL_DB_PATH` | `./data/lore.db` | Local database path |
| `--engram-url` | `ENGRAM_URL` | — | Engram service URL |
| `--api-key` | `ENGRAM_API_KEY` | — | Engram API key |
| `--source-id` | `RECALL_SOURCE_ID` | hostname | Client identifier |
| `--json` | — | — | Output as JSON |

### Commands

#### `recall record`

Capture new knowledge.

```bash
recall record --content "Event sourcing overkill for simple CRUD" \
  --category ARCHITECTURAL_DECISION \
  --context "story-2.1" \
  --confidence 0.8
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--content` | Yes | — | The insight (max 4000 chars) |
| `--category`, `-c` | Yes | — | Category (see below) |
| `--context` | No | — | Where this was learned (max 1000 chars) |
| `--confidence` | No | 0.5 | Initial confidence (0.0–1.0) |

#### `recall query`

Search for relevant lore.

```bash
recall query "database performance patterns" --top 10 --min-confidence 0.6
```

| Flag | Default | Description |
|------|---------|-------------|
| `--top`, `-k` | 5 | Max results |
| `--min-confidence` | 0.0 | Minimum confidence threshold |
| `--category` | — | Filter by categories (comma-separated) |

#### `recall feedback`

Improve lore quality through feedback.

```bash
# Single item
recall feedback --id L1 --type helpful

# Batch
recall feedback --helpful L1,L2 --incorrect L3 --not-relevant L4
```

Feedback effects:
- `helpful`: +0.08 confidence (caps at 1.0)
- `incorrect`: -0.15 confidence (floors at 0.0)
- `not_relevant`: no change (context mismatch, not quality issue)

#### `recall sync`

Synchronize with Engram.

```bash
recall sync push       # Send local changes to Engram
recall sync bootstrap  # Download full snapshot from Engram
recall sync --reinit   # Discard local data and re-bootstrap from Engram
```

| Flag | Description |
|------|-------------|
| `--reinit` | Discard local database and re-bootstrap from Engram. Requires confirmation unless `--force` is used. Aborts if unsynced local changes exist. |
| `--force` | Skip confirmation prompts (useful for scripts/automation) |

**Reinitialize workflow:**
1. Checks for unsynced local changes (aborts if any exist)
2. Prompts for confirmation (unless `--force`)
3. Downloads fresh snapshot from Engram
4. Replaces local database atomically

If Engram is unreachable, you can create an empty database with `--force`.

#### `recall session`

List lore surfaced in current session.

```bash
recall session
```

#### `recall stats`

Show store statistics.

```bash
recall stats
```

#### `recall version`

Print version info.

```bash
recall version
```

#### `recall mcp`

Run as MCP server (for AI agent integration).

```bash
recall mcp
```

## Lore Categories

| Category | Use For | Example |
|----------|---------|---------|
| `ARCHITECTURAL_DECISION` | System-level choices | "Chose event sourcing for audit trail" |
| `PATTERN_OUTCOME` | Pattern results | "Repository pattern unnecessary for simple CRUD" |
| `INTERFACE_LESSON` | API design insights | "Nullable returns caused null check proliferation" |
| `EDGE_CASE_DISCOVERY` | Implementation edge cases | "Empty collections need special handling" |
| `IMPLEMENTATION_FRICTION` | Design-to-code issues | "Interface was right but needed async" |
| `TESTING_STRATEGY` | Testing insights | "Queue consumers need idempotency tests" |
| `DEPENDENCY_BEHAVIOR` | Library gotchas | "ORM N+1 without eager loading config" |
| `PERFORMANCE_INSIGHT` | Performance findings | "In-memory failed at 10k; needed streaming" |

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
    // Create client
    client, err := recall.New(recall.Config{
        LocalPath: "./data/lore.db",
        // Optional: EngramURL and APIKey for sync
    })
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Record lore
    lore, err := client.Record(
        "Batch inserts with RETURNING avoid N queries for IDs",
        recall.CategoryPerformanceInsight,
        recall.WithContext("story-3.2"),
        recall.WithConfidence(0.7),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Recorded: %s\n", lore.ID)

    // Query for relevant lore
    ctx := context.Background()
    result, err := client.Query(ctx, recall.QueryParams{
        Query: "database insert performance",
        K:     5,
    })
    if err != nil {
        log.Fatal(err)
    }

    for _, l := range result.Lore {
        fmt.Printf("[%s] %s\n", l.Category, l.Content)
    }

    // Provide feedback
    if len(result.Lore) > 0 {
        client.Feedback("L1", recall.Helpful)
    }
}
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `RECALL_DB_PATH` | `./data/lore.db` | Local database path |
| `ENGRAM_URL` | — | Engram service URL (empty = offline mode) |
| `ENGRAM_API_KEY` | — | API key (required if ENGRAM_URL set) |
| `RECALL_SOURCE_ID` | hostname | Client identifier |
| `RECALL_DEBUG` | — | Enable debug logging (any non-empty value) |
| `RECALL_DEBUG_LOG` | stderr | Path to debug log file |

### Config Struct

```go
type Config struct {
    LocalPath    string        // Database path (default: ./data/lore.db)
    EngramURL    string        // Engram URL (empty = offline)
    APIKey       string        // Engram API key
    SourceID     string        // Client ID (default: hostname)
    SyncInterval time.Duration // Auto-sync interval (default: 5m)
    AutoSync     bool          // Background sync (default: true)
    Debug        bool          // Enable verbose API logging
    DebugLogPath string        // Debug log path (default: stderr)
}
```

### Debug Logging

Enable debug logging to see full Engram API communications:

```bash
export RECALL_DEBUG=1
export RECALL_DEBUG_LOG=/tmp/recall-debug.log
```

Debug logs include:
- Full HTTP request/response bodies
- Sync operation details
- Complete error messages from Engram API

## Confidence Model

Confidence scores (0.0–1.0) represent how validated lore is:

| Score | Meaning |
|-------|---------|
| 0.0–0.3 | Hypothesis, unvalidated |
| 0.3–0.6 | Some evidence |
| 0.6–0.8 | Validated in multiple contexts |
| 0.8–1.0 | Repeatedly confirmed |

Lore starts at 0.5 by default. Feedback from agents adjusts confidence over time.

## Security

- API keys are never logged or exposed in output
- Use environment variables for keys, not CLI flags
- Local SQLite database should be protected like any credential store

## Troubleshooting

### "mkdir ./data: permission denied"

Recall defaults to `./data/lore.db`. Set a different path:

```bash
export RECALL_DB_PATH="$HOME/.recall/lore.db"
```

### "sync unavailable: ENGRAM_URL not configured"

Sync requires Engram. Either configure it or use offline mode (local operations work fine without it).

### "invalid category"

Use one of the eight categories listed above (case-sensitive).

## Documentation

- [MCP Integration Guide](docs/mcp-integration.md) — Claude Code and AI agent setup
- [Engram API Specification](docs/engram-api-specification.md) — Central service API reference
- [Technical Design](docs/engram-recall.md) — Architecture and implementation details

## Development

```bash
make build       # Build CLI
make test        # Run tests
make lint        # Run linter
make ci          # All checks
```

## License

MIT License — see [LICENSE](LICENSE) for details.

## Author

**Lauri Jutila**
[ljuti@nmux.dev](mailto:ljuti@nmux.dev)

## Sponsorship

This project is sponsored by [NeuralMux](https://neuralmux.com) and is part of the [Hyper Engineering](https://hyperengineering.com) initiative to advance the union of human creativity and machine intelligence to build systems at extremes of scale, resilience, and performance.