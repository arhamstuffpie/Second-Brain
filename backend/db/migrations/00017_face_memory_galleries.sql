-- +goose Up
ALTER TABLE face_profile_samples
    ALTER COLUMN file_name DROP NOT NULL,
    ALTER COLUMN file_path DROP NOT NULL,
    ALTER COLUMN media_type DROP NOT NULL,
    ALTER COLUMN size_bytes DROP NOT NULL,
    DROP CONSTRAINT face_profile_samples_size_bytes_check,
    ADD CONSTRAINT face_profile_samples_size_bytes_check
        CHECK (size_bytes IS NULL OR size_bytes > 0),
    ADD COLUMN pose_bucket TEXT NOT NULL DEFAULT 'frontal'
        CHECK (pose_bucket IN (
            'frontal','left_three_quarter','left_profile',
            'right_three_quarter','right_profile'
        )),
    ADD COLUMN yaw DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (yaw BETWEEN -90 AND 90),
    ADD COLUMN pitch DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (pitch BETWEEN -90 AND 90),
    ADD COLUMN roll DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (roll BETWEEN -180 AND 180),
    ADD COLUMN quality_score DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (quality_score BETWEEN 0 AND 1),
    ADD COLUMN source_recording_id TEXT,
    ADD COLUMN source_track_id TEXT,
    ADD COLUMN observed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD CONSTRAINT face_profile_samples_recording_fk
        FOREIGN KEY (owner_user_id, source_recording_id)
        REFERENCES video_recordings(owner_user_id, id) ON DELETE SET NULL (source_recording_id);

CREATE INDEX face_profile_samples_gallery_idx
    ON face_profile_samples(owner_user_id, face_profile_id, pose_bucket, quality_score DESC, observed_at DESC);

-- +goose Down
DROP INDEX face_profile_samples_gallery_idx;
DELETE FROM face_profile_samples WHERE file_path IS NULL;
ALTER TABLE face_profile_samples
    DROP CONSTRAINT face_profile_samples_recording_fk,
    DROP COLUMN observed_at,
    DROP COLUMN source_track_id,
    DROP COLUMN source_recording_id,
    DROP COLUMN quality_score,
    DROP COLUMN roll,
    DROP COLUMN pitch,
    DROP COLUMN yaw,
    DROP COLUMN pose_bucket,
    DROP CONSTRAINT face_profile_samples_size_bytes_check,
    ADD CONSTRAINT face_profile_samples_size_bytes_check CHECK (size_bytes > 0),
    ALTER COLUMN size_bytes SET NOT NULL,
    ALTER COLUMN media_type SET NOT NULL,
    ALTER COLUMN file_path SET NOT NULL,
    ALTER COLUMN file_name SET NOT NULL;
