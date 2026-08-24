-- +goose Up
CREATE TABLE media_assets (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL DEFAULT '', chunk_id TEXT, source_kind TEXT NOT NULL,
    storage_provider TEXT NOT NULL, bucket TEXT NOT NULL DEFAULT '', object_key TEXT NOT NULL,
    file_name TEXT NOT NULL, media_type TEXT NOT NULL, size_bytes BIGINT NOT NULL CHECK (size_bytes > 0), sha256 TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'durable' CHECK (status IN ('uploading','verifying','durable','processing','completed','quarantined','deleting','deleted','failed')),
    retention_class TEXT NOT NULL DEFAULT 'standard', retention_expires_at TIMESTAMPTZ, archive_status TEXT NOT NULL DEFAULT 'available',
    capture_started_at TIMESTAMPTZ, actual_duration_seconds DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX media_assets_provider_object_key_idx ON media_assets(storage_provider, bucket, object_key);
ALTER TABLE voice_recordings ADD COLUMN media_asset_id TEXT REFERENCES media_assets(id);
ALTER TABLE video_recordings ADD COLUMN media_asset_id TEXT REFERENCES media_assets(id);
-- +goose Down
ALTER TABLE video_recordings DROP COLUMN media_asset_id;
ALTER TABLE voice_recordings DROP COLUMN media_asset_id;
DROP TABLE media_assets;
