-- +goose Up
WITH refreshed_voice_episodes AS (
    UPDATE voice_episodes e
    SET graph_revision=e.graph_revision+1,
        status='queued', last_error='', updated_at=NOW()
    WHERE EXISTS (
        SELECT 1 FROM voice_jobs j
        WHERE j.kind='memograph' AND j.episode_id=e.id
    )
      AND EXISTS (
          SELECT 1 FROM jsonb_array_elements(COALESCE(e.segments, '[]'::jsonb)) segment
          WHERE NULLIF(segment->>'speaker_profile_id', '') IS NOT NULL
      )
    RETURNING e.id
), requeued_voice_jobs AS (
    UPDATE voice_jobs j
    SET status='queued', attempts=0, run_at=NOW(), locked_at=NULL,
        last_error='', updated_at=NOW()
    FROM refreshed_voice_episodes e
    WHERE j.kind='memograph' AND j.episode_id=e.id
    RETURNING j.episode_id
)
UPDATE voice_recordings r
SET status='memograph_pending', last_error='', updated_at=NOW()
WHERE EXISTS (
    SELECT 1 FROM voice_episode_recordings er
    JOIN refreshed_voice_episodes e ON e.id=er.episode_id
    WHERE er.recording_id=r.id
);

WITH refreshed_video_episodes AS (
    UPDATE video_episodes e
    SET graph_revision=e.graph_revision+1,
        status='queued', last_error='', updated_at=NOW()
    WHERE EXISTS (
        SELECT 1 FROM video_jobs j
        WHERE j.kind='memograph' AND j.episode_id=e.id
          AND j.source IN ('speech', 'legacy')
    )
      AND EXISTS (
          SELECT 1
          FROM video_recordings r,
               jsonb_array_elements(COALESCE(r.transcript->'segments', '[]'::jsonb)) segment
          WHERE r.id=e.recording_id
            AND NULLIF(segment->>'speaker_profile_id', '') IS NOT NULL
      )
    RETURNING e.id, e.recording_id
), requeued_video_jobs AS (
    UPDATE video_jobs j
    SET status='queued', attempts=0, run_at=NOW(), locked_at=NULL,
        last_error='', updated_at=NOW()
    FROM refreshed_video_episodes e
    WHERE j.kind='memograph' AND j.episode_id=e.id
      AND j.source IN ('speech', 'legacy')
    RETURNING j.episode_id
)
UPDATE video_recordings r
SET status='memograph_pending', last_error='', updated_at=NOW()
WHERE r.id IN (SELECT recording_id FROM refreshed_video_episodes);

-- +goose Down
-- Graph refreshes are idempotent and intentionally remain processed.
SELECT 1;
