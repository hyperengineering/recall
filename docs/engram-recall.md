# Engram System — Technical Design

> **Purpose:** Enable AI agents to accumulate, persist, and recall experiential knowledge (lore) across sessions, projects, and distributed development environments.

**Version:** 1.0
**Status:** MVP Complete

---

## Executive Summary

The Engram system gives AI agents persistent memory:

- **Recall** (this library) — Local client that stores, searches, and retrieves lore
- **Engram** (central service) — Optional server that syncs lore across environments
- **Lore** — Individual pieces of experiential knowledge

```
┌─────────────────────────────────────────────────────────────────┐
│  Environment A              Environment B              Env N    │
│  ┌─────────────┐           ┌─────────────┐                     │
│  │   Recall    │           │   Recall    │            ...      │
│  │  (SQLite)   │           │  (SQLite)   │                     │
│  └──────┬──────┘           └──────┬──────┘                     │
└─────────┼──────────────────────────┼────────────────────────────┘
          │                          │
          └──────────┬───────────────┘
                     │ (optional sync)
              ┌──────▼──────┐
              │   Engram    │
              │  (central)  │
              └─────────────┘
```

---

## Naming Convention

| Term | Role | Description |
|------|------|-------------|
| **Engram** | Central service | Where lore is stored and synchronized |
| **Recall** | Local client | How agents retrieve and contribute lore |
| **Lore** | Knowledge unit | Individual learnings—the substance itself |

---

## 1. Problem Statement

AI agents generate valuable insights during technical work:
- Architectural decisions and their outcomes
- Pattern applications that succeeded or failed
- Edge cases discovered during implementation
- Interface designs that caused friction
- Testing strategies that proved effective
- Library and framework behavioral gotchas
- Performance characteristics discovered under load

Without persistent memory, these insights exist only within a single session. When the session ends, the knowledge is lost. Agents cannot benefit from prior experience, leading to:
- Repeated mistakes across similar problems
- No institutional memory accumulation
- Inability to improve design quality over time

---

## 2. Solution Overview

The Engram system:
1. **Captures** structured lore during agent workflows
2. **Stores** lore locally for low-latency recall (<20ms)
3. **Searches** semantically using cosine similarity on embeddings
4. **Reinforces** quality through feedback loops
5. **Synchronizes** optionally to Engram for cross-environment persistence

---

## 3. Core Concepts

### 3.1 Lore

A discrete unit of experiential knowledge captured from agent activity.

| Attribute | Type | Description |
|-----------|------|-------------|
| id | ULID | Globally unique identifier |
| content | Text | The knowledge—what was learned (max 4000 chars) |
| context | Text | Where/when learned (max 1000 chars) |
| category | Enum | Classification of lore type |
| confidence | Float [0.0-1.0] | How validated this lore is |
| embedding | Vector | Semantic representation for similarity search |
| source_id | String | Which environment generated this lore |
| validation_count | Integer | Times marked helpful |
| created_at | Timestamp | When captured |
| updated_at | Timestamp | Last modification |

### 3.2 Lore Categories

| Category | Description | Example |
|----------|-------------|---------|
| `ARCHITECTURAL_DECISION` | System-level choices and rationale | "Chose event sourcing for audit requirements" |
| `PATTERN_OUTCOME` | Results of applying a design pattern | "Repository pattern added unnecessary abstraction" |
| `INTERFACE_LESSON` | Contract/API design insights | "Nullable returns caused null check proliferation" |
| `EDGE_CASE_DISCOVERY` | Scenarios found during implementation | "Empty collections require special handling" |
| `IMPLEMENTATION_FRICTION` | Design-to-code translation difficulties | "Interface was correct but needed async" |
| `TESTING_STRATEGY` | Testing approach insights | "Queue consumers need idempotency verification" |
| `DEPENDENCY_BEHAVIOR` | Library/framework gotchas | "ORM generates N+1 without eager loading" |
| `PERFORMANCE_INSIGHT` | Performance characteristics | "In-memory failed at 10k—streaming required" |

**Design note:** Categories are semantically distinct. Security learnings fold into existing categories (security edge cases → `EDGE_CASE_DISCOVERY`, secure patterns → `PATTERN_OUTCOME`).

### 3.3 Confidence Model

Confidence represents how validated lore is:

