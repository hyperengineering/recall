# Recall MCP Integration Guide

This guide explains how to configure and use Recall with AI coding agents via the Model Context Protocol (MCP).

## Overview

Recall provides three MCP tools that enable AI agents to:
- **Query** existing lore for relevant context
- **Record** new learnings during workflows
- **Provide feedback** to improve lore quality over time

## Available MCP Tools

### recall_query

Retrieve relevant lore based on semantic similarity to a query.

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | Yes | - | Search query to find relevant lore |
| `k` | integer | No | 5 | Maximum number of results |
| `min_confidence` | number | No | 0.5 | Minimum confidence threshold (0.0-1.0) |
| `categories` | array | No | - | Filter by specific categories |

**Example:**

```json
{
  "query": "error handling patterns in Go",
  "k": 5,
  "min_confidence": 0.6,
  "categories": ["PATTERN_OUTCOME", "INTERFACE_LESSON"]
}
```

### recall_record

Capture lore from current experience for future recall.

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `content` | string | Yes | - | The lore content (max 4000 chars) |
| `category` | string | Yes | - | Category of lore (see below) |
| `context` | string | No | - | Additional context (story, epic, situation) |
| `confidence` | number | No | 0.5 | Initial confidence (0.0-1.0) |

**Categories:**

- `ARCHITECTURAL_DECISION` - System-level choices and rationale
- `PATTERN_OUTCOME` - Results of applying a design pattern
- `INTERFACE_LESSON` - Contract/API design insights
- `EDGE_CASE_DISCOVERY` - Scenarios found during implementation
- `IMPLEMENTATION_FRICTION` - Design-to-code translation difficulties
- `TESTING_STRATEGY` - Testing approach insights
- `DEPENDENCY_BEHAVIOR` - Library/framework gotchas
- `PERFORMANCE_INSIGHT` - Performance characteristics

**Example:**

```json
{
  "content": "SQLite WAL mode significantly improves concurrent read performance",
  "category": "PERFORMANCE_INSIGHT",
  "context": "story-4.2-database-optimization",
  "confidence": 0.8
}
```

### recall_feedback

Provide feedback on lore recalled during the session to adjust confidence.

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `helpful` | array | No | - | Session refs (L1, L2) of helpful lore |
| `not_relevant` | array | No | - | Session refs of irrelevant lore |
| `incorrect` | array | No | - | Session refs of wrong/misleading lore |

At least one of `helpful`, `not_relevant`, or `incorrect` must be provided.

**Example:**

```json
{
  "helpful": ["L1", "L3"],
  "not_relevant": ["L2"],
  "incorrect": ["L4"]
}
```

## Configuration for Claude Code

### Option 1: CLI-based MCP Server (Recommended)

Add Recall to your Claude Code MCP configuration in `~/.claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "recall": {
      "command": "recall",
      "args": ["mcp", "serve"],
      "env": {
        "RECALL_DB_PATH": "/path/to/your/lore.db",
        "RECALL_SOURCE_ID": "claude-code"
      }
    }
  }
}
```

**Note:** The `recall mcp serve` command is planned for a future release. For now, use Option 2.

### Option 2: CLI Tool Integration (Current)

Until the MCP server is available, configure Claude Code to use Recall via CLI commands.

**Project-level configuration** (`.claude/settings.json`):

```json
{
  "tools": {
    "recall_query": {
      "command": "recall query \"$query\" --top $k --json",
      "description": "Query lore for relevant context"
    },
    "recall_record": {
      "command": "recall record --content \"$content\" --category $category --json",
      "description": "Record new lore"
    }
  }
}
```

**Environment setup:**

```bash
# Add to your shell profile (~/.bashrc, ~/.zshrc, etc.)
export RECALL_DB_PATH="$HOME/.recall/lore.db"
export RECALL_SOURCE_ID="claude-code"

# Optional: For Engram sync
export ENGRAM_URL="https://engram.example.com"
export ENGRAM_API_KEY="your-api-key"
```

### Option 3: Custom MCP Server (Go Library)

For advanced integrations, use the Recall MCP package to build a custom server:

```go
package main

import (
    "log"
    "os"

    "github.com/hyperengineering/recall"
    recallmcp "github.com/hyperengineering/recall/mcp"
    "golang.org/x/tools/internal/mcp"
)

func main() {
    // Initialize Recall client
    client, err := recall.New(recall.Config{
        LocalPath: os.Getenv("RECALL_DB_PATH"),
        SourceID:  "mcp-server",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Create MCP server
    server := mcp.NewServer("recall", "1.0.0")

    // Register Recall tools
    recallmcp.RegisterTools(server, client)

    // Run server over stdio
    if err := server.Run(); err != nil {
        log.Fatal(err)
    }
}
```

## Usage Patterns for Coding Agents

### Pattern 1: Query Before Implementation

