-- +goose Up
CREATE TABLE voice_recordings (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    memory_id TEXT NOT NULL,
    device_id TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    file_name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    media_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    start_offset_seconds DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (start_offset_seconds >= 0),
    default_confidence DOUBLE PRECISION,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'transcribing', 'memograph_pending', 'completed', 'failed')),
    transcript JSONB,
    stt_provider TEXT NOT NULL DEFAULT '',
    stt_model TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX voice_recordings_owner_created_idx
    ON voice_recordings(owner_user_id, created_at DESC);
CREATE INDEX voice_recordings_session_idx
    ON voice_recordings(session_id);

CREATE TABLE voice_episodes (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    recording_id TEXT NOT NULL REFERENCES voice_recordings(id) ON DELETE CASCADE,
    bucket_index INTEGER NOT NULL CHECK (bucket_index >= 0),
    start_time DOUBLE PRECISION NOT NULL CHECK (start_time >= 0),
    end_time DOUBLE PRECISION NOT NULL CHECK (end_time >= start_time),
    description TEXT NOT NULL,
    confidence DOUBLE PRECISION,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'writing', 'completed', 'failed')),
    memograph_response JSONB,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT voice_episodes_recording_bucket_unique UNIQUE (recording_id, bucket_index)
);

CREATE TABLE voice_jobs (
    id BIGSERIAL PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('stt', 'memograph')),
    recording_id TEXT REFERENCES voice_recordings(id) ON DELETE CASCADE,
    episode_id TEXT REFERENCES voice_episodes(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'processing', 'completed', 'dead')),
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL CHECK (max_attempts > 0),
    run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT voice_jobs_target_check CHECK (
        (kind = 'stt' AND recording_id IS NOT NULL AND episode_id IS NULL) OR
        (kind = 'memograph' AND recording_id IS NULL AND episode_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX voice_jobs_stt_unique
    ON voice_jobs(recording_id) WHERE kind = 'stt';
CREATE UNIQUE INDEX voice_jobs_memograph_unique
    ON voice_jobs(episode_id) WHERE kind = 'memograph';
CREATE INDEX voice_jobs_claim_idx ON voice_jobs(status, run_at, id);

-- +goose Down
DROP TABLE voice_jobs;
DROP TABLE voice_episodes;
DROP TABLE voice_recordings;
