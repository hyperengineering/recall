# MCP Integration Guide

This guide covers integrating Recall with AI coding assistants via the Model Context Protocol (MCP).

## Quick Setup for Claude Code

Add Recall to your Claude Code configuration:

**~/.claude/claude_desktop_config.json**

```json
{
  "mcpServers": {
    "recall": {
      "command": "recall",
      "args": ["mcp"],
      "env": {
        "RECALL_SOURCE_ID": "claude-code"
      }
    }
  }
}
```

Restart Claude Code. You now have six tools available.

### Multi-Store Configuration

To target a specific store by default:

```json
{
  "mcpServers": {
    "recall": {
      "command": "recall",
      "args": ["mcp"],
      "env": {
        "ENGRAM_STORE": "my-project",
        "RECALL_SOURCE_ID": "claude-code"
      }
    }
  }
}
```

Or specify `store` per-tool-call to query different knowledge bases.

## MCP Tools

### recall_query

Search for relevant lore before starting work.

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | Yes | — | What to search for |
| `k` | integer | No | 5 | Max results |
| `min_confidence` | number | No | 0.5 | Minimum confidence (0.0–1.0) |
| `categories` | array | No | all | Filter by categories |
| `store` | string | No | resolved | Target store ID |

**Example:**

```json
{
  "query": "error handling patterns",
  "k": 5,
  "min_confidence": 0.6,
  "categories": ["PATTERN_OUTCOME", "INTERFACE_LESSON"],
  "store": "my-project"
}
```

**Returns:** Array of lore entries with session references (L1, L2, L3...) for use with feedback. Session references are global across all stores queried in a session.

### recall_record

Capture insights during implementation.

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `content` | string | Yes | — | The insight (max 4000 chars) |
| `category` | string | Yes | — | Category (see below) |
| `context` | string | No | — | Where this was learned |
| `confidence` | number | No | 0.5 | Initial confidence (0.0–1.0) |
| `store` | string | No | resolved | Target store ID |

**Categories:**

- `ARCHITECTURAL_DECISION` — System-level design choices
- `PATTERN_OUTCOME` — Results of applying patterns
- `INTERFACE_LESSON` — API/contract design insights
- `EDGE_CASE_DISCOVERY` — Unexpected behaviors found
- `IMPLEMENTATION_FRICTION` — Design-to-code difficulties
- `TESTING_STRATEGY` — Testing approach insights
- `DEPENDENCY_BEHAVIOR` — Library/framework gotchas
- `PERFORMANCE_INSIGHT` — Performance characteristics

**Example:**

```json
{
  "content": "SQLite WAL mode improves concurrent read performance significantly",
  "category": "PERFORMANCE_INSIGHT",
  "context": "story-4.2-database-optimization",
  "confidence": 0.8,
  "store": "my-project"
}
```

### recall_feedback

