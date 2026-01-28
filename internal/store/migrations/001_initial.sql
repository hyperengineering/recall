-- +goose Up
CREATE TABLE IF NOT EXISTS lore (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    context TEXT,
    category TEXT NOT NULL,
    confidence REAL NOT NULL DEFAULT 0.7,
    embedding BLOB,
    source_id TEXT NOT NULL,
    sources TEXT,
    validation_count INTEGER NOT NULL DEFAULT 0,
    last_validated TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    synced_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_lore_category ON lore(category);
CREATE INDEX IF NOT EXISTS idx_lore_confidence ON lore(confidence);
CREATE INDEX IF NOT EXISTS idx_lore_created_at ON lore(created_at);
CREATE INDEX IF NOT EXISTS idx_lore_synced_at ON lore(synced_at);
CREATE INDEX IF NOT EXISTS idx_lore_last_validated ON lore(last_validated);

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
DROP INDEX IF EXISTS idx_lore_last_validated;
DROP INDEX IF EXISTS idx_lore_synced_at;
DROP INDEX IF EXISTS idx_lore_created_at;
DROP INDEX IF EXISTS idx_lore_confidence;
DROP INDEX IF EXISTS idx_lore_category;
DROP TABLE IF EXISTS lore;