| Score | Meaning |
|-------|---------|
| 0.0–0.3 | Hypothesis, unvalidated |
| 0.3–0.6 | Some evidence, limited validation |
| 0.6–0.8 | Validated in multiple contexts |
| 0.8–1.0 | Repeatedly confirmed, high reliability |

**Feedback adjustments:**

| Feedback | Adjustment | Rationale |
|----------|------------|-----------|
| `helpful` | +0.08 | Lore was useful—reinforce |
| `not_relevant` | 0 | Context mismatch, not quality issue |
| `incorrect` | -0.15 | Bad lore is costly—asymmetric penalty |

**Boundaries:** Capped at 1.0, floored at 0.0.

---

## 4. Architecture

### 4.1 Component Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                           ENGRAM (Central)                           │
│  ┌───────────────┐  ┌───────────────┐  ┌─────────────────────────┐  │
│  │   API Layer   │  │   Embedding   │  │      Lore Store         │  │
│  │               │◀─│    Service    │◀─│   (SQLite + dedup)      │  │
│  │ - Ingest      │  │               │  │                         │  │
│  │ - Snapshot    │  │ - OpenAI      │  │ - Persistence           │  │
│  │ - Delta sync  │  │ - text-3-sm   │  │ - Conflict resolution   │  │
│  │ - Feedback    │  │               │  │ - Deduplication         │  │
│  └───────────────┘  └───────────────┘  └─────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────┘
                                    │
                     ═══════════════╪═══════════════
                          Network (HTTPS)
                     ═══════════════╪═══════════════
                                    │
        ┌───────────────────────────┼───────────────────────────┐
        ▼                           ▼                           ▼
┌───────────────────┐   ┌───────────────────┐   ┌───────────────────┐
│  Environment A    │   │  Environment B    │   │  Environment N    │
│  ┌─────────────┐  │   │  ┌─────────────┐  │   │  ┌─────────────┐  │
│  │   Recall    │  │   │  │   Recall    │  │   │  │   Recall    │  │
│  │   Client    │  │   │  │   Client    │  │   │  │   Client    │  │
│  ├─────────────┤  │   │  ├─────────────┤  │   │  ├─────────────┤  │
│  │   SQLite    │  │   │  │   SQLite    │  │   │  │   SQLite    │  │
│  │  (local)    │  │   │  │  (local)    │  │   │  │  (local)    │  │
│  └─────────────┘  │   │  └─────────────┘  │   │  └─────────────┘  │
│        │          │   │        │          │   │        │          │
│  ┌─────▼───────┐  │   │  ┌─────▼───────┐  │   │  ┌─────▼───────┐  │
│  │   Agents    │  │   │  │   Agents    │  │   │  │   Agents    │  │
│  └─────────────┘  │   │  └─────────────┘  │   │  └─────────────┘  │
└───────────────────┘   └───────────────────┘   └───────────────────┘
```

### 4.2 Component Responsibilities

| Component | Responsibility |
|-----------|----------------|
| **Engram API** | Authentication, request routing, rate limiting |
| **Embedding Service** | Convert text to vector representations (OpenAI) |
| **Engram Store** | Primary persistence, deduplication, conflict resolution |
| **Recall Client** | Agent-facing API, local storage, sync orchestration |
| **Local Store** | Low-latency queries, offline support, sync queue |

---

## 5. Data Model

### 5.1 Lore Record

```sql
lore_entries (
  id               TEXT PRIMARY KEY,  -- ULID
  content          TEXT NOT NULL,     -- max 4000 chars
  context          TEXT,              -- max 1000 chars
  category         TEXT NOT NULL,     -- enum
  confidence       REAL NOT NULL,     -- 0.0-1.0
  embedding        BLOB NOT NULL,     -- packed float32[]
  source_id        TEXT NOT NULL,
  sources          TEXT,              -- JSON array
  validation_count INTEGER DEFAULT 0,
  last_validated   TEXT,              -- ISO8601
  created_at       TEXT NOT NULL,     -- ISO8601
  updated_at       TEXT NOT NULL,     -- ISO8601
  synced_at        TEXT,              -- local only
  deleted_at       TEXT               -- soft delete
)

INDEXES:
  - category
  - confidence
  - created_at
  - synced_at
```

### 5.2 Metadata Record

```sql
metadata (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
)

