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

type VideoRepository interface {
	service.VideoRepository
}

type videoRepository struct {
	*base
}

func newVideoRepository(base *base) *videoRepository {
	return &videoRepository{base: base}
}

func (r *videoRepository) CreateVideoRecording(
	ctx context.Context,
	input service.CreateVideoRecordingInput,
	maxAttempts int,
) (service.VideoRecording, error) {
	const query = `
WITH asset AS (
	INSERT INTO media_assets (owner_user_id, session_id, source_kind, storage_provider, bucket, object_key, file_name, media_type, size_bytes, sha256)
	VALUES ($1,$2,'video',$14,$15,$8,$7,$9,$10,$16) RETURNING id
), recording AS (
	INSERT INTO video_recordings (
		owner_user_id, session_id, group_id, memory_id, device_id, location,
		file_name, file_path, media_type, size_bytes, start_offset_seconds,
		frame_interval_seconds, default_confidence, media_asset_id
	) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,id FROM asset
	RETURNING *
), jobs AS (
	INSERT INTO video_jobs (kind, recording_id, max_attempts)
	SELECT kind, recording.id, $17
	FROM recording
	CROSS JOIN (VALUES ('audio'), ('visual')) AS kinds(kind)
)
SELECT id, session_id, group_id, memory_id, status, audio_status, visual_status,
       merge_status, file_name, media_type, size_bytes, start_offset_seconds, created_at
FROM recording`
	var result service.VideoRecording
	err := r.db.QueryRowContext(
		ctx, query,
		input.OwnerUserID, input.SessionID, input.GroupID, input.MemoryID,
		input.DeviceID, input.Location, input.FileName, input.FilePath,
		input.MediaType, input.SizeBytes, input.StartOffset, input.FrameInterval,
		input.DefaultConfidence, input.StorageProvider, input.StorageBucket, input.SHA256, maxAttempts,
	).Scan(
		&result.ID, &result.SessionID, &result.GroupID, &result.MemoryID,
		&result.Status, &result.AudioStatus, &result.VisualStatus, &result.MergeStatus,
		&result.FileName, &result.MediaType, &result.SizeBytes, &result.StartTime,
		&result.CreatedAt,
	)
	if err != nil {
		return service.VideoRecording{}, fmt.Errorf("create video recording: %w", err)
	}
	return result, nil
}

func (r *videoRepository) CreateRealtimeVideoChunk(
	ctx context.Context,
	input service.CreateRealtimeVideoChunkInput,
	maxAttempts int,
) (service.VideoRecording, error) {
	const query = `
WITH claimed_session AS (
	UPDATE video_realtime_sessions
	SET next_chunk_index = next_chunk_index + 1, updated_at = NOW()
	WHERE id = $1 AND owner_user_id = $2 AND status = 'active'
	RETURNING *, next_chunk_index - 1 AS assigned_chunk_index
), asset AS (
	INSERT INTO media_assets (owner_user_id, session_id, chunk_id, source_kind, storage_provider, bucket, object_key, file_name, media_type, size_bytes, sha256)
	SELECT $2,id,$3,'video',$10,$11,$6,$5,$7,$8,$12 FROM claimed_session RETURNING id
), recording AS (
	INSERT INTO video_recordings (
		owner_user_id, realtime_session_id, session_id, group_id, memory_id,
		device_id, location, client_chunk_id, chunk_index, is_final,
		file_name, file_path, media_type, size_bytes, start_offset_seconds,
		frame_interval_seconds, default_confidence, media_asset_id
	)
	SELECT $2, claimed_session.id, claimed_session.id,
	       claimed_session.group_id, claimed_session.memory_id,
	       claimed_session.device_id, claimed_session.location, $3,
	       claimed_session.assigned_chunk_index, $4, $5, $6, $7, $8,
	       claimed_session.assigned_chunk_index * claimed_session.chunk_duration_seconds,
	       claimed_session.frame_interval_seconds, $9, asset.id
	FROM claimed_session CROSS JOIN asset
	RETURNING *
), jobs AS (
	INSERT INTO video_jobs (kind, recording_id, max_attempts)
	SELECT kind, recording.id, $13
	FROM recording
	CROSS JOIN (VALUES ('audio'), ('visual')) AS kinds(kind)
)
SELECT id, session_id, group_id, memory_id, status, audio_status, visual_status,
       merge_status, file_name, media_type, size_bytes, client_chunk_id,
       chunk_index, start_offset_seconds, is_final, created_at
FROM recording`
	var result service.VideoRecording
	var chunkIndex int
	err := r.db.QueryRowContext(
		ctx, query,
		input.RealtimeSessionID, input.OwnerUserID, input.ClientChunkID, input.IsFinal,
		input.FileName, input.FilePath, input.MediaType, input.SizeBytes,
		input.DefaultConfidence, input.StorageProvider, input.StorageBucket, input.SHA256, maxAttempts,
	).Scan(
		&result.ID, &result.SessionID, &result.GroupID, &result.MemoryID,
		&result.Status, &result.AudioStatus, &result.VisualStatus, &result.MergeStatus,
		&result.FileName, &result.MediaType, &result.SizeBytes, &result.ClientChunkID,
		&chunkIndex, &result.StartTime, &result.IsFinal, &result.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.VideoRecording{}, service.ErrConflict
	}
	if err != nil {
		return service.VideoRecording{}, fmt.Errorf("create realtime video chunk: %w", err)
	}
	result.ChunkIndex = &chunkIndex
	return result, nil
}

