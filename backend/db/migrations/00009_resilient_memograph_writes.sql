-- +goose Up
ALTER TABLE video_jobs
    ADD COLUMN source TEXT NOT NULL DEFAULT '';

UPDATE video_jobs
SET source = 'legacy'
WHERE kind = 'memograph';

ALTER TABLE video_jobs
    ADD CONSTRAINT video_jobs_source_check CHECK (
        (kind = 'memograph' AND source IN ('visual', 'speech', 'legacy')) OR
        (kind <> 'memograph' AND source = '')
    );

DROP INDEX video_jobs_memograph_unique;

CREATE UNIQUE INDEX video_jobs_memograph_source_unique
    ON video_jobs(episode_id, source)
    WHERE kind = 'memograph';

-- +goose Down
DELETE FROM video_jobs duplicate
USING video_jobs retained
WHERE duplicate.kind = 'memograph'
  AND retained.kind = 'memograph'
  AND duplicate.episode_id = retained.episode_id
  AND duplicate.id > retained.id;

DROP INDEX video_jobs_memograph_source_unique;

CREATE UNIQUE INDEX video_jobs_memograph_unique
    ON video_jobs(episode_id)
    WHERE kind = 'memograph';

ALTER TABLE video_jobs
    DROP CONSTRAINT video_jobs_source_check,
    DROP COLUMN source;
