# Recall Multi-Store Support - Technical Design

**Author:** Clario (Architecture Agent)
**Date:** 2026-01-31
**Status:** Fully Implemented (All Stories 7.1-7.6 Complete)

---

## Overview

This technical design document specifies how Recall (MCP tools and CLI) will support multi-store operations against Engram. Multi-store support enables project isolation, allowing teams and agents to maintain separate knowledge bases while preserving the zero-config quick start experience via a `default` store.

Recall is the client-side library and CLI that agents use to record and query lore. This document focuses exclusively on Recall's implementation changes to support multi-store operations against an Engram backend that provides store management APIs.

---

## Design Goals

1. **Project Isolation** - Separate lore stores for different projects, teams, or contexts prevent cross-contamination of knowledge and enable focused recall within specific domains.

2. **Zero-Config Quick Start** - The `default` store provides immediate usability without any configuration. Users can start recording and recalling lore instantly.

3. **Explicit Store Context** - For multi-project workflows, users can explicitly specify which store to operate against via parameter, environment variable, or configuration file.

4. **Read-Only MCP Store Operations** - MCP tools can list and inspect stores but cannot create or delete them. This prevents accidental store lifecycle operations during agent workflows.

5. **Full CLI Store Lifecycle** - The CLI provides complete store management: create, delete, export, and import operations for operator control.

---

## Store ID Format

### Specification

Store IDs follow a path-style format enabling hierarchical organization:

```
<segment>[/<segment>]*
```

**Format Rules:**
- **Segments:** 1 to 4 path segments separated by `/`
- **Segment Characters:** Lowercase alphanumeric characters and hyphens (`a-z`, `0-9`, `-`)
- **Segment Length:** 1 to 64 characters per segment
- **No Leading/Trailing Hyphens:** Each segment must not start or end with a hyphen
- **No Consecutive Hyphens:** Segments cannot contain `--`
- **Total Length:** Maximum 256 characters for the complete store ID

### Validation Regex

```regex
^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?(\/[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?){0,3}$
```

### Examples

**Valid Store IDs:**
```
default
my-project
neuralmux/engram
acme-corp/team-alpha/service-api
org/team/project/environment
```

**Invalid Store IDs:**
```
My-Project          # uppercase not allowed
-my-project         # leading hyphen
my-project-         # trailing hyphen
my--project         # consecutive hyphens
a/b/c/d/e           # exceeds 4 levels
project_name        # underscore not allowed
```

### Reserved Store IDs

| Store ID | Purpose |
|----------|---------|
| `default` | Zero-config quick start store |
| `_system` | Reserved for future internal use |

---

## Store Resolution Strategy

When a store context is needed but not explicitly provided, Recall resolves the store using the following priority chain:

### Resolution Order

```
1. Explicit Parameter    (highest priority)
2. ENGRAM_STORE env var
3. BMAD config file
4. "default"             (lowest priority)
```

### Pseudocode

```go
func ResolveStore(explicitStore string) string {
    // 1. Explicit parameter takes precedence
    if explicitStore != "" {
        if !isValidStoreID(explicitStore) {
            return error("INVALID_STORE_ID")
        }
        return explicitStore
    }

    // 2. Environment variable
    if envStore := os.Getenv("ENGRAM_STORE"); envStore != "" {
        if !isValidStoreID(envStore) {
            return error("INVALID_STORE_ID")
        }
        return envStore
    }

    // 3. BMAD config file (bmad.yaml in project root or ~/.config/bmad/config.yaml)
    if configStore := loadBMADConfig().EngramStore; configStore != "" {
        if !isValidStoreID(configStore) {
            return error("INVALID_STORE_ID")
        }
        return configStore
    }

    // 4. Default fallback
    return "default"
}
```

### Resolution Flow Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                    Store Resolution                          │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌─────────────────┐                                        │
│  │ Explicit param? │──yes──► Validate ──► Return store      │
│  └────────┬────────┘              │                         │
│           │ no                    └─invalid─► Error          │
│           ▼                                                  │
│  ┌─────────────────┐                                        │
│  │ ENGRAM_STORE?   │──yes──► Validate ──► Return store      │
│  └────────┬────────┘              │                         │
│           │ no                    └─invalid─► Error          │
│           ▼                                                  │
│  ┌─────────────────┐                                        │
│  │ BMAD config?    │──yes──► Validate ──► Return store      │
│  └────────┬────────┘              │                         │
│           │ no                    └─invalid─► Error          │
│           ▼                                                  │
│  ┌─────────────────┐                                        │
│  │ Return "default"│                                        │
│  └─────────────────┘                                        │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## MCP Tool Changes

