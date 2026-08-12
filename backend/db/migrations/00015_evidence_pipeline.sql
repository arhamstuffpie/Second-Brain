-- +goose Up
ALTER TABLE video_recordings
    ADD COLUMN processing_version INTEGER NOT NULL DEFAULT 2 CHECK (processing_version > 0);
ALTER TABLE video_jobs DROP CONSTRAINT video_jobs_source_check;
ALTER TABLE video_jobs ADD CONSTRAINT video_jobs_source_check CHECK (
    (kind = 'memograph' AND source IN (
        'visual','speech','legacy','visual_evidence','speech_evidence','context_summary'
    )) OR (kind <> 'memograph' AND source = '')
);

CREATE TABLE video_analysis_batches (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    recording_id TEXT NOT NULL REFERENCES video_recordings(id) ON DELETE CASCADE,
    batch_index INTEGER NOT NULL CHECK (batch_index >= 0),
    start_time DOUBLE PRECISION NOT NULL CHECK (start_time >= 0),
    end_time DOUBLE PRECISION NOT NULL CHECK (end_time >= start_time),
    frames JSONB NOT NULL CHECK (jsonb_array_length(frames) BETWEEN 1 AND 8),
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','processing','completed','dead')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    result JSONB,
    last_error TEXT NOT NULL DEFAULT '',
    processing_version INTEGER NOT NULL CHECK (processing_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (recording_id, batch_index, processing_version)
);
CREATE INDEX video_analysis_batches_claim_idx
    ON video_analysis_batches(recording_id, processing_version, status, batch_index);

ALTER TABLE video_episodes
    ADD COLUMN evidence_kind TEXT NOT NULL DEFAULT 'context_summary'
        CHECK (evidence_kind IN ('visual_evidence','speech_evidence','context_summary')),
    ADD COLUMN source_identity TEXT NOT NULL DEFAULT '',
    ADD COLUMN processing_version INTEGER NOT NULL DEFAULT 2 CHECK (processing_version > 0),
    ADD COLUMN media_asset_id TEXT REFERENCES media_assets(id),
    ADD COLUMN observation_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN frame_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN supporting_episode_ids JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE video_episodes DROP CONSTRAINT video_episodes_recording_bucket_unique;
UPDATE video_episodes SET source_identity = 'legacy:' || id WHERE source_identity = '';
CREATE UNIQUE INDEX video_episodes_evidence_identity_idx
    ON video_episodes(recording_id, evidence_kind, source_identity, processing_version);

ALTER TABLE voice_episodes
    ADD COLUMN evidence_kind TEXT NOT NULL DEFAULT 'speech_evidence'
        CHECK (evidence_kind IN ('speech_evidence','context_summary')),
    ADD COLUMN source_identity TEXT NOT NULL DEFAULT '',
    ADD COLUMN processing_version INTEGER NOT NULL DEFAULT 2 CHECK (processing_version > 0),
    ADD COLUMN media_asset_ids JSONB NOT NULL DEFAULT '[]'::jsonb;
DROP INDEX voice_episodes_batch_episode_unique;
UPDATE voice_episodes SET source_identity = 'legacy:' || id WHERE source_identity = '';
CREATE UNIQUE INDEX voice_episodes_evidence_identity_idx
    ON voice_episodes(batch_id, evidence_kind, source_identity, processing_version);

-- +goose Down
ALTER TABLE voice_episodes
    DROP COLUMN media_asset_ids,
    DROP COLUMN processing_version,
    DROP COLUMN source_identity,
    DROP COLUMN evidence_kind;
CREATE UNIQUE INDEX voice_episodes_batch_episode_unique
    ON voice_episodes(batch_id, episode_index);
ALTER TABLE video_episodes
    DROP COLUMN supporting_episode_ids,
    DROP COLUMN frame_ids,
    DROP COLUMN observation_ids,
    DROP COLUMN media_asset_id,
    DROP COLUMN processing_version,
    DROP COLUMN source_identity,
    DROP COLUMN evidence_kind;
ALTER TABLE video_episodes
    ADD CONSTRAINT video _episodes_recording_bucket_unique UNIQUE (recording_id, bucket_index);
DROP TABLE video_analysis_batches;
ALTER TABLE video_recordings DROP COLUMN processing_version;
ALTER TABLE video_jobs DROP CONSTRAINT video_jobs_source_check;
ALTER TABLE video_jobs ADD CONSTRAINT video_jobs_source_check CHECK (
    (kind = 'memograph' AND source IN ('visual','speech','legacy')) OR
    (kind <> 'memograph' AND source = '')
);
