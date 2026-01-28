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

### CLI

```bash
go install github.com/hyperengineering/recall/cmd/recall@latest
```

### Library

```bash
go get github.com/hyperengineering/recall
```

## Quick Start

### CLI Usage

```bash
# Record new lore
recall record "Queue consumers benefit from idempotency checks" -c PATTERN_OUTCOME

# Query for relevant lore
recall query "implementing message consumers"

# Provide feedback on recalled lore
recall feedback --helpful L1,L2

# Sync with Engram
recall sync
```

### Library Usage

```go
package main

import (
    "context"
    "github.com/hyperengineering/recall"
)

func main() {
    // Create client
    client, err := recall.New(recall.Config{
        LorePath:  "./data/lore.db",
        EngramURL: "https://engram.example.com",
        APIKey:    "your-api-key",
    })
    if err != nil {
        panic(err)
    }
    defer client.Close()

    ctx := context.Background()

    // Record lore
    lore, _ := client.Record(ctx, recall.RecordParams{
        Content:  "ORM generates N+1 queries without eager loading",
        Category: recall.CategoryDependencyBehavior,
        Context:  "performance-investigation",
    })

    // Query for relevant lore
    result, _ := client.Query(ctx, recall.QueryParams{
        Query:         "database performance",
        K:             5,
        MinConfidence: 0.5,
    })

    // Provide feedback
    client.Feedback(ctx, recall.FeedbackParams{
        Helpful: []string{"L1", "L2"},
    })
}
```

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
- `helpful`: +0.08
- `incorrect`: -0.15
- `not_relevant`: no change

## Configuration

### Environment Variables

| Variable | Description |
|----------|-------------|
| `RECALL_LORE_PATH` | Path to local SQLite database |
| `ENGRAM_URL` | URL of Engram central service |
| `ENGRAM_API_KEY` | API key for Engram authentication |

### Config Struct

```go
type Config struct {
    LorePath     string        // Local database path
    EngramURL    string        // Central service URL (optional)
    APIKey       string        // Engram API key
    SourceID     string        // Client identifier
    SyncInterval time.Duration // Auto-sync interval (default: 5m)
    AutoSync     bool          // Enable background sync (default: true)
    OfflineMode  bool          // Disable network operations
}
```

## MCP Integration

Recall provides optional MCP (Model Context Protocol) adapters:

```go
import (
    "github.com/hyperengineering/recall"
    recallmcp "github.com/hyperengineering/recall/mcp"
)

client, _ := recall.New(cfg)
recallmcp.RegisterTools(mcpRegistry, client)
```

Available tools:
- `recall_query` - Retrieve relevant lore
- `recall_record` - Capture new lore
- `recall_feedback` - Provide feedback on recalled lore

## Development

```bash
# Run tests
make test

# Run linter
make lint

# Build CLI
make build

# Install to GOPATH
make install
```

## Architecture

Recall is part of the Engram system:

- **Engram** (central service): Where lore is stored and synchronized
- **Recall** (this library): How agents retrieve and contribute lore
- **Lore**: Individual learnings — the substance itself

See [docs/engram.md](docs/engram.md) for the full technical design.

## License

MIT