### Existing Tools (Updated)

All existing MCP tools gain an optional `store` parameter. When omitted, the store is resolved using the resolution strategy above.

#### recall_query

**Description:** Retrieve relevant lore based on semantic similarity from a specific store.

**Updated Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | yes | - | Search query for semantic matching |
| `k` | integer | no | 5 | Maximum number of results |
| `min_confidence` | number | no | 0.5 | Minimum confidence threshold |
| `categories` | array | no | all | Filter by lore categories |
| `store` | string | no | resolved | Target store ID |

**Behavior:**
- If `store` is provided, validate format and use directly
- If `store` is omitted, resolve using the priority chain
- If resolved store does not exist, return `STORE_NOT_FOUND` error

**Example Usage:**
```json
{
  "name": "recall_query",
  "arguments": {
    "query": "queue consumer idempotency patterns",
    "k": 5,
    "store": "neuralmux/engram"
  }
}
```

#### recall_record

**Description:** Capture lore from current experience into a specific store.

**Updated Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `content` | string | yes | - | The lore content (max 4000 chars) |
| `category` | string | yes | - | Lore category enum |
| `context` | string | no | - | Additional context (max 1000 chars) |
| `confidence` | number | no | 0.7 | Initial confidence score |
| `store` | string | no | resolved | Target store ID |

**Behavior:**
- If `store` is provided, validate format and use directly
- If `store` is omitted, resolve using the priority chain
- If resolved store does not exist, return `STORE_NOT_FOUND` error

**Example Usage:**
```json
{
  "name": "recall_record",
  "arguments": {
    "content": "SQLite WAL mode requires PRAGMA journal_mode=WAL before any writes",
    "category": "DEPENDENCY_BEHAVIOR",
    "context": "engram database initialization",
    "store": "neuralmux/engram"
  }
}
```

#### recall_feedback

**Description:** Provide feedback on lore recalled this session in a specific store.

**Updated Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `helpful` | array | no | [] | Session refs or IDs of helpful lore |
| `not_relevant` | array | no | [] | Session refs or IDs of irrelevant lore |
| `incorrect` | array | no | [] | Session refs or IDs of incorrect lore |
| `store` | string | no | resolved | Target store ID |

**Behavior:**
- If `store` is provided, validate format and use directly
- If `store` is omitted, resolve using the priority chain
- If resolved store does not exist, return `STORE_NOT_FOUND` error
- Lore IDs must belong to the specified store

**Example Usage:**
```json
{
  "name": "recall_feedback",
  "arguments": {
    "helpful": ["L1", "L2"],
    "not_relevant": ["L3"],
    "store": "neuralmux/engram"
  }
}
```

#### recall_sync

**Description:** Synchronize local lore with Engram for a specific store.

**Updated Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `direction` | string | no | "both" | Sync direction: "pull", "push", or "both" |
| `store` | string | no | resolved | Target store ID |

**Behavior:**
- If `store` is provided, validate format and use directly
- If `store` is omitted, resolve using the priority chain
- If resolved store does not exist, return `STORE_NOT_FOUND` error

**Example Usage:**
```json
{
  "name": "recall_sync",
  "arguments": {
    "direction": "pull",
    "store": "neuralmux/engram"
  }
}
```

---

### New Tools

#### recall_store_list

**Description:** List available stores from Engram. This is a read-only operation suitable for agent discovery of available knowledge bases.

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `prefix` | string | no | - | Filter stores by path prefix |

**Response Schema:**

```json
{
  "stores": [
    {
      "id": "default",
      "description": "Default store for quick start",
      "lore_count": 1234,
      "created_at": "2026-01-15T10:30:00Z",
      "updated_at": "2026-01-31T08:00:00Z"
    },
    {
      "id": "neuralmux/engram",
      "description": "Engram project knowledge base",
      "lore_count": 567,
      "created_at": "2026-01-20T14:00:00Z",
      "updated_at": "2026-01-31T12:00:00Z"
    }
  ],
  "total": 2
}
```

