package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/arham/ai-second-brain/internal/service"
)

type VoiceRepository interface {
	service.VoiceRepository
}

type voiceRepository struct {
	*base
}

func newVoiceRepository(base *base) *voiceRepository {
	return &voiceRepository{base: base}
}

func (r *voiceRepository) CreateRecording(ctx context.Context, input service.CreateRecordingInput, maxAttempts int) (service.VoiceRecording, error) {
	const query = `
WITH recording AS (
	INSERT INTO voice_recordings (
		owner_user_id, session_id, group_id, memory_id, device_id, location,
		file_name, file_path, media_type, size_bytes, start_offset_seconds,
		default_confidence, chunk_index, is_final
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	RETURNING id, session_id, group_id, memory_id, status, file_name, media_type, size_bytes, created_at
), job AS (
	INSERT INTO voice_jobs (kind, recording_id, max_attempts)
	SELECT 'stt', id, $15 FROM recording
)
SELECT id, session_id, group_id, memory_id, status, file_name, media_type, size_bytes, created_at
FROM recording`
	var recording service.VoiceRecording
	err := r.db.QueryRowContext(ctx, query,
		input.OwnerUserID, input.SessionID, input.GroupID, input.MemoryID,
		input.DeviceID, input.Location, input.FileName, input.FilePath,
		input.MediaType, input.SizeBytes, input.StartOffset, input.DefaultConfidence,
		input.ChunkIndex, input.IsFinal, maxAttempts,
	).Scan(
		&recording.ID, &recording.SessionID, &recording.GroupID, &recording.MemoryID,
		&recording.Status, &recording.FileName, &recording.MediaType,
		&recording.SizeBytes, &recording.CreatedAt,
	)
	if err != nil {
		return service.VoiceRecording{}, fmt.Errorf("create voice recording: %w", err)
	}
	recording.ChunkIndex = input.ChunkIndex
	recording.IsFinal = input.IsFinal
	return recording, nil
}

func (r *voiceRepository) FindRecordingByChunk(
	ctx context.Context,
	ownerUserID, sessionID string,
	chunkIndex int,
) (service.VoiceRecording, bool, error) {
	const query = `
SELECT id, session_id, group_id, memory_id, status, file_name, media_type,
       size_bytes, chunk_index, is_final, created_at
FROM voice_recordings
WHERE owner_user_id = $1 AND session_id = $2 AND chunk_index = $3`
	var recording service.VoiceRecording
	var storedChunkIndex int
	err := r.db.QueryRowContext(ctx, query, ownerUserID, sessionID, chunkIndex).Scan(
		&recording.ID, &recording.SessionID, &recording.GroupID, &recording.MemoryID,
		&recording.Status, &recording.FileName, &recording.MediaType, &recording.SizeBytes,
		&storedChunkIndex, &recording.IsFinal, &recording.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.VoiceRecording{}, false, nil
	}
	if err != nil {
		return service.VoiceRecording{}, false, fmt.Errorf("find realtime voice chunk: %w", err)
	}
	recording.ChunkIndex = &storedChunkIndex
	return recording, true, nil
}

