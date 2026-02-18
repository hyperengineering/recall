-- +goose Up
-- Story 9.1: Change Log & Sync Meta tables for universal sync protocol

-- Change log: tracks all mutations for sync protocol
CREATE TABLE IF NOT EXISTS change_log (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    table_name TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    operation TEXT NOT NULL CHECK (operation IN ('upsert', 'delete')),
    payload TEXT,
    source_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    received_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- sequence is INTEGER PRIMARY KEY → already indexed by SQLite (rowid alias)
CREATE INDEX IF NOT EXISTS idx_change_log_table_entity ON change_log(table_name, entity_id);

-- Push idempotency: prevents duplicate push processing
CREATE TABLE IF NOT EXISTS push_idempotency (
    push_id TEXT PRIMARY KEY,
    store_id TEXT NOT NULL,
    response TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_push_idempotency_expires_at ON push_idempotency(expires_at);

-- Sync meta: key-value store for sync protocol state
CREATE TABLE IF NOT EXISTS sync_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Server-level sync_meta initial values
INSERT OR IGNORE INTO sync_meta (key, value) VALUES ('schema_version', '2');
INSERT OR IGNORE INTO sync_meta (key, value) VALUES ('last_compaction_seq', '0');
INSERT OR IGNORE INTO sync_meta (key, value) VALUES ('last_compaction_at', '');

-- +goose Down
DROP INDEX IF EXISTS idx_push_idempotency_expires_at;
DROP INDEX IF EXISTS idx_change_log_table_entity;
DROP TABLE IF EXISTS sync_meta;
DROP TABLE IF EXISTS push_idempotency;
DROP TABLE IF EXISTS change_log;