**Example Usage:**

List all stores:
```json
{
  "name": "recall_store_list",
  "arguments": {}
}
```

List stores under a prefix:
```json
{
  "name": "recall_store_list",
  "arguments": {
    "prefix": "neuralmux/"
  }
}
```

#### recall_store_info

**Description:** Get detailed information about a specific store. This is a read-only operation for inspecting store metadata and statistics.

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `store` | string | no | resolved | Store ID to inspect |

**Response Schema:**

```json
{
  "id": "neuralmux/engram",
  "description": "Engram project knowledge base",
  "created_at": "2026-01-20T14:00:00Z",
  "updated_at": "2026-01-31T12:00:00Z",
  "stats": {
    "lore_count": 567,
    "category_distribution": {
      "PATTERN_OUTCOME": 145,
      "DEPENDENCY_BEHAVIOR": 98,
      "ARCHITECTURAL_DECISION": 87,
      "TESTING_STRATEGY": 72,
      "EDGE_CASE_DISCOVERY": 65,
      "IMPLEMENTATION_FRICTION": 45,
      "INTERFACE_LESSON": 32,
      "PERFORMANCE_INSIGHT": 23
    },
    "average_confidence": 0.72,
    "validated_count": 234,
    "last_activity": "2026-01-31T12:00:00Z"
  }
}
```

**Example Usage:**

Get info for resolved store:
```json
{
  "name": "recall_store_info",
  "arguments": {}
}
```

Get info for specific store:
```json
{
  "name": "recall_store_info",
  "arguments": {
    "store": "neuralmux/engram"
  }
}
```

---

## CLI Commands

### Store Management Commands

The CLI provides full store lifecycle management capabilities.

#### recall store list

**Description:** List all available stores with optional filtering.

**Usage:**
```bash
recall store list [--prefix PREFIX] [--format FORMAT]
```

**Arguments & Flags:**

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--prefix` | `-p` | string | - | Filter by store ID prefix |
| `--format` | `-f` | string | table | Output format: table, json, yaml |

**Output (table format):**
```
STORE ID             DESCRIPTION                      LORE COUNT  UPDATED
default              Default store for quick start    1234        2h ago
neuralmux/engram     Engram project knowledge base    567         15m ago
acme/service-api     API service patterns             89          3d ago
```

**Output (JSON format):**
```json
{
  "stores": [
    {
      "id": "default",
      "description": "Default store for quick start",
      "lore_count": 1234,
      "created_at": "2026-01-15T10:30:00Z",
      "updated_at": "2026-01-31T08:00:00Z"
    }
  ],
  "total": 1
}
```

**Examples:**
```bash
# List all stores
recall store list

# List stores under prefix
recall store list --prefix neuralmux/

# JSON output for scripting
recall store list --format json
```

---

#### recall store create

**Description:** Create a new store in Engram.

**Usage:**
```bash
recall store create <store-id> [--description DESC]
```

**Arguments & Flags:**

| Argument/Flag | Type | Required | Description |
|---------------|------|----------|-------------|
| `<store-id>` | string | yes | Store ID to create |
| `--description` | string | no | Human-readable description |

**Output:**
```
Store created: neuralmux/engram
Description: Engram project knowledge base
```

**Exit Codes:**
- `0` - Success
- `1` - Invalid store ID format
- `2` - Store already exists
- `3` - Permission denied
- `4` - Network/API error

**Examples:**
```bash
# Create with description
recall store create neuralmux/engram --description "Engram project knowledge base"

# Create simple store
recall store create my-project
```

---

#### recall store delete

**Description:** Delete a store and all its lore from Engram. This is a destructive operation requiring confirmation.

**Usage:**
```bash
recall store delete <store-id> --confirm
```

**Arguments & Flags:**

| Argument/Flag | Type | Required | Description |
|---------------|------|----------|-------------|
| `<store-id>` | string | yes | Store ID to delete |
| `--confirm` | flag | yes | Required safety flag |
| `--force` | flag | no | Skip interactive prompt |

**Behavior:**
- Without `--confirm`, the command fails with usage hint
- With `--confirm` but without `--force`, prompts for interactive confirmation
- With both `--confirm` and `--force`, deletes immediately

**Output:**
```
WARNING: This will permanently delete store 'neuralmux/engram' and all 567 lore entries.
Type 'neuralmux/engram' to confirm: neuralmux/engram
Store deleted: neuralmux/engram
```

**Exit Codes:**
- `0` - Success
- `1` - Invalid store ID
- `2` - Store not found
- `3` - Confirmation not provided
- `4` - Cannot delete 'default' store
- `5` - Network/API error

**Examples:**
```bash
# Delete with interactive confirmation
recall store delete neuralmux/engram --confirm