Mark which lore helped (or didn't) to improve future recommendations.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `helpful` | array | No | Session refs of useful lore (+0.08 confidence) |
| `not_relevant` | array | No | Didn't apply to this context (no change) |
| `incorrect` | array | No | Wrong or misleading (-0.15 confidence) |
| `store` | string | No | Target store (only needed for direct lore IDs) |

At least one parameter required.

**Example:**

```json
{
  "helpful": ["L1", "L3"],
  "not_relevant": ["L2"],
  "incorrect": ["L4"]
}
```

**Note:** When using session references (L1, L2, etc.), the store is automatically resolved—feedback routes to the correct store where each lore entry was originally queried. The `store` parameter is only needed when providing direct lore IDs instead of session references.

### recall_sync

Synchronize local lore with Engram for team sharing (requires Engram configuration).

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `direction` | string | No | "both" | Sync direction: "pull", "push", or "both" |
| `store` | string | No | resolved | Target store ID |

**Example:**

```json
{
  "direction": "push",
  "store": "my-project"
}
```

### recall_store_list

List available stores. This is a read-only operation for discovering knowledge bases.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prefix` | string | No | Filter stores by path prefix (e.g., "team/") |

**Example:**

```json
{
  "prefix": "neuralmux/"
}
```

**Returns:**
```
Available stores (3):

  default
    Description: Default store for quick start
    Lore: 1,234 entries | Updated: 2h ago

  neuralmux/engram
    Description: Engram project knowledge base
    Lore: 567 entries | Updated: 15m ago

  acme/api-service
    Description: API service patterns
    Lore: 89 entries | Updated: 3d ago
```

### recall_store_info

Get detailed information about a specific store. Read-only operation for inspecting store metadata and statistics.

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `store` | string | No | resolved | Store ID to inspect |

**Example:**

```json
{
  "store": "my-project"
}
```

**Returns:**
```
Store: my-project
Description: My project knowledge base
Created: 2026-01-20 14:00:00 UTC
Updated: 2026-01-31 12:00:00 UTC

Statistics:
  Lore Count: 567
  Average Confidence: 0.72
  Validated Entries: 234 (41.3%)

Category Distribution:
  PATTERN_OUTCOME        145 (25.6%)
  DEPENDENCY_BEHAVIOR     98 (17.3%)
  ARCHITECTURAL_DECISION  87 (15.3%)
  ...
```

## Typical Workflow

```
1. START TASK
   Agent: "I'm implementing a message queue consumer"

   → recall_query: "message queue consumer patterns"

   Returns:
   - L1: "Queue consumers benefit from idempotency checks" (0.85)
   - L2: "Dead letter queues prevent message loss" (0.72)

2. IMPLEMENT
   Agent uses L1 insight, implements idempotency checks

3. DISCOVER SOMETHING NEW
   Agent finds the library drops messages when buffer is full

   → recall_record: {
       "content": "xyz library silently drops messages when buffer full",
       "category": "DEPENDENCY_BEHAVIOR",
       "context": "story-3.1",
       "confidence": 0.7
     }

4. PROVIDE FEEDBACK
   Agent: "L1 was exactly right, L2 wasn't relevant here"

   → recall_feedback: {
       "helpful": ["L1"],
       "not_relevant": ["L2"]
     }
```

## Configuration Options

You can configure Recall MCP either by editing the JSON config file or using the CLI one-liner.

**Important:** When using `claude mcp add`, environment variables (`-e` flags) must come **before** the `--` separator. The command after `--` is what gets executed.

### Basic (Offline Only)

Works without any external service, using the default store:

```json
{
  "mcpServers": {
    "recall": {
      "command": "recall",
      "args": ["mcp"],
      "env": {
        "RECALL_SOURCE_ID": "claude-code"
      }
    }
  }
}
```

**CLI one-liner:**

```bash
claude mcp add recall -e RECALL_SOURCE_ID=claude-code -- recall mcp
```

### Project-Specific Store

Target a specific knowledge base for your project:

```json
{
  "mcpServers": {
    "recall": {
      "command": "recall",
      "args": ["mcp"],
      "env": {
        "ENGRAM_STORE": "my-project",
        "RECALL_SOURCE_ID": "claude-code"
      }
    }
  }
}
```

**CLI one-liner:**

```bash
claude mcp add recall -e ENGRAM_STORE=my-project -e RECALL_SOURCE_ID=claude-code -- recall mcp
```

### With Engram (Team Sync)

Share lore across environments:

```json
{
  "mcpServers": {
    "recall": {
      "command": "recall",
      "args": ["mcp"],
      "env": {
        "ENGRAM_STORE": "team/project",
        "RECALL_SOURCE_ID": "claude-code-macbook",
        "ENGRAM_URL": "https://engram.example.com",
        "ENGRAM_API_KEY": "your-api-key"
      }
    }
  }
}
```

**CLI one-liner:**

```bash
claude mcp add recall \
  -e ENGRAM_STORE=team/project \
  -e RECALL_SOURCE_ID=claude-code-macbook \
  -e ENGRAM_URL=https://engram.example.com \
  -e ENGRAM_API_KEY=your-api-key \
  -- recall mcp
```

### Docker

```json
{
  "mcpServers": {
    "recall": {
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "-v", "/Users/yourname/.recall:/root/.recall",
        "-e", "ENGRAM_STORE=my-project",
        "ghcr.io/hyperengineering/recall:latest",
        "mcp"
      ]
    }
  }
}
```

## Session References

When you query lore, each result gets a session reference (L1, L2, L3...).

These references:
- Are valid for the current MCP server session
- Reset when Claude Code restarts
- Can be used with `recall_feedback` to mark what helped
- Are **global across stores**—if you query store A (gets L1-L3) then store B (gets L4-L5), feedback on L4 automatically routes to store B

If you need to reference lore after a restart, use the full lore ID instead.

## Best Practices

### When to Query

- **Before starting a task** — Get context on similar past work
- **Before architectural decisions** — Check for existing insights
- **When hitting friction** — Others may have documented the same issue

### When to Record

- **After solving a non-obvious problem** — Future you will thank present you
- **When discovering edge cases** — Document unexpected behaviors
- **After architectural decisions** — Capture the rationale
- **When a pattern works well** — Reinforce successful approaches

### What Makes Good Lore

**Good:**
```
"Batch database inserts with RETURNING clause avoid N queries for IDs"
"Go interfaces should be defined by consumers, not producers"
"SQLite PRAGMA journal_mode=WAL improves concurrent reads 10x"
```

**Not useful:**
```
"The code didn't work" (too vague)
"Use function X in file Y" (too project-specific)
"This might be a good pattern" (not validated)
```

### Confidence Guidelines

| Initial Confidence | When to Use |
|-------------------|-------------|
| 0.3–0.4 | First observation, hypothesis |
| 0.5 | Reasonable certainty (default) |
| 0.6–0.7 | Validated in current context |
| 0.8+ | Confirmed across multiple contexts |

## Troubleshooting

### "No lore found"

- Try broader search terms
- Lower `min_confidence` to 0.3
- Run `recall sync bootstrap` to pull from Engram

### Session references not working

Session references (L1, L2) only persist while the MCP server is running. When Claude Code restarts, you get a fresh session.

For persistent references, use the full lore ID (shown in query results).

### Sync errors

- Check `ENGRAM_URL` and `ENGRAM_API_KEY` are set
- Verify network connectivity to Engram
- Use `recall sync push --json` from CLI to see detailed errors

## Database Locations

With multi-store support, each store has its own database:

```
~/.recall/stores/{store-id}/lore.db
```

Examples:
- `~/.recall/stores/default/lore.db` — Default store
- `~/.recall/stores/my-project/lore.db` — Simple store ID
- `~/.recall/stores/neuralmux__engram/lore.db` — Path-style ID (`/` encoded as `__`)

The directories are created automatically when you create or use a store.

### Legacy Single-Store Migration

If you have an existing database from before multi-store support, Recall automatically migrates it to `~/.recall/stores/default/lore.db` on first run.