func (r *videoRepository) FindVideoRecordingByClientChunk(
	ctx context.Context,
	ownerUserID, sessionID, clientChunkID string,
) (service.VideoRecording, bool, error) {
	const query = `
SELECT id, session_id, group_id, memory_id, status, audio_status, visual_status,
       merge_status, file_name, media_type, size_bytes, client_chunk_id,
       chunk_index, start_offset_seconds, is_final, created_at
FROM video_recordings
WHERE owner_user_id = $1 AND session_id = $2 AND client_chunk_id = $3`
	var result service.VideoRecording
	var chunkIndex sql.NullInt64
	err := r.db.QueryRowContext(ctx, query, ownerUserID, sessionID, clientChunkID).Scan(
		&result.ID, &result.SessionID, &result.GroupID, &result.MemoryID,
		&result.Status, &result.AudioStatus, &result.VisualStatus, &result.MergeStatus,
		&result.FileName, &result.MediaType, &result.SizeBytes, &result.ClientChunkID,
		&chunkIndex, &result.StartTime, &result.IsFinal, &result.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.VideoRecording{}, false, nil
	}
	if err != nil {
		return service.VideoRecording{}, false, fmt.Errorf("find realtime video chunk: %w", err)
	}
	if chunkIndex.Valid {
		value := int(chunkIndex.Int64)
		result.ChunkIndex = &value
	}
	return result, true, nil
}