REQUIRED KEYS:
  - schema_version
  - embedding_model
  - embedding_dimensions
  - last_sync (local only)
  - source_id (local only)
```

### 5.3 Sync Queue (Local Only)

```sql
sync_queue (
  id        INTEGER PRIMARY KEY,
  lore_id   TEXT NOT NULL,
  operation TEXT NOT NULL,  -- INSERT, FEEDBACK
  payload   TEXT,           -- JSON
  queued_at TEXT NOT NULL,
  attempts  INTEGER DEFAULT 0,
  last_error TEXT
)
```

---

## 6. Interface Contracts

### 6.1 Recall Client API

```go
// Core operations
func (c *Client) Record(content string, category Category, opts ...RecordOption) (*Lore, error)
func (c *Client) Query(ctx context.Context, params QueryParams) (*QueryResult, error)
func (c *Client) Feedback(ref string, ft FeedbackType) (*Lore, error)
func (c *Client) FeedbackBatch(ctx context.Context, params FeedbackParams) (*FeedbackResult, error)

// Sync operations
func (c *Client) Sync(ctx context.Context) error
func (c *Client) SyncPush(ctx context.Context) error
func (c *Client) Bootstrap(ctx context.Context) error

// Session
func (c *Client) GetSessionLore() []SessionLore
```

### 6.2 Engram API (Central Service)

```
POST /api/v1/lore           - Ingest lore batch
GET  /api/v1/lore/snapshot  - Download full database
GET  /api/v1/lore/delta     - Incremental changes
POST /api/v1/lore/feedback  - Submit feedback batch
GET  /api/v1/health         - Service health check
```

See [Engram API Specification](engram-api-specification.md) for full details.

---

## 7. Sync Protocol

### 7.1 Bootstrap (Full Sync)

```
Recall                              Engram
   │                                   │
   │  GET /health                      │
   │──────────────────────────────────▶│
   │  {embedding_model, version}       │
   │◀──────────────────────────────────│
   │                                   │
   │  [Validate model compatibility]   │
   │                                   │
   │  GET /lore/snapshot               │
   │──────────────────────────────────▶│
   │  <binary database>                │
   │◀──────────────────────────────────│
   │                                   │
   │  [Replace local store]            │
   │  [Set last_sync timestamp]        │
```

### 7.2 Incremental Sync

```
Recall                              Engram
   │                                   │
   │  POST /lore {pending lore}        │
   │──────────────────────────────────▶│
   │  {accepted, merged, rejected}     │
   │◀──────────────────────────────────│
   │                                   │
   │  POST /feedback {pending}         │
   │──────────────────────────────────▶│
   │  {updates}                        │
   │◀──────────────────────────────────│
   │                                   │
   │  GET /delta?since={last_sync}     │
   │──────────────────────────────────▶│
   │  {lore[], deleted_ids[]}          │
   │◀──────────────────────────────────│
   │                                   │
   │  [Merge into local store]         │
   │  [Update last_sync]               │
