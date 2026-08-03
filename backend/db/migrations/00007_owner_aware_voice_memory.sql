-- +goose Up
CREATE TABLE voice_enrollment_samples (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    slot SMALLINT NOT NULL CHECK (slot BETWEEN 0 AND 3),
    provider_label TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    media_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    duration_seconds DOUBLE PRECISION NOT NULL
        CHECK (duration_seconds BETWEEN 2 AND 10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT voice_enrollment_samples_owner_slot_unique
        UNIQUE (owner_user_id, slot),
    CONSTRAINT voice_enrollment_samples_owner_label_unique
        UNIQUE (owner_user_id, provider_label)
);

CREATE INDEX voice_enrollment_samples_owner_created_idx
    ON voice_enrollment_samples(owner_user_id, created_at);

CREATE TABLE voice_episode_batches (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    memory_id TEXT NOT NULL,
    realtime_session_id TEXT REFERENCES voice_realtime_sessions(id) ON DELETE CASCADE,
    closed BOOLEAN NOT NULL DEFAULT FALSE,
    transcript_revision BIGINT NOT NULL DEFAULT 0,
    assembled_revision BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX voice_episode_batches_owner_session_idx
    ON voice_episode_batches(owner_user_id, session_id, created_at DESC);

ALTER TABLE voice_recordings
    ADD COLUMN batch_id TEXT REFERENCES voice_episode_batches(id) ON DELETE CASCADE,
    ADD COLUMN speaker_reference_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

INSERT INTO voice_episode_batches (
    id, owner_user_id, session_id, group_id, memory_id, closed, created_at, updated_at
)
SELECT id, owner_user_id, session_id, group_id, memory_id,
       TRUE, created_at, updated_at
FROM voice_recordings;

UPDATE voice_recordings SET batch_id = id;

ALTER TABLE voice_recordings
    ALTER COLUMN batch_id SET NOT NULL;

CREATE INDEX voice_recordings_batch_timeline_idx
    ON voice_recordings(batch_id, start_offset_seconds, chunk_index);

ALTER TABLE voice_realtime_sessions
    ADD COLUMN batch_id TEXT REFERENCES voice_episode_batches(id) ON DELETE CASCADE;

ALTER TABLE voice_episodes
    ADD COLUMN batch_id TEXT REFERENCES voice_episode_batches(id) ON DELETE CASCADE,
    ADD COLUMN episode_index INTEGER,
    ADD COLUMN segments JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN source_recording_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN owner_utterance_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN other_utterance_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN unknown_utterance_count INTEGER NOT NULL DEFAULT 0;

UPDATE voice_episodes e
SET batch_id = r.batch_id,
    episode_index = e.bucket_index,
    source_recording_ids = jsonb_build_array(e.recording_id)
FROM voice_recordings r
WHERE r.id = e.recording_id;

ALTER TABLE voice_episodes
    ALTER COLUMN batch_id SET NOT NULL,
    ALTER COLUMN episode_index SET NOT NULL,
    ADD CONSTRAINT voice_episodes_episode_index_check CHECK (episode_index >= 0),
    ADD CONSTRAINT voice_episodes_attribution_counts_check CHECK (
        owner_utterance_count >= 0 AND
        other_utterance_count >= 0 AND
        unknown_utterance_count >= 0
    );

CREATE UNIQUE INDEX voice_episodes_batch_episode_unique
    ON voice_episodes(batch_id, episode_index);

CREATE TABLE voice_episode_recordings (
    episode_id TEXT NOT NULL REFERENCES voice_episodes(id) ON DELETE CASCADE,
    recording_id TEXT NOT NULL REFERENCES voice_recordings(id) ON DELETE CASCADE,
    PRIMARY KEY (episode_id, recording_id)
);

INSERT INTO voice_episode_recordings (episode_id, recording_id)
SELECT id, recording_id FROM voice_episodes;

CREATE INDEX voice_episode_recordings_recording_idx
    ON voice_episode_recordings(recording_id, episode_id);

ALTER TABLE voice_jobs
    DROP CONSTRAINT voice_jobs_kind_check,
    DROP CONSTRAINT voice_jobs_target_check,
    ADD COLUMN batch_id TEXT REFERENCES voice_episode_batches(id) ON DELETE CASCADE,
    ADD CONSTRAINT voice_jobs_kind_check CHECK (kind IN ('stt', 'assemble', 'memograph')),
    ADD CONSTRAINT voice_jobs_target_check CHECK (
        (kind = 'stt' AND recording_id IS NOT NULL AND episode_id IS NULL AND batch_id IS NULL) OR
        (kind = 'assemble' AND recording_id IS NULL AND episode_id IS NULL AND batch_id IS NOT NULL) OR
        (kind = 'memograph' AND recording_id IS NULL AND episode_id IS NOT NULL AND batch_id IS NULL)
    );

CREATE UNIQUE INDEX voice_jobs_assemble_unique
    ON voice_jobs(batch_id) WHERE kind = 'assemble';

ALTER TABLE voice_recordings
    DROP CONSTRAINT voice_recordings_status_check,
    ADD CONSTRAINT voice_recordings_status_check CHECK (
        status IN ('queued', 'transcribing', 'assembling', 'memograph_pending', 'completed', 'failed')
    );

-- +goose Down
UPDATE voice_recordings
SET status = 'memograph_pending'
WHERE status = 'assembling';

DELETE FROM voice_jobs WHERE kind = 'assemble';

ALTER TABLE voice_recordings
    DROP CONSTRAINT voice_recordings_status_check,
    ADD CONSTRAINT voice_recordings_status_check CHECK (
        status IN ('queued', 'transcribing', 'memograph_pending', 'completed', 'failed')
    );

DROP INDEX voice_jobs_assemble_unique;
ALTER TABLE voice_jobs
    DROP CONSTRAINT voice_jobs_target_check,
    DROP CONSTRAINT voice_jobs_kind_check,
    DROP COLUMN batch_id,
    ADD CONSTRAINT voice_jobs_kind_check CHECK (kind IN ('stt', 'memograph')),
    ADD CONSTRAINT voice_jobs_target_check CHECK (
        (kind = 'stt' AND recording_id IS NOT NULL AND episode_id IS NULL) OR
        (kind = 'memograph' AND recording_id IS NULL AND episode_id IS NOT NULL)
    );

DROP INDEX voice_episode_recordings_recording_idx;
DROP TABLE voice_episode_recordings;
DROP INDEX voice_episodes_batch_episode_unique;
ALTER TABLE voice_episodes
    DROP CONSTRAINT voice_episodes_attribution_counts_check,
    DROP CONSTRAINT voice_episodes_episode_index_check,
    DROP COLUMN unknown_utterance_count,
    DROP COLUMN other_utterance_count,
    DROP COLUMN owner_utterance_count,
    DROP COLUMN source_recording_ids,
    DROP COLUMN segments,
    DROP COLUMN episode_index,
    DROP COLUMN batch_id;

ALTER TABLE voice_realtime_sessions DROP COLUMN batch_id;
DROP INDEX voice_recordings_batch_timeline_idx;
ALTER TABLE voice_recordings
    DROP COLUMN speaker_reference_ids,
    DROP COLUMN batch_id;
DROP TABLE voice_episode_batches;
DROP TABLE voice_enrollment_samples;