func (r *videoRepository) GetVideoRecording(
	ctx context.Context,
	id, ownerUserID string,
) (service.VideoRecordingDetail, error) {
	const query = `
SELECT id, session_id, group_id, memory_id, status, audio_status, visual_status,
       merge_status, file_name, media_type, size_bytes, COALESCE(client_chunk_id, ''),
       chunk_index, start_offset_seconds, is_final, device_id, location,
       stt_provider, stt_model, visual_provider, visual_model,
       speaker_reference_ids, transcript, visual_analysis, last_error,
       created_at, updated_at
FROM video_recordings
WHERE id = $1 AND owner_user_id = $2`
	var result service.VideoRecordingDetail
	var chunkIndex sql.NullInt64
	var referenceIDsJSON, transcriptJSON, visualJSON []byte
	err := r.db.QueryRowContext(ctx, query, id, ownerUserID).Scan(
		&result.ID, &result.SessionID, &result.GroupID, &result.MemoryID,
		&result.Status, &result.AudioStatus, &result.VisualStatus, &result.MergeStatus,
		&result.FileName, &result.MediaType, &result.SizeBytes, &result.ClientChunkID,
		&chunkIndex, &result.StartTime, &result.IsFinal, &result.DeviceID,
		&result.Location, &result.STTProvider, &result.STTModel,
		&result.VisualProvider, &result.VisualModel,
		&referenceIDsJSON, &transcriptJSON, &visualJSON, &result.LastError,
		&result.CreatedAt, &result.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.VideoRecordingDetail{}, service.ErrNotFound
	}
	if err != nil {
		return service.VideoRecordingDetail{}, fmt.Errorf("get video recording: %w", err)
	}
	if chunkIndex.Valid {
		value := int(chunkIndex.Int64)
		result.ChunkIndex = &value
	}
	if err := json.Unmarshal(referenceIDsJSON, &result.SpeakerReferenceIDs); err != nil {
		return service.VideoRecordingDetail{}, fmt.Errorf("decode video speaker references: %w", err)
	}
	if len(transcriptJSON) > 0 {
		var transcript service.Transcript
		if err := json.Unmarshal(transcriptJSON, &transcript); err != nil {
			return service.VideoRecordingDetail{}, fmt.Errorf("decode video transcript: %w", err)
		}
		result.Transcript = &transcript
	}
	if len(visualJSON) > 0 {
		var analysis service.VisualAnalysis
		if err := json.Unmarshal(visualJSON, &analysis); err != nil {
			return service.VideoRecordingDetail{}, fmt.Errorf("decode visual analysis: %w", err)
		}
		result.VisualAnalysis = &analysis
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT id, bucket_index, start_time, end_time, description,
       visual_description, speech_description, location, confidence,
       status, memograph_response, last_error
FROM video_episodes WHERE recording_id = $1 ORDER BY bucket_index`, id)
	if err != nil {
		return service.VideoRecordingDetail{}, fmt.Errorf("list video episodes: %w", err)
	}
	defer rows.Close()
	result.Episodes = make([]service.VideoEpisode, 0)
	for rows.Next() {
		var episode service.VideoEpisode
		var confidence sql.NullFloat64
		var responseJSON []byte
		if err := rows.Scan(
			&episode.ID, &episode.BucketIndex, &episode.StartTime, &episode.EndTime,
			&episode.Description, &episode.VisualDescription,
			&episode.SpeechDescription, &episode.Location, &confidence, &episode.Status,
			&responseJSON, &episode.LastError,
		); err != nil {
			return service.VideoRecordingDetail{}, fmt.Errorf("scan video episode: %w", err)
		}
		if confidence.Valid {
			episode.Confidence = &confidence.Float64
		}
		if len(responseJSON) > 0 {
			episode.Response = json.RawMessage(responseJSON)
		}
		result.Episodes = append(result.Episodes, episode)
	}
	if err := rows.Err(); err != nil {
		return service.VideoRecordingDetail{}, fmt.Errorf("iterate video episodes: %w", err)
	}
	return result, nil
}

func (r *videoRepository) CreateVideoRealtimeSession(
	ctx context.Context,
	input service.StartVideoRealtimeSessionInput,
) (service.RealtimeVideoSession, error) {
	const query = `
WITH generated AS (SELECT gen_random_uuid()::text AS id)
INSERT INTO video_realtime_sessions (
	id, owner_user_id, memory_id, group_id, device_id, location,
	chunk_duration_seconds, frame_interval_seconds
)
SELECT id, $1, $2,
       COALESCE(NULLIF($3, ''), 'account-owner:' || $1::text),
       $4, $5, $6, $7
FROM generated
RETURNING id, memory_id, group_id, device_id, location, chunk_duration_seconds,
          frame_interval_seconds, next_chunk_index, status, created_at, updated_at, stopped_at`
	var result service.RealtimeVideoSession
	var stoppedAt sql.NullTime
	err := r.db.QueryRowContext(
		ctx, query, input.OwnerUserID, input.MemoryID, input.GroupID,
		input.DeviceID, input.Location, input.ChunkDurationSeconds,
		input.FrameIntervalSeconds,
	).Scan(
		&result.ID, &result.MemoryID, &result.GroupID, &result.DeviceID,
		&result.Location, &result.ChunkDurationSeconds, &result.FrameIntervalSeconds,
		&result.NextChunkIndex, &result.Status, &result.CreatedAt, &result.UpdatedAt,
		&stoppedAt,
	)
	if err != nil {
		return service.RealtimeVideoSession{}, fmt.Errorf("create realtime video session: %w", err)
	}
	return result, nil
}

func (r *videoRepository) GetVideoRealtimeSession(
	ctx context.Context,
	id, ownerUserID string,
) (service.RealtimeVideoSessionDetail, error) {
	const sessionQuery = `
SELECT id, memory_id, group_id, device_id, location, chunk_duration_seconds,
       frame_interval_seconds, next_chunk_index, status, created_at, updated_at, stopped_at
FROM video_realtime_sessions WHERE id = $1 AND owner_user_id = $2`
	var result service.RealtimeVideoSessionDetail
	var stoppedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, sessionQuery, id, ownerUserID).Scan(
		&result.ID, &result.MemoryID, &result.GroupID, &result.DeviceID,
		&result.Location, &result.ChunkDurationSeconds, &result.FrameIntervalSeconds,
		&result.NextChunkIndex, &result.Status, &result.CreatedAt, &result.UpdatedAt,
		&stoppedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.RealtimeVideoSessionDetail{}, service.ErrNotFound
	}
	if err != nil {
		return service.RealtimeVideoSessionDetail{}, fmt.Errorf("get realtime video session: %w", err)
	}
	if stoppedAt.Valid {
		result.StoppedAt = &stoppedAt.Time
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT id, session_id, group_id, memory_id, status, audio_status, visual_status,
       merge_status, file_name, media_type, size_bytes, client_chunk_id,
       chunk_index, start_offset_seconds, is_final, created_at
FROM video_recordings
WHERE owner_user_id = $1 AND realtime_session_id = $2
ORDER BY chunk_index`, ownerUserID, id)
	if err != nil {
		return service.RealtimeVideoSessionDetail{}, fmt.Errorf("list realtime video chunks: %w", err)
	}
	defer rows.Close()
	result.Progress.LatestChunkIndex = -1
	result.Chunks = make([]service.VideoRecording, 0)
	for rows.Next() {
		var recording service.VideoRecording
		var chunkIndex int
		if err := rows.Scan(
			&recording.ID, &recording.SessionID, &recording.GroupID, &recording.MemoryID,
			&recording.Status, &recording.AudioStatus, &recording.VisualStatus,
			&recording.MergeStatus, &recording.FileName, &recording.MediaType,
			&recording.SizeBytes, &recording.ClientChunkID, &chunkIndex,
			&recording.StartTime, &recording.IsFinal, &recording.CreatedAt,
		); err != nil {
			return service.RealtimeVideoSessionDetail{}, fmt.Errorf("scan realtime video chunk: %w", err)
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
		case "processing", "merging", "memograph_pending":
			result.Progress.Processing++
		case "completed":
			result.Progress.Completed++
		case "failed":
			result.Progress.Failed++
		}
	}
	if err := rows.Err(); err != nil {
		return service.RealtimeVideoSessionDetail{}, fmt.Errorf("iterate realtime video chunks: %w", err)
	}
	return result, nil
}