```

### 7.3 Conflict Resolution

When semantically equivalent lore arrives from different sources:
- Cosine similarity >= 0.92 within same category triggers merge
- Existing lore confidence boosted by 0.10
- Context appended with separator
- All source IDs tracked in `sources` array

---

## 8. Recall Algorithm

### 8.1 Local Search

```go
func (c *Client) Query(ctx context.Context, params QueryParams) (*QueryResult, error) {
    // 1. Fetch candidates matching filters (category, min_confidence)
    candidates := store.QueryWithFilters(params)

    // 2. Compute cosine similarity against query embedding
    scored := similarity.Search(queryEmbedding, candidates, params.K)

    // 3. Track in session for feedback
    for i, lore := range scored {
        session.Track(lore.ID)  // Assigns L1, L2, L3...
    }

    return &QueryResult{Lore: scored, SessionRefs: refs}
}
```

### 8.2 Performance Tiers

| Lore Count | Strategy | Expected Latency |
|------------|----------|------------------|
| < 1,000 | Brute-force scan | < 5ms |
| 1,000–5,000 | Brute-force with early termination | 5–20ms |
| 5,000–20,000 | HNSW index (future) | < 10ms |
| > 20,000 | Dedicated vector DB (future) | < 5ms |

Current implementation uses brute-force, which is sufficient for MVP scale.

---

## 9. Embedding Strategy

**Decision: Server-side embedding at Engram.**

Rationale:
- Guarantees all embeddings use identical model
- Simplifies client implementation
- Centralizes API key management
- Offline recall works (embeddings already stored locally)
- Only new lore needs network (not queries)

**Model:** OpenAI `text-embedding-3-small` (1536 dimensions)

**Compatibility check:** Client validates embedding model on bootstrap. Mismatch triggers warning and requires re-bootstrap.

---

## 10. Technology Stack

### Recall (Client Library)

| Component | Technology | Rationale |
|-----------|------------|-----------|
| Language | Go | Wide adoption, easy distribution |
| Database | modernc.org/sqlite | Pure Go, no CGO |
| IDs | oklog/ulid/v2 | Sortable, unique |
| CLI | spf13/cobra | Standard Go CLI framework |
| MCP | mark3labs/mcp-go | MCP protocol support |

### Engram (Central Service)

| Component | Technology | Rationale |
|-----------|------------|-----------|
| Language | Go | Consistency with ecosystem |
| Framework | Chi | Lightweight, idiomatic |
| Database | SQLite | Simple, sufficient scale |
| Embeddings | OpenAI text-embedding-3-small | Quality/cost ratio |
| Hosting | Fly.io | Persistent volumes, simple deploy |

---

## 11. Implementation Status

### Completed (MVP)

- [x] Core client: Record, Query, Feedback
- [x] Local SQLite store with migrations
- [x] Session tracking with L-references
- [x] Confidence model and feedback loop
- [x] Sync queue with retry logic
- [x] Bootstrap sync (full snapshot)
- [x] Push sync (lore and feedback)
- [x] CLI tool (all commands)
- [x] MCP server integration
- [x] JSON output mode
- [x] Comprehensive test coverage

### Planned (Post-MVP)

- [ ] Delta sync (incremental pull)
- [ ] HNSW indexing for >5,000 entries
- [ ] Confidence decay for stale lore
- [ ] Passive injection helpers
- [ ] Framework-specific integration packages

---

## 12. Capacity Targets

### Storage

| Component | Per Entry | 10,000 Entries |
|-----------|-----------|----------------|
| Content | ~500 B | ~5 MB |
| Context | ~200 B | ~2 MB |
| Embedding | ~6 KB | ~60 MB |
| Metadata | ~200 B | ~2 MB |
| **Total** | ~7 KB | ~70 MB |

### Throughput

| Operation | Target |
|-----------|--------|
| Local query | < 20ms |
| Record | < 100ms |
| Sync push | < 2s |
| Bootstrap | < 10s |

---

## 13. Security

### Authentication

- Client → Engram: Bearer token (API key per organization)
- Engram → OpenAI: Server-side only (keys never reach clients)

### Data Protection

- HTTPS required for all Engram communication
- API keys never logged or included in errors
- Local SQLite should be protected like credential stores

### Input Validation

| Field | Constraint |
|-------|------------|
| content | 1–4000 chars, valid UTF-8 |
| context | 0–1000 chars, valid UTF-8 |
| category | Valid enum value |
| confidence | 0.0–1.0 |

---

## Appendix: Test Scenarios

### Recording

```
GIVEN agent completes technical design
WHEN design includes pattern decision
THEN lore recorded with PATTERN_OUTCOME

GIVEN content exceeds 4000 chars
WHEN record attempted
THEN validation error returned

GIVEN lore recorded locally
WHEN sync succeeds
THEN synced_at timestamp set
```

### Querying

```
GIVEN lore about "queue consumer patterns"
WHEN querying "implementing message consumers"
THEN semantically similar lore returned

GIVEN no matching lore
WHEN query executed
THEN empty list returned (not error)

GIVEN lore recalled in session
THEN session refs (L1, L2) assigned
```

### Feedback

```
GIVEN L1, L2 recalled this session
WHEN feedback(helpful: ["L1", "L2"])
THEN confidence +0.08 each
AND validation_count incremented
AND last_validated updated

GIVEN lore at confidence 0.95
WHEN marked helpful
THEN confidence capped at 1.0
```

### Sync

```
GIVEN new devcontainer boots
WHEN sync initialized
THEN full snapshot downloaded

GIVEN pending lore locally
WHEN sync push triggered
THEN lore sent to Engram and marked synced
```
