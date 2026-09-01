-- +goose Up
CREATE TABLE voice_realtime_sessions (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    memory_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    device_id TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    chunk_duration_seconds INTEGER NOT NULL DEFAULT 30
        CHECK (chunk_duration_seconds BETWEEN 5 AND 300),
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'stopped')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    stopped_at TIMESTAMPTZ
);

CREATE INDEX voice_realtime_sessions_owner_created_idx
    ON voice_realtime_sessions(owner_user_id, created_at DESC);

ALTER TABLE voice_recordings
    ADD COLUMN chunk_index INTEGER CHECK (chunk_index >= 0),
    ADD COLUMN is_final BOOLEAN NOT NULL DEFAULT FALSE;

CREATE UNIQUE INDEX voice_recordings_realtime_chunk_unique
    ON voice_recordings(owner_user_id, session_id, chunk_index)
    WHERE chunk_index IS NOT NULL;

CREATE INDEX voice_recordings_realtime_session_idx
    ON voice_recordings(owner_user_id, session_id, chunk_index)
    WHERE chunk_index IS NOT NULL;

-- +goose Down
DROP INDEX voice_recordings_realtime_session_idx;
DROP INDEX voice_recordings_realtime_chunk_unique;
ALTER TABLE voice_recordings
    DROP COLUMN is_final,
    DROP COLUMN chunk_index;
DROP TABLE voice_realtime_sessions;
