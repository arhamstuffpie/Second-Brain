-- +goose Up
-- +goose StatementBegin
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid='identity_link_evidence'::regclass
      AND contype='u'
      AND pg_get_constraintdef(oid) LIKE '%owner_user_id, recording_id, person_track_id, voice_speaker_profile_id, processing_version%';
    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE identity_link_evidence DROP CONSTRAINT %I', constraint_name);
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE identity_link_evidence
    ALTER COLUMN person_track_id DROP NOT NULL,
    ADD COLUMN deterministic_key TEXT,
    ADD COLUMN canonical_person_profile_id TEXT,
    ADD COLUMN segment_id TEXT,
    ADD COLUMN segment_start_time DOUBLE PRECISION CHECK (segment_start_time IS NULL OR segment_start_time >= 0),
    ADD COLUMN segment_end_time DOUBLE PRECISION CHECK (
        segment_end_time IS NULL OR segment_start_time IS NULL OR segment_end_time > segment_start_time
    ),
    ADD COLUMN active_speaker_runner_up_score DOUBLE PRECISION
        CHECK (active_speaker_runner_up_score BETWEEN 0 AND 1),
    ADD COLUMN decision_margin DOUBLE PRECISION CHECK (decision_margin BETWEEN -1 AND 1),
    ADD COLUMN combined_score DOUBLE PRECISION CHECK (combined_score BETWEEN 0 AND 1),
    ADD COLUMN raw_evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT identity_link_evidence_canonical_person_fk
        FOREIGN KEY (owner_user_id, canonical_person_profile_id)
        REFERENCES person_profiles(owner_user_id, id) ON DELETE CASCADE;

UPDATE identity_link_evidence SET deterministic_key='legacy:' || id WHERE deterministic_key IS NULL;
ALTER TABLE identity_link_evidence ALTER COLUMN deterministic_key SET NOT NULL;
CREATE UNIQUE INDEX identity_link_evidence_deterministic_key_idx
    ON identity_link_evidence(deterministic_key);
CREATE INDEX identity_link_fusion_aggregate_idx
    ON identity_link_evidence(
        owner_user_id,canonical_person_profile_id,person_track_id,decision,created_at
    ) WHERE canonical_person_profile_id IS NOT NULL AND person_track_id IS NOT NULL;

ALTER TABLE person_tracks
    ADD COLUMN resolution_method TEXT NOT NULL DEFAULT '',
    ADD COLUMN resolution_evidence_count INTEGER NOT NULL DEFAULT 0
        CHECK (resolution_evidence_count >= 0),
    ADD COLUMN resolution_confidence DOUBLE PRECISION
        CHECK (resolution_confidence BETWEEN 0 AND 1),
    ADD COLUMN resolution_processing_version INTEGER
        CHECK (resolution_processing_version IS NULL OR resolution_processing_version > 0),
    ADD COLUMN resolved_at TIMESTAMPTZ;

ALTER TABLE face_profiles
    ADD COLUMN enrollment_source TEXT NOT NULL DEFAULT '',
    ADD COLUMN source_evidence_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE face_profiles
    DROP COLUMN source_evidence_ids,
    DROP COLUMN enrollment_source;
ALTER TABLE person_tracks
    DROP COLUMN resolved_at,
    DROP COLUMN resolution_processing_version,
    DROP COLUMN resolution_confidence,
    DROP COLUMN resolution_evidence_count,
    DROP COLUMN resolution_method;
DROP INDEX identity_link_fusion_aggregate_idx;
DROP INDEX identity_link_evidence_deterministic_key_idx;
ALTER TABLE identity_link_evidence
    DROP CONSTRAINT identity_link_evidence_canonical_person_fk,
    DROP COLUMN raw_evidence,
    DROP COLUMN combined_score,
    DROP COLUMN decision_margin,
    DROP COLUMN active_speaker_runner_up_score,
    DROP COLUMN segment_end_time,
    DROP COLUMN segment_start_time,
    DROP COLUMN segment_id,
    DROP COLUMN canonical_person_profile_id,
    DROP COLUMN deterministic_key,
    ALTER COLUMN person_track_id SET NOT NULL,
    ADD UNIQUE (owner_user_id,recording_id,person_track_id,voice_speaker_profile_id,processing_version);