func (r *videoRepository) StopVideoRealtimeSession(
	ctx context.Context,
	id, ownerUserID string,
) (service.RealtimeVideoSession, error) {
	const query = `
UPDATE video_realtime_sessions
SET status = 'stopped', stopped_at = COALESCE(stopped_at, NOW()), updated_at = NOW()
WHERE id = $1 AND owner_user_id = $2
RETURNING id, memory_id, group_id, device_id, location, chunk_duration_seconds,
          frame_interval_seconds, next_chunk_index, status, created_at, updated_at, stopped_at`
	var result service.RealtimeVideoSession
	var stoppedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, id, ownerUserID).Scan(
		&result.ID, &result.MemoryID, &result.GroupID, &result.DeviceID,
		&result.Location, &result.ChunkDurationSeconds, &result.FrameIntervalSeconds,
		&result.NextChunkIndex, &result.Status, &result.CreatedAt, &result.UpdatedAt,
		&stoppedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.RealtimeVideoSession{}, service.ErrNotFound
	}
	if err != nil {
		return service.RealtimeVideoSession{}, fmt.Errorf("stop realtime video session: %w", err)
	}
	if stoppedAt.Valid {
		result.StoppedAt = &stoppedAt.Time
	}
	return result, nil
}

func (r *videoRepository) ClaimVideoJob(ctx context.Context) (service.VideoJob, bool, error) {
	const query = `
WITH candidate AS (
	SELECT id FROM video_jobs
	WHERE (status = 'queued' AND run_at <= NOW())
	   OR (status = 'processing' AND locked_at < NOW() - INTERVAL '10 minutes')
	ORDER BY run_at, id
	FOR UPDATE SKIP LOCKED
	LIMIT 1
), claimed AS (
	UPDATE video_jobs j
	SET status = 'processing', attempts = attempts + 1, locked_at = NOW(), updated_at = NOW()
	FROM candidate c WHERE j.id = c.id
	RETURNING j.*
), target AS (
	SELECT c.*, COALESCE(c.recording_id, e.recording_id) AS target_recording_id
	FROM claimed c
	LEFT JOIN video_episodes e ON e.id = c.episode_id
), recording_status AS (
	UPDATE video_recordings r
	SET status = CASE
			WHEN target.kind = 'merge' THEN 'merging'
			WHEN target.kind IN ('audio', 'visual') THEN 'processing'
			ELSE r.status END,
		audio_status = CASE WHEN target.kind = 'audio' THEN 'processing' ELSE r.audio_status END,
		visual_status = CASE WHEN target.kind = 'visual' THEN 'processing' ELSE r.visual_status END,
		merge_status = CASE WHEN target.kind = 'merge' THEN 'processing' ELSE r.merge_status END,
		updated_at = NOW()
	FROM target WHERE r.id = target.target_recording_id
), episode_status AS (
	UPDATE video_episodes e SET status = 'writing', updated_at = NOW()
	FROM target WHERE e.id = target.episode_id
)
SELECT target.id, target.kind, COALESCE(target.recording_id, ''),
       COALESCE(target.episode_id, ''), target.source, r.owner_user_id,
       target.attempts, target.max_attempts,
       r.file_path, r.file_name, r.media_type, r.session_id, r.group_id, r.memory_id,
       r.device_id, COALESCE(NULLIF(e.location, ''), r.location),
       COALESCE(r.client_chunk_id, ''),
       r.start_offset_seconds, r.frame_interval_seconds,
       r.transcript, r.visual_analysis, COALESCE(e.description, ''),
	   COALESCE(e.visual_description, ''), COALESCE(e.speech_description, ''),
	   COALESCE(e.start_time, 0), COALESCE(e.end_time, 0),
	   COALESCE(e.confidence, r.default_confidence), COALESCE(e.graph_revision, 0)
FROM target
JOIN video_recordings r ON r.id = target.target_recording_id
LEFT JOIN video_episodes e ON e.id = target.episode_id`
	var job service.VideoJob
	var transcriptJSON, visualJSON []byte
	var confidence sql.NullFloat64
	err := r.db.QueryRowContext(ctx, query).Scan(
		&job.ID, &job.Kind, &job.RecordingID, &job.EpisodeID,
		&job.MemographSource, &job.OwnerUserID,
		&job.Attempts, &job.MaxAttempts, &job.FilePath, &job.FileName,
		&job.MediaType, &job.SessionID, &job.GroupID, &job.MemoryID,
		&job.DeviceID, &job.Location, &job.ClientChunkID,
		&job.StartOffset, &job.FrameInterval,
		&transcriptJSON, &visualJSON, &job.Description,
		&job.VisualDescription, &job.SpeechDescription, &job.EpisodeStart,
		&job.EpisodeEnd, &confidence, &job.GraphRevision,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.VideoJob{}, false, nil
	}
	if err != nil {
		return service.VideoJob{}, false, fmt.Errorf("claim video job: %w", err)
	}
	if len(transcriptJSON) > 0 {
		if err := json.Unmarshal(transcriptJSON, &job.Transcript); err != nil {
			return service.VideoJob{}, false, fmt.Errorf("decode claimed video transcript: %w", err)
		}
	}
	if len(visualJSON) > 0 {
		if err := json.Unmarshal(visualJSON, &job.VisualAnalysis); err != nil {
			return service.VideoJob{}, false, fmt.Errorf("decode claimed visual analysis: %w", err)
		}
	}
	if confidence.Valid {
		job.Confidence = &confidence.Float64
	}
	return job, true, nil
}

