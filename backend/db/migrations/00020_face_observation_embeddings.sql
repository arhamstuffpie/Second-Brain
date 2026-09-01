-- +goose Up
CREATE TABLE face_track_observation_embeddings (
    observation_id TEXT PRIMARY KEY REFERENCES face_track_observations(id) ON DELETE CASCADE,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recording_id TEXT NOT NULL,
    processing_version INTEGER NOT NULL CHECK (processing_version > 0),
    embedding_model TEXT NOT NULL,
    embedding_dimensions INTEGER NOT NULL CHECK (embedding_dimensions > 0),
    embedding DOUBLE PRECISION[] NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (cardinality(embedding) = embedding_dimensions),
    FOREIGN KEY (owner_user_id, recording_id)
        REFERENCES video_recordings(owner_user_id, id) ON DELETE CASCADE
);
CREATE INDEX face_track_observation_embeddings_recording_idx
    ON face_track_observation_embeddings(owner_user_id, recording_id, processing_version);

-- Existing recordings receive one initial dense stage. New recordings are
-- enqueued transactionally by video_repository.go.
INSERT INTO analysis_runs (
    owner_user_id,recording_id,processing_version,status,configuration_profile
)
SELECT owner_user_id,id,GREATEST(processing_version,requested_processing_version),
       'queued','pending:dense_person_tracking'
FROM video_recordings
ON CONFLICT (recording_id,processing_version) DO NOTHING;

INSERT INTO analysis_stage_jobs (analysis_run_id,stage,required,max_attempts)
SELECT a.id,'dense_person_tracking',TRUE,5
FROM analysis_runs a
JOIN video_recordings r ON r.id=a.recording_id
WHERE a.processing_version=GREATEST(r.processing_version,r.requested_processing_version)
ON CONFLICT (analysis_run_id,stage) DO NOTHING;

-- +goose Down
DROP TABLE face_track_observation_embeddings;
