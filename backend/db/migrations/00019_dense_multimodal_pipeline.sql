-- +goose Up
ALTER TABLE video_recordings
    ALTER COLUMN processing_version SET DEFAULT 3,
    ADD COLUMN requested_processing_version INTEGER NOT NULL DEFAULT 3
        CHECK (requested_processing_version > 0);
ALTER TABLE video_episodes ALTER COLUMN processing_version SET DEFAULT 3;
ALTER TABLE voice_episodes ALTER COLUMN processing_version SET DEFAULT 3;

CREATE TABLE analysis_runs (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recording_id TEXT NOT NULL,
    processing_version INTEGER NOT NULL CHECK (processing_version > 0),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','processing','completed','retryable_failed','dead')),
    is_active BOOLEAN NOT NULL DEFAULT FALSE,
    configuration_profile TEXT NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (recording_id, processing_version),
    FOREIGN KEY (owner_user_id, recording_id)
        REFERENCES video_recordings(owner_user_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX analysis_runs_one_active_idx
    ON analysis_runs(recording_id) WHERE is_active;
CREATE INDEX analysis_runs_claim_idx
    ON analysis_runs(status, updated_at, created_at);

CREATE TABLE analysis_stage_jobs (
    id BIGSERIAL PRIMARY KEY,
    analysis_run_id TEXT NOT NULL REFERENCES analysis_runs(id) ON DELETE CASCADE,
    stage TEXT NOT NULL CHECK (stage IN (
        'media_normalization','audio_analysis','dense_person_tracking','transcription',
        'identity_matching','active_speaker_fusion','episode_generation','graph_persistence'
    )),
    required BOOLEAN NOT NULL DEFAULT TRUE,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','processing','completed','retryable_failed','dead')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL CHECK (max_attempts > 0),
    run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    checkpoint JSONB NOT NULL DEFAULT '{}'::jsonb,
    result_provenance JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (analysis_run_id, stage)
);
CREATE INDEX analysis_stage_jobs_claim_idx
    ON analysis_stage_jobs(stage, status, run_at, id);

ALTER TABLE person_tracks
    ADD COLUMN lifecycle_status TEXT NOT NULL DEFAULT 'confirmed'
        CHECK (lifecycle_status IN ('tentative','confirmed','lost','ended')),
    ADD COLUMN first_frame INTEGER CHECK (first_frame IS NULL OR first_frame >= 0),
    ADD COLUMN last_frame INTEGER CHECK (last_frame IS NULL OR last_frame >= first_frame),
    ADD COLUMN observation_count INTEGER NOT NULL DEFAULT 0 CHECK (observation_count >= 0),
    ADD COLUMN quality_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN model_provenance JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE face_track_observations (
    id TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recording_id TEXT NOT NULL,
    person_track_id TEXT NOT NULL,
    processing_version INTEGER NOT NULL CHECK (processing_version > 0),
    frame_index INTEGER NOT NULL CHECK (frame_index >= 0),
    observed_at_seconds DOUBLE PRECISION NOT NULL CHECK (observed_at_seconds >= 0),
    face_box JSONB NOT NULL,
    landmarks JSONB NOT NULL DEFAULT '[]'::jsonb,
    detection_score DOUBLE PRECISION NOT NULL CHECK (detection_score BETWEEN 0 AND 1),
    quality JSONB NOT NULL DEFAULT '{}'::jsonb,
    pose JSONB NOT NULL DEFAULT '{}'::jsonb,
    embedding_reference TEXT,
    embedding_model TEXT NOT NULL,
    mouth_visible BOOLEAN NOT NULL DEFAULT FALSE,
    mouth_activity DOUBLE PRECISION CHECK (mouth_activity BETWEEN 0 AND 1),
    gallery_selected BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (person_track_id, frame_index, processing_version),
    FOREIGN KEY (owner_user_id, recording_id, person_track_id, processing_version)
        REFERENCES person_tracks(owner_user_id, recording_id, id, processing_version)
        ON DELETE CASCADE
);
CREATE INDEX face_track_observations_timeline_idx
    ON face_track_observations(owner_user_id, recording_id, observed_at_seconds, person_track_id);
CREATE INDEX face_track_observations_gallery_idx
    ON face_track_observations(owner_user_id, person_track_id, gallery_selected, observed_at_seconds)
    WHERE gallery_selected;

CREATE TABLE audio_regions (
    id TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recording_id TEXT NOT NULL,
    processing_version INTEGER NOT NULL CHECK (processing_version > 0),
    start_time DOUBLE PRECISION NOT NULL CHECK (start_time >= 0),
    end_time DOUBLE PRECISION NOT NULL CHECK (end_time > start_time),
    kind TEXT NOT NULL CHECK (kind IN ('silence','speech','overlap')),
    active_speaker_labels JSONB NOT NULL DEFAULT '[]'::jsonb,
    concurrent_speaker_count INTEGER NOT NULL CHECK (concurrent_speaker_count >= 0),
    overlap BOOLEAN NOT NULL,
    diarization_confidence DOUBLE PRECISION CHECK (diarization_confidence BETWEEN 0 AND 1),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','processing','completed','retryable_failed','dead','budget_exhausted','ambiguous')),
    checkpoint JSONB NOT NULL DEFAULT '{}'::jsonb,
    model_provenance JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (recording_id, processing_version, start_time, end_time),
    FOREIGN KEY (owner_user_id, recording_id)
        REFERENCES video_recordings(owner_user_id, id) ON DELETE CASCADE
);
CREATE INDEX audio_regions_claim_idx
    ON audio_regions(recording_id, processing_version, status, start_time);

CREATE TABLE audio_sources (
    id TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recording_id TEXT NOT NULL,
    audio_region_id TEXT NOT NULL REFERENCES audio_regions(id) ON DELETE CASCADE,
    processing_version INTEGER NOT NULL CHECK (processing_version > 0),
    source_index INTEGER NOT NULL CHECK (source_index >= 0),
    diarization_cluster_id TEXT,
    separation_status TEXT NOT NULL CHECK (separation_status IN (
        'not_required','accepted','ambiguous','rejected','failed','budget_exhausted'
    )),
    separation_confidence DOUBLE PRECISION CHECK (separation_confidence BETWEEN 0 AND 1),
    reconstruction_score DOUBLE PRECISION,
    speaker_match_score DOUBLE PRECISION CHECK (speaker_match_score BETWEEN -1 AND 1),
    speaker_runner_up_score DOUBLE PRECISION CHECK (speaker_runner_up_score BETWEEN -1 AND 1),
    derived_media_asset_id TEXT,
    transcription_status TEXT NOT NULL DEFAULT 'queued'
        CHECK (transcription_status IN ('queued','processing','completed','retryable_failed','dead','skipped')),
    model_provenance JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (audio_region_id, source_index),
    FOREIGN KEY (owner_user_id, recording_id)
        REFERENCES video_recordings(owner_user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (owner_user_id, derived_media_asset_id)
        REFERENCES media_assets(owner_user_id, id) ON DELETE RESTRICT
);
CREATE INDEX audio_sources_transcription_idx
    ON audio_sources(recording_id, processing_version, transcription_status, source_index);

CREATE TABLE audio_word_attributions (
    id TEXT PRIMARY KEY,
    audio_source_id TEXT NOT NULL REFERENCES audio_sources(id) ON DELETE CASCADE,
    segment_id TEXT NOT NULL,
    word_index INTEGER NOT NULL CHECK (word_index >= 0),
    word TEXT NOT NULL,
    start_time DOUBLE PRECISION NOT NULL CHECK (start_time >= 0),
    end_time DOUBLE PRECISION NOT NULL CHECK (end_time >= start_time),
    confidence DOUBLE PRECISION CHECK (confidence BETWEEN 0 AND 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (audio_source_id, segment_id, word_index)
);

ALTER TABLE identity_link_evidence
    ADD COLUMN overlap_group_id TEXT,
    ADD COLUMN separation_status TEXT,
    ADD COLUMN separation_score DOUBLE PRECISION CHECK (separation_score BETWEEN 0 AND 1),
    ADD COLUMN audio_source_id TEXT REFERENCES audio_sources(id) ON DELETE SET NULL,
    ADD COLUMN mouth_activity_score DOUBLE PRECISION CHECK (mouth_activity_score BETWEEN 0 AND 1),
    ADD COLUMN physical_presence_confidence DOUBLE PRECISION CHECK (physical_presence_confidence BETWEEN 0 AND 1),
    ADD COLUMN conflict_reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN model_provenance JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE media_assets
    ADD COLUMN parent_media_asset_id TEXT REFERENCES media_assets(id) ON DELETE CASCADE,
    ADD COLUMN access_scope TEXT NOT NULL DEFAULT 'standard'
        CHECK (access_scope IN ('standard','evidence_review'));

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION activate_analysis_run(run_id TEXT) RETURNS VOID
LANGUAGE plpgsql AS $$
DECLARE
    selected analysis_runs%ROWTYPE;
BEGIN
    SELECT * INTO selected FROM analysis_runs WHERE id=run_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'analysis run % does not exist', run_id;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM analysis_stage_jobs WHERE analysis_run_id=run_id) THEN
        RAISE EXCEPTION 'analysis run % has no stages', run_id;
    END IF;
    IF EXISTS (
        SELECT 1 FROM analysis_stage_jobs
        WHERE analysis_run_id=run_id AND required AND status <> 'completed'
    ) THEN
        RAISE EXCEPTION 'analysis run % has incomplete required stages', run_id;
    END IF;
    UPDATE analysis_runs SET is_active=FALSE,updated_at=NOW()
    WHERE recording_id=selected.recording_id AND is_active;
    UPDATE analysis_runs SET status='completed',is_active=TRUE,completed_at=NOW(),updated_at=NOW()
    WHERE id=run_id;
    UPDATE video_recordings SET processing_version=selected.processing_version,updated_at=NOW()
    WHERE id=selected.recording_id;
END;
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION activate_analysis_run(TEXT);
ALTER TABLE media_assets DROP COLUMN access_scope, DROP COLUMN parent_media_asset_id;
ALTER TABLE identity_link_evidence
    DROP COLUMN model_provenance,
    DROP COLUMN conflict_reasons,
    DROP COLUMN physical_presence_confidence,
    DROP COLUMN mouth_activity_score,
    DROP COLUMN audio_source_id,
    DROP COLUMN separation_score,
    DROP COLUMN separation_status,
    DROP COLUMN overlap_group_id;
DROP TABLE audio_word_attributions;
DROP TABLE audio_sources;
DROP TABLE audio_regions;
DROP TABLE face_track_observations;
ALTER TABLE person_tracks
    DROP COLUMN model_provenance,
    DROP COLUMN quality_summary,
    DROP COLUMN observation_count,
    DROP COLUMN last_frame,
    DROP COLUMN first_frame,
    DROP COLUMN lifecycle_status;
DROP TABLE analysis_stage_jobs;
DROP TABLE analysis_runs;
ALTER TABLE video_recordings
    DROP COLUMN requested_processing_version,
    ALTER COLUMN processing_version SET DEFAULT 2;
ALTER TABLE video_episodes ALTER COLUMN processing_version SET DEFAULT 2;
ALTER TABLE voice_episodes ALTER COLUMN processing_version SET DEFAULT 2;