Before starting work on a story or feature, query for relevant lore:

```
Agent: "I'm starting work on implementing a message queue consumer."

→ recall_query: "message queue consumer implementation patterns"

Recall returns:
- L1: "Queue consumers benefit from idempotency checks" (confidence: 0.85)
- L2: "Dead letter queues prevent message loss on processing failures" (confidence: 0.72)
```

### Pattern 2: Record During Implementation

Capture insights as they emerge during coding:

```
Agent discovers that a library behaves unexpectedly:

→ recall_record: {
    "content": "The xyz library silently drops messages when buffer is full instead of blocking",
    "category": "DEPENDENCY_BEHAVIOR",
    "context": "story-3.1-message-processing",
    "confidence": 0.7
  }
```

### Pattern 3: Feedback After Validation

After using recalled lore, provide feedback on its usefulness:

```
Agent: "The idempotency pattern (L1) was exactly what I needed.
        The dead letter queue suggestion (L2) wasn't relevant for this use case."

→ recall_feedback: {
    "helpful": ["L1"],
    "not_relevant": ["L2"]
  }
```

### Pattern 4: Session-based Learning Loop

A complete learning cycle within a coding session:

```
1. START: Query for context
   → recall_query: "implementing REST API validation"

2. WORK: Implement the feature, using recalled lore as guidance

3. DISCOVER: Record new insights
   → recall_record: "OpenAPI schema validation should happen before business logic"

4. REFLECT: Provide feedback on what helped
   → recall_feedback: {"helpful": ["L1", "L3"], "incorrect": ["L2"]}

5. SYNC: Push learnings to Engram (if configured)
   → recall sync push
```

## Best Practices

### When to Query

- **At session start** - Load relevant context for the current task
- **Before major decisions** - Check for existing insights on architectural choices
- **When encountering friction** - Others may have documented similar challenges
- **During code review** - Validate patterns against known outcomes

### When to Record

- **After solving a non-obvious problem** - Future agents will benefit
- **When discovering edge cases** - Document unexpected behaviors
- **After architectural decisions** - Capture the rationale
- **When a pattern works well** - Reinforce successful approaches

### What Makes Good Lore

**Good lore is:**
- Specific and actionable
- Context-independent (works across projects)
- Validated through experience
- Categorized appropriately

**Examples of good lore:**

```
✓ "Batch database inserts with RETURNING clause to get IDs without N queries"
✓ "Go interfaces should be defined by consumers, not producers"
✓ "SQLite PRAGMA journal_mode=WAL improves concurrent read performance 10x"
```

**Examples of poor lore:**

```
✗ "The code didn't work" (too vague)
✗ "Use function X in file Y" (too project-specific)
✗ "This might be a good pattern" (not validated)
```

### Confidence Guidelines

| Starting Confidence | When to Use |
|---------------------|-------------|
| 0.3 - 0.4 | Hypothesis, first observation |
| 0.5 | Default, reasonable certainty |
| 0.6 - 0.7 | Validated in current context |
| 0.8+ | Repeatedly confirmed across contexts |

## Troubleshooting

### "No lore found" for queries

- Try broader search terms
- Lower the `min_confidence` threshold
- Check that `RECALL_DB_PATH` points to a populated database
- Run `recall sync bootstrap` to pull lore from Engram

### Session references (L1, L2) not working

Session references are valid only within the current session. Each CLI invocation is a separate session. For persistent references, use the full lore ID.

### Sync failures

- Verify `ENGRAM_URL` and `ENGRAM_API_KEY` are set correctly
- Check network connectivity to the Engram service
- Run `recall sync push --json` to see detailed error messages

## Database Location

The recommended database locations by platform:

| Platform | Path |
|----------|------|
| macOS | `~/.recall/lore.db` |
| Linux | `~/.local/share/recall/lore.db` |
| Windows | `%APPDATA%\recall\lore.db` |

Create the directory if it doesn't exist:

```bash
mkdir -p ~/.recall
export RECALL_DB_PATH="$HOME/.recall/lore.db"
```

## Multi-Environment Setup

For teams sharing lore across environments:

```bash
# Development workstation
export RECALL_DB_PATH="$HOME/.recall/lore.db"
export RECALL_SOURCE_ID="dev-$(hostname)"
export ENGRAM_URL="https://engram.company.com"
export ENGRAM_API_KEY="$ENGRAM_API_KEY"

# CI/CD pipeline
export RECALL_DB_PATH="/tmp/recall-ci.db"
export RECALL_SOURCE_ID="ci-${CI_JOB_ID}"
# Bootstrap from Engram at job start
recall sync bootstrap
```

## Security Considerations

- Store `ENGRAM_API_KEY` securely (use secrets management in CI)
- The local SQLite database contains all synced lore - protect it accordingly
- In shared environments, use separate `RECALL_SOURCE_ID` values per user/agent
