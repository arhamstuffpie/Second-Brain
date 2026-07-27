-- +goose Up
CREATE TABLE video_realtime_sessions (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    memory_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    device_id TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    chunk_duration_seconds INTEGER NOT NULL DEFAULT 30
        CHECK (chunk_duration_seconds BETWEEN 5 AND 300),
    frame_interval_seconds INTEGER NOT NULL DEFAULT 5
        CHECK (frame_interval_seconds BETWEEN 1 AND 60),
    next_chunk_index INTEGER NOT NULL DEFAULT 0 CHECK (next_chunk_index >= 0),
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'stopped')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    stopped_at TIMESTAMPTZ
);

CREATE INDEX video_realtime_sessions_owner_created_idx
    ON video_realtime_sessions(owner_user_id, created_at DESC);

CREATE TABLE video_recordings (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    realtime_session_id TEXT REFERENCES video_realtime_sessions(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    memory_id TEXT NOT NULL,
    device_id TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    client_chunk_id TEXT,
    chunk_index INTEGER CHECK (chunk_index >= 0),
    is_final BOOLEAN NOT NULL DEFAULT FALSE,
    file_name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    media_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    start_offset_seconds DOUBLE PRECISION NOT NULL DEFAULT 0
        CHECK (start_offset_seconds >= 0),
    frame_interval_seconds DOUBLE PRECISION NOT NULL DEFAULT 5
        CHECK (frame_interval_seconds > 0),
    default_confidence DOUBLE PRECISION,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'processing', 'merging', 'memograph_pending', 'completed', 'failed')),
    audio_status TEXT NOT NULL DEFAULT 'queued'
        CHECK (audio_status IN ('queued', 'processing', 'completed', 'failed')),
    visual_status TEXT NOT NULL DEFAULT 'queued'
        CHECK (visual_status IN ('queued', 'processing', 'completed', 'failed')),
    merge_status TEXT NOT NULL DEFAULT 'waiting'
        CHECK (merge_status IN ('waiting', 'queued', 'processing', 'completed', 'failed')),
    transcript JSONB,
    visual_analysis JSONB,
    stt_provider TEXT NOT NULL DEFAULT '',
    stt_model TEXT NOT NULL DEFAULT '',
    visual_provider TEXT NOT NULL DEFAULT '',
    visual_model TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX video_recordings_owner_created_idx
    ON video_recordings(owner_user_id, created_at DESC);
CREATE INDEX video_recordings_session_idx
    ON video_recordings(owner_user_id, session_id, chunk_index);
CREATE UNIQUE INDEX video_recordings_client_chunk_unique
    ON video_recordings(owner_user_id, session_id, client_chunk_id)
    WHERE client_chunk_id IS NOT NULL;
CREATE UNIQUE INDEX video_recordings_realtime_index_unique
    ON video_recordings(owner_user_id, realtime_session_id, chunk_index)
    WHERE realtime_session_id IS NOT NULL;

CREATE TABLE video_episodes (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    recording_id TEXT NOT NULL REFERENCES video_recordings(id) ON DELETE CASCADE,
    bucket_index INTEGER NOT NULL CHECK (bucket_index >= 0),
    start_time DOUBLE PRECISION NOT NULL CHECK (start_time >= 0),
    end_time DOUBLE PRECISION NOT NULL CHECK (end_time >= start_time),
    description TEXT NOT NULL,
    location TEXT NOT NULL DEFAULT '',
    confidence DOUBLE PRECISION,
    visual_observations JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'writing', 'completed', 'failed')),
    memograph_response JSONB,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT video_episodes_recording_bucket_unique UNIQUE (recording_id, bucket_index)
);

CREATE TABLE video_jobs (
    id BIGSERIAL PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('audio', 'visual', 'merge', 'memograph')),
    recording_id TEXT REFERENCES video_recordings(id) ON DELETE CASCADE,
    episode_id TEXT REFERENCES video_episodes(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'processing', 'completed', 'dead')),
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL CHECK (max_attempts > 0),
    run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT video_jobs_target_check CHECK (
        (kind IN ('audio', 'visual', 'merge') AND recording_id IS NOT NULL AND episode_id IS NULL) OR
        (kind = 'memograph' AND recording_id IS NULL AND episode_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX video_jobs_recording_kind_unique
    ON video_jobs(recording_id, kind)
    WHERE recording_id IS NOT NULL;
CREATE UNIQUE INDEX video_jobs_memograph_unique
    ON video_jobs(episode_id)
    WHERE kind = 'memograph';
CREATE INDEX video_jobs_claim_idx ON video_jobs(status, run_at, id);

-- +goose Down
DROP TABLE video_jobs;
DROP TABLE video_episodes;
DROP TABLE video_recordings;
DROP TABLE video_realtime_sessions;