func (r *voiceRepository) GetRecording(ctx context.Context, id, ownerUserID string) (service.VoiceRecordingDetail, error) {
	const recordingQuery = `
SELECT r.id, r.session_id, r.group_id, r.memory_id,
       CASE WHEN EXISTS (SELECT 1 FROM voice_episodes e WHERE e.recording_id = r.id)
            THEN CASE
                WHEN EXISTS (
                    SELECT 1 FROM voice_episodes e
                    WHERE e.recording_id = r.id AND e.status = 'failed'
                ) THEN 'failed'
                WHEN NOT EXISTS (
                    SELECT 1 FROM voice_episodes e
                    WHERE e.recording_id = r.id AND e.status <> 'completed'
                ) THEN 'completed'
                ELSE 'memograph_pending'
            END
            ELSE r.status
       END AS effective_status,
       r.file_name, r.media_type, r.size_bytes, r.chunk_index, r.is_final,
       r.device_id, r.location, r.transcript, r.last_error, r.created_at, r.updated_at
FROM voice_recordings r
WHERE r.id = $1 AND r.owner_user_id = $2`
	var result service.VoiceRecordingDetail
	var transcript []byte
	var chunkIndex sql.NullInt64
	err := r.db.QueryRowContext(ctx, recordingQuery, id, ownerUserID).Scan(
		&result.ID, &result.SessionID, &result.GroupID, &result.MemoryID,
		&result.Status, &result.FileName, &result.MediaType, &result.SizeBytes,
		&chunkIndex, &result.IsFinal, &result.DeviceID, &result.Location, &transcript, &result.LastError,
		&result.CreatedAt, &result.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.VoiceRecordingDetail{}, service.ErrNotFound
	}
	if err != nil {
		return service.VoiceRecordingDetail{}, fmt.Errorf("get voice recording: %w", err)
	}
	if chunkIndex.Valid {
		value := int(chunkIndex.Int64)
		result.ChunkIndex = &value
	}
	if len(transcript) > 0 {
		var decoded service.Transcript
		if err := json.Unmarshal(transcript, &decoded); err != nil {
			return service.VoiceRecordingDetail{}, fmt.Errorf("decode stored transcript: %w", err)
		}
		result.Transcript = &decoded
	}

	const episodesQuery = `
SELECT id, bucket_index, start_time, end_time, description, confidence, status,
       memograph_response, last_error
FROM voice_episodes
WHERE recording_id = $1
ORDER BY bucket_index`
	rows, err := r.db.QueryContext(ctx, episodesQuery, id)
	if err != nil {
		return service.VoiceRecordingDetail{}, fmt.Errorf("list voice episodes: %w", err)
	}
	defer rows.Close()
	result.Episodes = make([]service.VoiceEpisode, 0)
	for rows.Next() {
		var episode service.VoiceEpisode
		var confidence sql.NullFloat64
		var response []byte
		if err := rows.Scan(
			&episode.ID, &episode.BucketIndex, &episode.StartTime, &episode.EndTime,
			&episode.Description, &confidence, &episode.Status, &response, &episode.LastError,
		); err != nil {
			return service.VoiceRecordingDetail{}, fmt.Errorf("scan voice episode: %w", err)
		}
		if confidence.Valid {
			episode.Confidence = &confidence.Float64
		}
		if len(response) > 0 {
			episode.Response = json.RawMessage(response)
		}
		result.Episodes = append(result.Episodes, episode)
	}
	if err := rows.Err(); err != nil {
		return service.VoiceRecordingDetail{}, fmt.Errorf("iterate voice episodes: %w", err)
	}
	return result, nil
}

func (r *voiceRepository) CreateRealtimeSession(
	ctx context.Context,
	input service.StartRealtimeSessionInput,
) (service.RealtimeVoiceSession, error) {
	const query = `
WITH generated AS (
	SELECT gen_random_uuid()::text AS id
)
INSERT INTO voice_realtime_sessions (
	id, owner_user_id, memory_id, group_id, device_id, location, chunk_duration_seconds
)
SELECT id, $1, $2, COALESCE(NULLIF($3, ''), id), $4, $5, $6
FROM generated
RETURNING id, memory_id, group_id, device_id, location, chunk_duration_seconds,
          status, created_at, updated_at, stopped_at`
	var session service.RealtimeVoiceSession
	var stoppedAt sql.NullTime
	err := r.db.QueryRowContext(
		ctx, query, input.OwnerUserID, input.MemoryID, input.GroupID,
		input.DeviceID, input.Location, input.ChunkDurationSeconds,
	).Scan(
		&session.ID, &session.MemoryID, &session.GroupID, &session.DeviceID,
		&session.Location, &session.ChunkDurationSeconds, &session.Status,
		&session.CreatedAt, &session.UpdatedAt, &stoppedAt,
	)
	if err != nil {
		return service.RealtimeVoiceSession{}, fmt.Errorf("create realtime voice session: %w", err)
	}
	return session, nil
}

