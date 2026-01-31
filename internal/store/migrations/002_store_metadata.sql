-- +goose Up
-- Add store metadata fields

-- Store description (human-readable)
INSERT OR IGNORE INTO metadata (key, value) VALUES ('description', '');

-- Store creation timestamp (set on first migration run)
INSERT OR IGNORE INTO metadata (key, value) VALUES ('created_at', datetime('now'));

-- Migration source path (if migrated from existing DB)
-- Value is empty for new stores, original path for migrated stores
INSERT OR IGNORE INTO metadata (key, value) VALUES ('migrated_from', '');

-- +goose Down
DELETE FROM metadata WHERE key IN ('description', 'created_at', 'migrated_from');
