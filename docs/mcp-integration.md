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
        "RECALL_DB_PATH": "/Users/yourname/.recall/lore.db",
        "RECALL_SOURCE_ID": "claude-code"
      }
    }
  }
}
```

Restart Claude Code. You now have four new tools available.

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

**Example:**

```json
{
  "query": "error handling patterns",
  "k": 5,
  "min_confidence": 0.6,
  "categories": ["PATTERN_OUTCOME", "INTERFACE_LESSON"]
}
```

**Returns:** Array of lore entries with session references (L1, L2, L3...) for use with feedback.

### recall_record

Capture insights during implementation.

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `content` | string | Yes | — | The insight (max 4000 chars) |
| `category` | string | Yes | — | Category (see below) |
| `context` | string | No | — | Where this was learned |
| `confidence` | number | No | 0.5 | Initial confidence (0.0–1.0) |

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
  "confidence": 0.8
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

At least one parameter required.

**Example:**

```json
{
  "helpful": ["L1", "L3"],
  "not_relevant": ["L2"],
  "incorrect": ["L4"]
}
```

### recall_sync

Push local changes to Engram for team sharing (requires Engram configuration).

**Example:**

```json
{}
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

### Basic (Offline Only)

Works without any external service:

```json
{
  "mcpServers": {
    "recall": {
      "command": "recall",
      "args": ["mcp"],
      "env": {
        "RECALL_DB_PATH": "/Users/yourname/.recall/lore.db"
      }
    }
  }
}
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
        "RECALL_DB_PATH": "/Users/yourname/.recall/lore.db",
        "RECALL_SOURCE_ID": "claude-code-macbook",
        "ENGRAM_URL": "https://engram.example.com",
        "ENGRAM_API_KEY": "your-api-key"
      }
    }
  }
}
```

### Docker

```json
{
  "mcpServers": {
    "recall": {
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "-v", "/Users/yourname/.recall:/data",
        "-e", "RECALL_DB_PATH=/data/lore.db",
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

Recommended paths by platform:

| Platform | Path |
|----------|------|
| macOS | `~/.recall/lore.db` |
| Linux | `~/.local/share/recall/lore.db` |
| Windows | `%APPDATA%\recall\lore.db` |

Create the directory first:

```bash
mkdir -p ~/.recall
```