func (r *voiceRepository) GetRealtimeSession(
	ctx context.Context,
	id, ownerUserID string,
) (service.RealtimeVoiceSessionDetail, error) {
	const sessionQuery = `
SELECT id, memory_id, group_id, device_id, location, chunk_duration_seconds,
       status, created_at, updated_at, stopped_at
FROM voice_realtime_sessions
WHERE id = $1 AND owner_user_id = $2`
	var result service.RealtimeVoiceSessionDetail
	var stoppedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, sessionQuery, id, ownerUserID).Scan(
		&result.ID, &result.MemoryID, &result.GroupID, &result.DeviceID,
		&result.Location, &result.ChunkDurationSeconds, &result.Status,
		&result.CreatedAt, &result.UpdatedAt, &stoppedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.RealtimeVoiceSessionDetail{}, service.ErrNotFound
	}
	if err != nil {
		return service.RealtimeVoiceSessionDetail{}, fmt.Errorf("get realtime voice session: %w", err)
	}
	if stoppedAt.Valid {
		result.StoppedAt = &stoppedAt.Time
	}

	const chunksQuery = `
SELECT r.id, r.session_id, r.group_id, r.memory_id,
       CASE WHEN EXISTS (SELECT 1 FROM voice_episodes e WHERE e.recording_id = r.id)
            THEN CASE
                WHEN EXISTS (
                    SELECT 1 FROM voice_episodes e
                    WHERE e.recording_id = r.id AND e.status = 'failed'
                ) THEN 'failed'
                WHEN NOT EXISTS (
                    SELECT 1 FROM voice_episodes e
                    WHERE e.recording_id = r.id AND e.status <> 'completed'
                ) THEN 'completed'
                ELSE 'memograph_pending'
            END
            ELSE r.status
       END AS effective_status,
       r.file_name, r.media_type, r.size_bytes, r.chunk_index, r.is_final, r.created_at
FROM voice_recordings r
WHERE r.owner_user_id = $1 AND r.session_id = $2 AND r.chunk_index IS NOT NULL
ORDER BY r.chunk_index`
	rows, err := r.db.QueryContext(ctx, chunksQuery, ownerUserID, id)
	if err != nil {
		return service.RealtimeVoiceSessionDetail{}, fmt.Errorf("list realtime voice chunks: %w", err)
	}
	defer rows.Close()
	result.Progress.LatestChunkIndex = -1
	result.Chunks = make([]service.VoiceRecording, 0)
	for rows.Next() {
		var recording service.VoiceRecording
		var chunkIndex int
		if err := rows.Scan(
			&recording.ID, &recording.SessionID, &recording.GroupID, &recording.MemoryID,
			&recording.Status, &recording.FileName, &recording.MediaType,
			&recording.SizeBytes, &chunkIndex, &recording.IsFinal, &recording.CreatedAt,
		); err != nil {
			return service.RealtimeVoiceSessionDetail{}, fmt.Errorf("scan realtime voice chunk: %w", err)
		}
		recording.ChunkIndex = &chunkIndex
		result.Chunks = append(result.Chunks, recording)
		result.Progress.Total++
		if chunkIndex > result.Progress.LatestChunkIndex {
			result.Progress.LatestChunkIndex = chunkIndex
		}
		switch recording.Status {
		case "queued":
			result.Progress.Queued++
		case "transcribing", "memograph_pending":
			result.Progress.Processing++
		case "completed":
			result.Progress.Completed++
		case "failed":
			result.Progress.Failed++
		}
	}
	if err := rows.Err(); err != nil {
		return service.RealtimeVoiceSessionDetail{}, fmt.Errorf("iterate realtime voice chunks: %w", err)
	}
	return result, nil
}

func (r *voiceRepository) StopRealtimeSession(
	ctx context.Context,
	id, ownerUserID string,
) (service.RealtimeVoiceSession, error) {
	const query = `
UPDATE voice_realtime_sessions
SET status = 'stopped', stopped_at = COALESCE(stopped_at, NOW()), updated_at = NOW()
WHERE id = $1 AND owner_user_id = $2
RETURNING id, memory_id, group_id, device_id, location, chunk_duration_seconds,
          status, created_at, updated_at, stopped_at`
	var session service.RealtimeVoiceSession
	var stoppedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, id, ownerUserID).Scan(
		&session.ID, &session.MemoryID, &session.GroupID, &session.DeviceID,
		&session.Location, &session.ChunkDurationSeconds, &session.Status,
		&session.CreatedAt, &session.UpdatedAt, &stoppedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.RealtimeVoiceSession{}, service.ErrNotFound
	}
	if err != nil {
		return service.RealtimeVoiceSession{}, fmt.Errorf("stop realtime voice session: %w", err)
	}
	if stoppedAt.Valid {
		session.StoppedAt = &stoppedAt.Time
	}
	return session, nil
}

