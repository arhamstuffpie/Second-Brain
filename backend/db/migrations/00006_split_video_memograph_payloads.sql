-- +goose Up
ALTER TABLE video_episodes
    ADD COLUMN visual_description TEXT NOT NULL DEFAULT '',
    ADD COLUMN speech_description TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE video_episodes
    DROP COLUMN speech_description,
    DROP COLUMN visual_description;