# Delete without prompts (scripting)
recall store delete neuralmux/engram --confirm --force
```

---

#### recall store info

**Description:** Display detailed information about a specific store.

**Usage:**
```bash
recall store info <store-id> [--format FORMAT]
```

**Arguments & Flags:**

| Argument/Flag | Type | Required | Description |
|---------------|------|----------|-------------|
| `<store-id>` | string | no | Store ID (defaults to resolved store) |
| `--format` | string | no | Output format: text, json, yaml |

**Output (text format):**
```
Store: neuralmux/engram
Description: Engram project knowledge base
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
  TESTING_STRATEGY        72 (12.7%)
  EDGE_CASE_DISCOVERY     65 (11.5%)
  IMPLEMENTATION_FRICTION 45 (7.9%)
  INTERFACE_LESSON        32 (5.6%)
  PERFORMANCE_INSIGHT     23 (4.1%)
```

**Examples:**
```bash
# Info for specific store
recall store info neuralmux/engram

# Info for current resolved store
recall store info

# JSON output
recall store info neuralmux/engram --format json
```

---

#### recall store export

**Description:** Export a store's lore to a file for backup or migration.

**Usage:**
```bash
recall store export <store-id> -o <file> [--format FORMAT]
```

**Arguments & Flags:**

| Argument/Flag | Type | Required | Description |
|---------------|------|----------|-------------|
| `<store-id>` | string | yes | Store ID to export |
| `-o, --output` | string | yes | Output file path |
| `--format` | string | no | Export format: json (default), sqlite |

**Export Format (JSON):**
```json
{
  "version": "1.0",
  "exported_at": "2026-01-31T12:00:00Z",
  "store_id": "neuralmux/engram",
  "metadata": {
    "description": "Engram project knowledge base",
    "created_at": "2026-01-20T14:00:00Z"
  },
  "lore": [
    {
      "id": "01HXK4...",
      "content": "SQLite WAL mode requires...",
      "category": "DEPENDENCY_BEHAVIOR",
      "confidence": 0.85,
      "...": "..."
    }
  ]
}
```

**Examples:**
```bash
# Export to JSON
recall store export neuralmux/engram -o backup.json

# Export as SQLite database
recall store export neuralmux/engram -o backup.db --format sqlite
```

---

#### recall store import

**Description:** Import lore into a store from an export file.

**Usage:**
```bash
recall store import <store-id> -i <file> [--merge-strategy STRATEGY]
```

**Arguments & Flags:**

| Argument/Flag | Type | Required | Description |
|---------------|------|----------|-------------|
| `<store-id>` | string | yes | Target store ID |
| `-i, --input` | string | yes | Input file path |
| `--merge-strategy` | string | no | How to handle conflicts: skip, replace, merge (default: merge) |
| `--dry-run` | flag | no | Preview import without changes |

**Merge Strategies:**
- `skip` - Skip entries that already exist (by ID)
- `replace` - Replace existing entries with imported versions
- `merge` - Apply Engram's standard deduplication/merge logic

**Output:**
```
Importing from backup.json into neuralmux/engram...
  Total entries: 567
  New entries: 423
  Merged: 89
  Skipped: 55
Import complete.
```

**Examples:**
```bash
# Import with merge (default)
recall store import neuralmux/engram -i backup.json

# Preview import
recall store import neuralmux/engram -i backup.json --dry-run

# Replace existing entries
recall store import neuralmux/engram -i backup.json --merge-strategy replace
```

---

## Configuration Integration

### BMAD Config

Store context can be set in BMAD configuration files for project-level defaults.

**File Locations (in priority order):**
1. `./bmad.yaml` - Project-local config
2. `~/.config/bmad/config.yaml` - User global config

**Configuration Schema:**

```yaml
# bmad.yaml
engram_store: "neuralmux/engram"
```

**Full Example:**

```yaml
# bmad.yaml - Project configuration
engram_store: "acme-corp/api-service"