func (r *voiceRepository) ClaimJob(ctx context.Context) (service.VoiceJob, bool, error) {
	const query = `
WITH candidate AS (
	SELECT id FROM voice_jobs
	WHERE (status = 'queued' AND run_at <= NOW())
	   OR (status = 'processing' AND locked_at < NOW() - INTERVAL '10 minutes')
	ORDER BY run_at, id
	FOR UPDATE SKIP LOCKED
	LIMIT 1
), claimed AS (
	UPDATE voice_jobs j
	SET status = 'processing', attempts = attempts + 1, locked_at = NOW(), updated_at = NOW()
	FROM candidate c
	WHERE j.id = c.id
	RETURNING j.*
), target AS (
	SELECT c.*,
	       COALESCE(c.recording_id, e.recording_id) AS target_recording_id
	FROM claimed c
	LEFT JOIN voice_episodes e ON e.id = c.episode_id
), recording_status AS (
	UPDATE voice_recordings r
	SET status = CASE WHEN target.kind = 'stt' THEN 'transcribing' ELSE r.status END,
	    updated_at = NOW()
	FROM target
	WHERE r.id = target.target_recording_id
), episode_status AS (
	UPDATE voice_episodes e
	SET status = 'writing', updated_at = NOW()
	FROM target
	WHERE e.id = target.episode_id
)
SELECT target.id, target.kind, COALESCE(target.recording_id, ''), COALESCE(target.episode_id, ''),
       target.attempts, target.max_attempts,
       r.file_path, r.file_name, r.media_type, r.session_id, r.group_id, r.memory_id,
       r.device_id, r.location, r.start_offset_seconds,
       COALESCE(e.description, ''), COALESCE(e.start_time, 0), COALESCE(e.end_time, 0),
       COALESCE(e.confidence, r.default_confidence)
FROM target
JOIN voice_recordings r ON r.id = target.target_recording_id
LEFT JOIN voice_episodes e ON e.id = target.episode_id`
	var job service.VoiceJob
	var confidence sql.NullFloat64
	err := r.db.QueryRowContext(ctx, query).Scan(
		&job.ID, &job.Kind, &job.RecordingID, &job.EpisodeID,
		&job.Attempts, &job.MaxAttempts, &job.FilePath, &job.FileName,
		&job.MediaType, &job.SessionID, &job.GroupID, &job.MemoryID,
		&job.DeviceID, &job.Location, &job.StartOffset,
		&job.Description, &job.EpisodeStart, &job.EpisodeEnd, &confidence,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.VoiceJob{}, false, nil
	}
	if err != nil {
		return service.VoiceJob{}, false, fmt.Errorf("claim voice job: %w", err)
	}
	if confidence.Valid {
		job.Confidence = &confidence.Float64
	}
	return job, true, nil
}