func (r *videoRepository) SaveVideoTranscript(
	ctx context.Context,
	job service.VideoJob,
	transcript service.Transcript,
	referenceIDs []string,
	provider, model string,
	maxAttempts int,
) error {
	payload, err := json.Marshal(transcript)
	if err != nil {
		return fmt.Errorf("encode video transcript: %w", err)
	}
	referencePayload, err := json.Marshal(referenceIDs)
	if err != nil {
		return fmt.Errorf("encode video speaker references: %w", err)
	}
	const query = `
WITH updated AS (
	UPDATE video_recordings
	SET transcript = $2::jsonb, stt_provider = $3, stt_model = $4,
	    speaker_reference_ids = $5::jsonb,
	    audio_status = 'completed',
	    merge_status = CASE WHEN visual_status = 'completed' THEN 'queued' ELSE merge_status END,
	    status = CASE WHEN visual_status = 'completed' THEN 'merging' ELSE 'processing' END,
	    last_error = '', updated_at = NOW()
	WHERE id = $1
	RETURNING id, visual_status
), merge_job AS (
	INSERT INTO video_jobs (kind, recording_id, max_attempts)
	SELECT 'merge', id, $6 FROM updated WHERE visual_status = 'completed'
	ON CONFLICT (recording_id, kind) WHERE recording_id IS NOT NULL DO NOTHING
)
UPDATE video_jobs SET status = 'completed', locked_at = NULL, updated_at = NOW()
WHERE id = $7`
	if _, err := r.db.ExecContext(
		ctx, query, job.RecordingID, payload, provider, model,
		referencePayload, maxAttempts, job.ID,
	); err != nil {
		return fmt.Errorf("save video transcript: %w", err)
	}
	return nil
}