# Other BMAD settings...
agents:
  default: spark
```

### Environment Variables

**ENGRAM_STORE:**

Sets the default store context for all Recall operations.

```bash
# Set in shell
export ENGRAM_STORE="neuralmux/engram"

# Set per-command
ENGRAM_STORE=my-project recall query "patterns"

# In docker-compose.yml
environment:
  - ENGRAM_STORE=neuralmux/engram

# In .env file
ENGRAM_STORE=neuralmux/engram
```

### Priority Examples

**Scenario 1: All sources set**
```bash
# Environment
export ENGRAM_STORE="env-store"

# bmad.yaml
engram_store: "config-store"

# Command with explicit store
recall query "patterns" --store explicit-store
```
Result: Uses `explicit-store` (explicit parameter wins)

**Scenario 2: Environment and config set**
```bash
# Environment
export ENGRAM_STORE="env-store"

# bmad.yaml
engram_store: "config-store"

# Command without explicit store
recall query "patterns"
```
Result: Uses `env-store` (environment beats config)

**Scenario 3: Only config set**
```yaml
# bmad.yaml
engram_store: "config-store"
```
```bash
# No ENGRAM_STORE environment variable
recall query "patterns"
```
Result: Uses `config-store`

**Scenario 4: Nothing set**
```bash
# No environment, no config
recall query "patterns"
```
Result: Uses `default`

---

## Error Handling

### Error Codes

| Error Code | HTTP Status | Description |
|------------|-------------|-------------|
| `STORE_NOT_FOUND` | 404 | Specified store does not exist |
| `STORE_ALREADY_EXISTS` | 409 | Cannot create store; ID already taken |
| `INVALID_STORE_ID` | 400 | Store ID fails format validation |
| `STORE_CONTEXT_REQUIRED` | 400 | Store could not be resolved and is required |
| `STORE_DELETE_PROTECTED` | 403 | Cannot delete protected store (e.g., `default`) |
| `STORE_EXPORT_FAILED` | 500 | Export operation failed |
| `STORE_IMPORT_FAILED` | 500 | Import operation failed |

### Error Response Format

All errors follow RFC 7807 Problem Details format, consistent with existing Engram API patterns.

**Example: Store Not Found**

```json
{
  "type": "https://engram.dev/errors/store-not-found",
  "title": "Store Not Found",
  "status": 404,
  "detail": "Store 'neuralmux/engram' does not exist. Use 'recall store create' to create it.",
  "instance": "/api/v1/stores/neuralmux/engram"
}
```

**Example: Invalid Store ID**

```json
{
  "type": "https://engram.dev/errors/invalid-store-id",
  "title": "Invalid Store ID",
  "status": 400,
  "detail": "Store ID 'My-Project' is invalid. Store IDs must be lowercase alphanumeric with hyphens, 1-4 path segments separated by '/'.",
  "instance": "/api/v1/stores/My-Project",
  "errors": [
    {
      "field": "store_id",
      "message": "uppercase characters not allowed",
      "value": "My-Project"
    }
  ]
}
```

**Example: Store Already Exists**

```json
{
  "type": "https://engram.dev/errors/store-already-exists",
  "title": "Store Already Exists",
  "status": 409,
  "detail": "Store 'neuralmux/engram' already exists.",
  "instance": "/api/v1/stores/neuralmux/engram"
}
```

---

## Migration Path

### For Existing Users

**Scenario: User with existing local lore database**

Existing lore databases work unchanged. The store resolution defaults to `default`, so all operations continue targeting the implicit default store. No action required.

**Scenario: User wants to migrate existing lore to a named store**

1. Create the new store:
   ```bash
   recall store create my-project --description "My project lore"
   ```

2. Export from default (or local):
   ```bash
   recall store export default -o migration.json
   ```

3. Import to new store:
   ```bash
   recall store import my-project -i migration.json
   ```

4. Update configuration:
   ```yaml
   # bmad.yaml
   engram_store: "my-project"
   ```

### Zero-Config Users

Users who never configure a store context experience no change:

- All tools default to the `default` store
- The `default` store is auto-created on first use (by Engram)
- No configuration required
- No migration required

### Deprecation Notes

None. Multi-store support is purely additive. Existing behavior is preserved for all users who do not explicitly adopt multi-store features.

---

## API Contract with Engram

Recall will call these Engram API endpoints. Engram team is responsible for implementing these endpoints.

### Endpoints Recall Will Call

#### Store Listing

```
GET /api/v1/stores
```

**Query Parameters:**
- `prefix` (optional): Filter stores by ID prefix

**Response:**
```json
{
  "stores": [
    {
      "id": "default",
      "description": "Default store",
      "lore_count": 1234,
      "created_at": "2026-01-15T10:30:00Z",
      "updated_at": "2026-01-31T08:00:00Z"
    }
  ],
  "total": 1
}
```

**Errors:**
- 401 Unauthorized
- 500 Internal Server Error

---

#### Store Info

```
GET /api/v1/stores/{store_id}
```

**Path Parameters:**
- `store_id`: URL-encoded store ID (e.g., `neuralmux%2Fengram`)

**Response:**
```json
{
  "id": "neuralmux/engram",
  "description": "Engram project knowledge base",
  "created_at": "2026-01-20T14:00:00Z",
  "updated_at": "2026-01-31T12:00:00Z",
  "stats": {
    "lore_count": 567,
    "category_distribution": {
      "PATTERN_OUTCOME": 145
    },
    "average_confidence": 0.72,
    "validated_count": 234
  }
}
```

**Errors:**
- 401 Unauthorized
- 404 Store Not Found
- 500 Internal Server Error

---

#### Store Creation (CLI Only)

```
POST /api/v1/stores
```

**Request:**
```json
{
  "id": "neuralmux/engram",
  "description": "Engram project knowledge base"
}
```

**Response (201 Created):**
```json
{
  "id": "neuralmux/engram",
  "description": "Engram project knowledge base",
  "created_at": "2026-01-31T12:00:00Z"
}
```

**Errors:**
- 400 Invalid Store ID
- 401 Unauthorized
- 409 Store Already Exists
- 500 Internal Server Error

---

#### Store Deletion (CLI Only)

```
DELETE /api/v1/stores/{store_id}
```

**Path Parameters:**
- `store_id`: URL-encoded store ID

**Response:** 204 No Content

**Errors:**
- 401 Unauthorized
- 403 Cannot delete protected store
- 404 Store Not Found
- 500 Internal Server Error

---

#### Lore Operations (Updated)

All existing lore endpoints gain store context via path prefix:

```
POST   /api/v1/stores/{store_id}/lore
GET    /api/v1/stores/{store_id}/lore/snapshot
GET    /api/v1/stores/{store_id}/lore/delta
POST   /api/v1/stores/{store_id}/lore/feedback
DELETE /api/v1/stores/{store_id}/lore/{lore_id}
```

**Note:** For backward compatibility, Engram may continue to support the original paths (`/api/v1/lore/*`) which implicitly target the `default` store. This decision is deferred to Engram implementation.

---

#### Store Export (CLI Only)

```
GET /api/v1/stores/{store_id}/export
```

**Query Parameters:**
- `format`: `json` (default) or `sqlite`

**Response:**
- Content-Type: `application/json` or `application/octet-stream`
- Body: Export data

---

#### Store Import (CLI Only)

```
POST /api/v1/stores/{store_id}/import
```

**Query Parameters:**
- `merge_strategy`: `skip`, `replace`, or `merge` (default)
- `dry_run`: `true` or `false` (default)

**Request Body:** Export file content

**Response:**
```json
{
  "total": 567,
  "created": 423,
  "merged": 89,
  "skipped": 55,
  "errors": []
}
```

---

## Implementation Notes

### Store ID URL Encoding

Store IDs containing `/` must be URL-encoded in path parameters:
- `neuralmux/engram` becomes `neuralmux%2Fengram`

Recall CLI and MCP tools handle this encoding automatically.

### Local Store Caching

Each store has its own local SQLite replica:
- Location: `~/.recall/stores/{encoded_store_id}/lore.db`
- Encoding: Store ID with `/` replaced by `__`
- Example: `~/.recall/stores/neuralmux__engram/lore.db`

### Session Tracking Per Store

Session tracking (L1, L2, etc.) is scoped per store:
- Each store maintains independent session counters
- Feedback references only resolve within the same store context

---

## Suggested Story Breakdown

### Story 1: Store Resolution Infrastructure

**Scope:** Implement store resolution logic in Recall

**Tasks:**
- Implement `ResolveStore()` function with priority chain
- Add store ID validation function with regex
- Load BMAD config for `engram_store` setting
- Read `ENGRAM_STORE` environment variable
- Add `--store` flag to existing CLI commands
- Add `store` parameter to existing MCP tool schemas
- Unit tests for resolution priority and validation

**Acceptance Criteria:**
- Store resolution follows documented priority order
- Invalid store IDs are rejected with clear error messages
- Existing commands work unchanged when store not specified

---

### Story 2: MCP Store Discovery Tools

**Scope:** Implement `recall_store_list` and `recall_store_info` MCP tools

**Tasks:**
- Implement `recall_store_list` tool with prefix filtering
- Implement `recall_store_info` tool with stats
- Add new tools to MCP registry
- Update MCP tool documentation
- Integration tests against Engram API

**Acceptance Criteria:**
- Agents can list available stores
- Agents can inspect store details and statistics
- Tools are read-only (no create/delete capability)

---

### Story 3: CLI Store Lifecycle Commands

**Scope:** Implement store management CLI commands

**Tasks:**
- Implement `recall store list` with table/JSON output
- Implement `recall store create` with description
- Implement `recall store delete` with confirmation flow
- Implement `recall store info` with statistics display
- Add help text and examples for all commands

**Acceptance Criteria:**
- Operators can create, list, and delete stores via CLI
- Delete operation requires explicit confirmation
- Output formats support both human and machine consumption

---

### Story 4: CLI Store Export/Import

**Scope:** Implement store data portability commands

**Tasks:**
- Implement `recall store export` with JSON and SQLite formats
- Implement `recall store import` with merge strategies
- Add `--dry-run` support for import preview
- Handle large exports with streaming
- Validate import file format before processing

**Acceptance Criteria:**
- Stores can be exported for backup
- Stores can be imported with configurable conflict handling
- Dry-run mode previews changes without modification

---

## Open Questions

1. **Backward Compatibility API Routes:** Should Engram continue to support `/api/v1/lore/*` routes as aliases for `/api/v1/stores/default/lore/*`? This simplifies migration but adds API surface.

2. **Store Quotas:** Should stores have configurable lore count limits or size quotas? Not addressed in current design; may be Phase 2 consideration.

3. **Store Permissions:** Current design uses single API key for all stores. Should stores have individual access controls? Deferred to multi-tenancy work.

4. **Cross-Store Queries:** Should agents be able to query across multiple stores in a single operation? Current design requires explicit store targeting.

5. **Store Soft Delete:** Should deleted stores be recoverable for a grace period? Current design is immediate hard delete.

---

The path is clear. Build well.

---

## Implementation Notes (2026-01-31)

### Completed Stories

| Story | Description | Status |
|-------|-------------|--------|
| 7.1 | Store Infrastructure & Resolution | ✅ Complete |
| 7.2 | CLI Store Lifecycle Commands | ✅ Complete |
| 7.3 | MCP Multi-Store Support | ✅ Complete |
| 7.4 | CLI Store Export/Import | ✅ Complete |
| 7.5 | Engram Multi-Store Sync | ✅ Complete |
| 7.6 | CLI Output Polish | ✅ Complete |

### Implementation Deviations

1. **BMAD config not implemented:** Store resolution uses explicit param > `ENGRAM_STORE` env > "default". BMAD config file reading was descoped per user request.

2. **Store ID max length:** Updated to 128 characters per OpenAPI spec (Story 7.5).

### Key Files

- `internal/store/storeid.go` — Store ID validation (128 char max)
- `internal/store/resolve.go` — Store resolution logic
- `internal/store/path.go` — Store path encoding
- `internal/store/migrate.go` — Legacy database migration
- `internal/sync/client.go` — Engram HTTP client with store-prefixed APIs
- `internal/sync/types.go` — Store management types (ListStoresResponse, StoreInfoResponse, etc.)
- `cmd/recall/store.go` — CLI store commands (including `--remote` flag)
- `cmd/recall/store_export.go` — Export command
- `cmd/recall/store_import.go` — Import command
- `cmd/recall/styles.go` — CLI styling (lipgloss)
- `mcp/server.go` — MCP tools with store parameter
- `sync.go` — Syncer with store context (SetStoreID)