func (r *voiceRepository) SaveTranscriptAndEpisodes(
	ctx context.Context,
	job service.VoiceJob,
	transcript service.Transcript,
	episodes []service.EpisodeDraft,
	provider, model string,
	maxAttempts int,
) error {
	transcriptJSON, err := json.Marshal(transcript)
	if err != nil {
		return fmt.Errorf("encode transcript: %w", err)
	}
	episodesJSON, err := json.Marshal(episodes)
	if err != nil {
		return fmt.Errorf("encode episodes: %w", err)
	}
	const query = `
WITH updated_recording AS (
	UPDATE voice_recordings
	SET transcript = $2::jsonb, stt_provider = $3, stt_model = $4,
	    status = 'memograph_pending', last_error = '', updated_at = NOW()
	WHERE id = $1
), episode_rows AS (
	SELECT * FROM jsonb_to_recordset($5::jsonb) AS x(
		bucket_index integer, start_time double precision, end_time double precision,
		description text, confidence double precision
	)
), inserted_episodes AS (
	INSERT INTO voice_episodes (
		recording_id, bucket_index, start_time, end_time, description, confidence
	)
	SELECT $1, bucket_index, start_time, end_time, description, confidence
	FROM episode_rows
	ON CONFLICT (recording_id, bucket_index) DO UPDATE
	SET start_time = EXCLUDED.start_time, end_time = EXCLUDED.end_time,
	    description = EXCLUDED.description, confidence = EXCLUDED.confidence,
	    updated_at = NOW()
	RETURNING id
), inserted_jobs AS (
	INSERT INTO voice_jobs (kind, episode_id, max_attempts)
	SELECT 'memograph', id, $6 FROM inserted_episodes
	ON CONFLICT (episode_id) WHERE kind = 'memograph' DO NOTHING
)
UPDATE voice_jobs
SET status = 'completed', updated_at = NOW()
WHERE id = $7`
	if _, err := r.db.ExecContext(ctx, query, job.RecordingID, transcriptJSON, provider, model, episodesJSON, maxAttempts, job.ID); err != nil {
		return fmt.Errorf("save transcript and episodes: %w", err)
	}
	return nil
}

func (r *voiceRepository) CompleteMemographEpisode(ctx context.Context, job service.VoiceJob, response json.RawMessage) error {
	const query = `
WITH completed_episode AS (
	UPDATE voice_episodes
	SET status = 'completed', memograph_response = $2::jsonb, last_error = '', updated_at = NOW()
	WHERE id = $1
	RETURNING recording_id
), completed_job AS (
	UPDATE voice_jobs SET status = 'completed', updated_at = NOW() WHERE id = $3
)
UPDATE voice_recordings r
SET status = CASE WHEN NOT EXISTS (
		SELECT 1 FROM voice_episodes e
		WHERE e.recording_id = r.id AND e.id <> $1 AND e.status <> 'completed'
	) THEN 'completed' ELSE 'memograph_pending' END,
	last_error = '', updated_at = NOW()
FROM completed_episode ce
WHERE r.id = ce.recording_id`
	if _, err := r.db.ExecContext(ctx, query, job.EpisodeID, []byte(response), job.ID); err != nil {
		return fmt.Errorf("complete Memograph episode: %w", err)
	}
	return nil
}

func (r *voiceRepository) RetryJob(ctx context.Context, job service.VoiceJob, cause string, runAt time.Time, dead bool) error {
	status := "queued"
	if dead {
		status = "dead"
	}
	const updateJob = `
UPDATE voice_jobs
SET status = $2, run_at = $3, locked_at = NULL, last_error = $4, updated_at = NOW()
WHERE id = $1`
	if _, err := r.db.ExecContext(ctx, updateJob, job.ID, status, runAt, cause); err != nil {
		return fmt.Errorf("retry voice job: %w", err)
	}
	if job.Kind == "stt" {
		recordingStatus := "queued"
		if dead {
			recordingStatus = "failed"
		}
		_, err := r.db.ExecContext(ctx, `
UPDATE voice_recordings SET status = $2, last_error = $3, updated_at = NOW() WHERE id = $1`,
			job.RecordingID, recordingStatus, cause)
		if err != nil {
			return fmt.Errorf("update failed recording: %w", err)
		}
		return nil
	}
	episodeStatus := "queued"
	if dead {
		episodeStatus = "failed"
	}
	if _, err := r.db.ExecContext(ctx, `
UPDATE voice_episodes SET status = $2, last_error = $3, updated_at = NOW() WHERE id = $1`,
		job.EpisodeID, episodeStatus, cause); err != nil {
		return fmt.Errorf("update failed episode: %w", err)
	}
	if dead {
		_, err := r.db.ExecContext(ctx, `
UPDATE voice_recordings r SET status = 'failed', last_error = $2, updated_at = NOW()
FROM voice_episodes e WHERE e.id = $1 AND r.id = e.recording_id`, job.EpisodeID, cause)
		if err != nil {
			return fmt.Errorf("update failed recording: %w", err)
		}
	}
	return nil
}

var _ VoiceRepository = (*voiceRepository)(nil)