func (r *videoRepository) SaveVideoAnalysis(
	ctx context.Context,
	job service.VideoJob,
	analysis service.VisualAnalysis,
	provider, model string,
	maxAttempts int,
) error {
	payload, err := json.Marshal(analysis)
	if err != nil {
		return fmt.Errorf("encode visual analysis: %w", err)
	}
	const query = `
WITH updated AS (
	UPDATE video_recordings
	SET visual_analysis = $2::jsonb, visual_provider = $3, visual_model = $4,
	    visual_status = 'completed',
	    merge_status = CASE WHEN audio_status = 'completed' THEN 'queued' ELSE merge_status END,
	    status = CASE WHEN audio_status = 'completed' THEN 'merging' ELSE 'processing' END,
	    last_error = '', updated_at = NOW()
	WHERE id = $1
	RETURNING id, audio_status
), merge_job AS (
	INSERT INTO video_jobs (kind, recording_id, max_attempts)
	SELECT 'merge', id, $5 FROM updated WHERE audio_status = 'completed'
	ON CONFLICT (recording_id, kind) WHERE recording_id IS NOT NULL DO NOTHING
)
UPDATE video_jobs SET status = 'completed', locked_at = NULL, updated_at = NOW()
WHERE id = $6`
	if _, err := r.db.ExecContext(
		ctx, query, job.RecordingID, payload, provider, model, maxAttempts, job.ID,
	); err != nil {
		return fmt.Errorf("save visual analysis: %w", err)
	}
	return nil
}

