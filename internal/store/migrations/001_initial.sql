-- +goose Up
CREATE TABLE IF NOT EXISTS lore_entries (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    context TEXT,
    category TEXT NOT NULL,
    confidence REAL NOT NULL DEFAULT 0.5,
    embedding BLOB,
    embedding_status TEXT NOT NULL DEFAULT 'complete',
    source_id TEXT NOT NULL,
    sources TEXT NOT NULL DEFAULT '[]',
    validation_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT,
    last_validated_at TEXT,
    synced_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_lore_entries_category ON lore_entries(category);
CREATE INDEX IF NOT EXISTS idx_lore_entries_confidence ON lore_entries(confidence);
CREATE INDEX IF NOT EXISTS idx_lore_entries_created_at ON lore_entries(created_at);
CREATE INDEX IF NOT EXISTS idx_lore_entries_synced_at ON lore_entries(synced_at);
CREATE INDEX IF NOT EXISTS idx_lore_entries_last_validated_at ON lore_entries(last_validated_at);
CREATE INDEX IF NOT EXISTS idx_lore_entries_deleted_at ON lore_entries(deleted_at);

CREATE TABLE IF NOT EXISTS metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sync_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    lore_id TEXT NOT NULL,
    operation TEXT NOT NULL,
    payload TEXT,
    queued_at TEXT NOT NULL,
    attempts INTEGER DEFAULT 0,
    last_error TEXT
);

CREATE INDEX IF NOT EXISTS idx_sync_queue_queued_at ON sync_queue(queued_at);

-- +goose Down
DROP INDEX IF EXISTS idx_sync_queue_queued_at;
DROP TABLE IF EXISTS sync_queue;
DROP TABLE IF EXISTS metadata;
DROP INDEX IF EXISTS idx_lore_entries_deleted_at;
DROP INDEX IF EXISTS idx_lore_entries_last_validated_at;
DROP INDEX IF EXISTS idx_lore_entries_synced_at;
DROP INDEX IF EXISTS idx_lore_entries_created_at;
DROP INDEX IF EXISTS idx_lore_entries_confidence;
DROP INDEX IF EXISTS idx_lore_entries_category;
DROP TABLE IF EXISTS lore_entries;
