-- +goose Up
ALTER TABLE video_recordings
    ADD COLUMN speaker_reference_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE video_recordings
    DROP COLUMN speaker_reference_ids;