func (r *videoRepository) SaveVideoEpisodes(
	ctx context.Context,
	job service.VideoJob,
	episodes []service.VideoEpisodeDraft,
	maxAttempts int,
) error {
	payload, err := json.Marshal(episodes)
	if err != nil {
		return fmt.Errorf("encode video episodes: %w", err)
	}
	const query = `
WITH episode_rows AS (
	SELECT * FROM jsonb_to_recordset($2::jsonb) AS x(
		bucket_index integer, start_time double precision, end_time double precision,
		description text, visual_description text, speech_description text,
		location text, confidence double precision,
		visual_observations jsonb
	)
), inserted AS (
	INSERT INTO video_episodes (
		recording_id, bucket_index, start_time, end_time, description,
		visual_description, speech_description, location, confidence,
		visual_observations
	)
	SELECT $1, bucket_index, start_time, end_time, description,
	       visual_description, speech_description, location, confidence,
	       visual_observations
	FROM episode_rows
	ON CONFLICT (recording_id, bucket_index) DO UPDATE
	SET start_time = EXCLUDED.start_time, end_time = EXCLUDED.end_time,
	    description = EXCLUDED.description, location = EXCLUDED.location,
	    visual_description = EXCLUDED.visual_description,
	    speech_description = EXCLUDED.speech_description,
	    confidence = EXCLUDED.confidence,
	    visual_observations = EXCLUDED.visual_observations, updated_at = NOW()
	RETURNING id, description, visual_description, speech_description
), memograph_branches AS (
	SELECT inserted.id AS episode_id, branch.source
	FROM inserted
	CROSS JOIN LATERAL (VALUES
		('visual', NULLIF(BTRIM(inserted.visual_description), '')),
		('speech', NULLIF(BTRIM(inserted.speech_description), '')),
		('legacy', CASE
			WHEN BTRIM(inserted.visual_description) = ''
			 AND BTRIM(inserted.speech_description) = ''
			THEN NULLIF(BTRIM(inserted.description), '')
			ELSE NULL
		END)
	) AS branch(source, data)
	WHERE branch.data IS NOT NULL
), memograph_jobs AS (
	INSERT INTO video_jobs (kind, episode_id, source, max_attempts)
	SELECT 'memograph', episode_id, source, $3 FROM memograph_branches
	ON CONFLICT (episode_id, source) WHERE kind = 'memograph' DO NOTHING
), completed_job AS (
	UPDATE video_jobs SET status = 'completed', locked_at = NULL, updated_at = NOW()
	WHERE id = $4
), completed_recording AS (
	UPDATE video_recordings
	SET status = 'memograph_pending', merge_status = 'completed',
	    last_error = '', updated_at = NOW()
	WHERE id = $1
	RETURNING media_asset_id
)
UPDATE media_assets a
SET status = 'completed', updated_at = NOW()
FROM completed_recording r
WHERE a.id = r.media_asset_id`
	if _, err := r.db.ExecContext(ctx, query, job.RecordingID, payload, maxAttempts, job.ID); err != nil {
		return fmt.Errorf("save merged video episodes: %w", err)
	}
	return nil
}

