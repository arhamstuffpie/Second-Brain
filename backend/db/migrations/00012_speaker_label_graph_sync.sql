-- +goose Up
ALTER TABLE voice_episodes
    ADD COLUMN graph_revision BIGINT NOT NULL DEFAULT 0
        CHECK (graph_revision >= 0);

ALTER TABLE video_episodes
    ADD COLUMN graph_revision BIGINT NOT NULL DEFAULT 0
        CHECK (graph_revision >= 0);

-- +goose Down
ALTER TABLE video_episodes DROP COLUMN graph_revision;
ALTER TABLE voice_episodes DROP COLUMN graph_revision;