func (r *videoRepository) CompleteVideoMemographBranch(
	ctx context.Context,
	job service.VideoJob,
	response json.RawMessage,
) error {
	const query = `
WITH completed_job AS (
	UPDATE video_jobs SET status = 'completed', locked_at = NULL, updated_at = NOW()
	WHERE id = $1 AND episode_id = $2 AND source = $3
	RETURNING episode_id
), completed_episode AS (
	UPDATE video_episodes e
	SET memograph_response = COALESCE(e.memograph_response, '{}'::jsonb) ||
	        jsonb_build_object($3, $4::jsonb),
	    status = CASE
	      WHEN EXISTS (
		SELECT 1 FROM video_jobs failed
		WHERE failed.episode_id = e.id AND failed.kind = 'memograph'
		  AND failed.status = 'dead'
	      ) THEN 'failed'
	      WHEN NOT EXISTS (
		SELECT 1 FROM video_jobs pending
		WHERE pending.episode_id = e.id AND pending.kind = 'memograph'
		  AND pending.id <> $1
		  AND pending.status <> 'completed'
	      ) THEN 'completed'
	      ELSE 'queued'
	    END,
	    last_error = CASE WHEN EXISTS (
		SELECT 1 FROM video_jobs failed
		WHERE failed.episode_id = e.id AND failed.kind = 'memograph'
		  AND failed.status = 'dead'
	    ) THEN e.last_error ELSE '' END,
	    updated_at = NOW()
	FROM completed_job
	WHERE e.id = completed_job.episode_id
	RETURNING e.id, e.recording_id, e.status
)
UPDATE video_recordings r
	SET status = CASE
	  WHEN completed_episode.status = 'failed' THEN 'failed'
	  WHEN completed_episode.status = 'completed' AND NOT EXISTS (
		SELECT 1 FROM video_episodes e
		WHERE e.recording_id = r.id AND e.id <> completed_episode.id
		  AND e.status <> 'completed'
	  ) THEN 'completed'
	  ELSE 'memograph_pending'
	END,
	last_error = CASE WHEN completed_episode.status = 'failed' THEN r.last_error ELSE '' END,
	updated_at = NOW()
FROM completed_episode WHERE r.id = completed_episode.recording_id`
	if _, err := r.db.ExecContext(
		ctx, query, job.ID, job.EpisodeID, job.MemographSource, []byte(response),
	); err != nil {
		return fmt.Errorf("complete video Memograph branch: %w", err)
	}
	return nil
}

func (r *videoRepository) RetryVideoJob(
	ctx context.Context,
	job service.VideoJob,
	cause string,
	runAt time.Time,
	dead bool,
) error {
	jobStatus := "queued"
	if dead {
		jobStatus = "dead"
	}
	if job.Kind == "memograph" {
		episodeStatus := "queued"
		if dead {
			episodeStatus = "failed"
		}
		const query = `
WITH updated_job AS (
	UPDATE video_jobs
	SET status = $2, run_at = $3, locked_at = NULL,
	    last_error = $4, updated_at = NOW()
	WHERE id = $1 AND episode_id = $5
	RETURNING episode_id
), updated_episode AS (
	UPDATE video_episodes e
	SET status = $6, last_error = $4, updated_at = NOW()
	FROM updated_job
	WHERE e.id = updated_job.episode_id
	RETURNING e.recording_id
)
UPDATE video_recordings r
SET status = CASE WHEN $7 THEN 'failed' ELSE r.status END,
	last_error = CASE WHEN $7 THEN $4 ELSE r.last_error END,
	updated_at = NOW()
FROM updated_episode WHERE r.id = updated_episode.recording_id`
		if _, err := r.db.ExecContext(
			ctx, query, job.ID, jobStatus, runAt, cause, job.EpisodeID,
			episodeStatus, dead,
		); err != nil {
			return fmt.Errorf("retry video Memograph branch: %w", err)
		}
		return nil
	}
	if _, err := r.db.ExecContext(ctx, `
UPDATE video_jobs
SET status = $2, run_at = $3, locked_at = NULL, last_error = $4, updated_at = NOW()
WHERE id = $1`, job.ID, jobStatus, runAt, cause); err != nil {
		return fmt.Errorf("retry video job: %w", err)
	}

	componentColumn := map[string]string{
		"audio":  "audio_status",
		"visual": "visual_status",
		"merge":  "merge_status",
	}[job.Kind]
	if componentColumn == "" {
		return fmt.Errorf("unsupported video job kind %q", job.Kind)
	}
	componentStatus := "queued"
	recordingStatus := "queued"
	if dead {
		componentStatus = "failed"
		recordingStatus = "failed"
	}
	query := fmt.Sprintf(`
UPDATE video_recordings
SET %s = $2, status = $3, last_error = $4, updated_at = NOW()
WHERE id = $1`, componentColumn)
	if _, err := r.db.ExecContext(
		ctx, query, job.RecordingID, componentStatus, recordingStatus, cause,
	); err != nil {
		return fmt.Errorf("update failed video component: %w", err)
	}
	return nil
}

var _ VideoRepository = (*videoRepository)(nil)
