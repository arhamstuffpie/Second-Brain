package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dbsqlc "github.com/arham/ai-second-brain/internal/db/sqlc"
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

func (r *voiceRepository) CreateEnrollmentSample(
	ctx context.Context,
	input service.CreateEnrollmentSampleInput,
) (service.VoiceEnrollmentRecord, error) {
	params := dbsqlc.CreateVoiceEnrollmentSampleParams{
		OwnerUserID: input.OwnerUserID, ProviderLabel: input.ProviderLabel,
		FileName: input.FileName, FilePath: input.FilePath, MediaType: input.MediaType,
		SizeBytes: input.SizeBytes, DurationSeconds: input.DurationSeconds,
	}
	for attempt := 0; attempt < 4; attempt++ {
		row, err := r.queries.CreateVoiceEnrollmentSample(ctx, params)
		if err == nil {
			return enrollmentRecord(row), nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return service.VoiceEnrollmentRecord{}, fmt.Errorf("create voice enrollment sample: %w", err)
		}
	}
	return service.VoiceEnrollmentRecord{}, service.ErrConflict
}

func (r *voiceRepository) ListEnrollmentSamples(
	ctx context.Context,
	ownerUserID string,
) ([]service.VoiceEnrollmentRecord, error) {
	rows, err := r.queries.ListVoiceEnrollmentSamples(ctx, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("list voice enrollment samples: %w", err)
	}
	result := make([]service.VoiceEnrollmentRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, enrollmentRecord(row))
	}
	return result, nil
}

func (r *voiceRepository) GetEnrollmentSample(
	ctx context.Context,
	id, ownerUserID string,
) (service.VoiceEnrollmentRecord, error) {
	row, err := r.queries.GetVoiceEnrollmentSample(ctx, dbsqlc.GetVoiceEnrollmentSampleParams{
		ID: id, OwnerUserID: ownerUserID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return service.VoiceEnrollmentRecord{}, service.ErrNotFound
	}
	if err != nil {
		return service.VoiceEnrollmentRecord{}, fmt.Errorf("get voice enrollment sample: %w", err)
	}
	return enrollmentRecord(row), nil
}

func (r *voiceRepository) DeleteEnrollmentSample(
	ctx context.Context,
	id, ownerUserID string,
) (service.VoiceEnrollmentRecord, error) {
	row, err := r.queries.DeleteVoiceEnrollmentSample(ctx, dbsqlc.DeleteVoiceEnrollmentSampleParams{
		ID: id, OwnerUserID: ownerUserID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return service.VoiceEnrollmentRecord{}, service.ErrNotFound
	}
	if err != nil {
		return service.VoiceEnrollmentRecord{}, fmt.Errorf("delete voice enrollment sample: %w", err)
	}
	return enrollmentRecord(row), nil
}

func enrollmentRecord(row dbsqlc.VoiceEnrollmentSample) service.VoiceEnrollmentRecord {
	return service.VoiceEnrollmentRecord{
		ID: row.ID, OwnerUserID: row.OwnerUserID, Slot: int(row.Slot),
		ProviderLabel: row.ProviderLabel, FileName: row.FileName, FilePath: row.FilePath,
		MediaType: row.MediaType, SizeBytes: row.SizeBytes, DurationSeconds: row.DurationSeconds,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (r *voiceRepository) CreateRecording(ctx context.Context, input service.CreateRecordingInput, maxAttempts int) (service.VoiceRecording, error) {
	const query = `
WITH created_batch AS (
	INSERT INTO voice_episode_batches (
		owner_user_id, session_id, group_id, memory_id, closed
	)
	SELECT $1, $2, $3, $4, $17
	WHERE $16 = ''
	RETURNING id
), selected_batch AS (
	SELECT id FROM created_batch
	UNION ALL
	SELECT $16 WHERE $16 <> ''
), recording AS (
	INSERT INTO voice_recordings (
		owner_user_id, session_id, group_id, memory_id, device_id, location,
		file_name, file_path, media_type, size_bytes, start_offset_seconds,
		default_confidence, chunk_index, is_final, batch_id
	)
	SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,id
	FROM selected_batch
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
		input.ChunkIndex, input.IsFinal, maxAttempts, input.BatchID, input.BatchClosed,
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
       CASE WHEN EXISTS (
            SELECT 1 FROM voice_episode_recordings er WHERE er.recording_id = r.id
       )
            THEN CASE
                WHEN EXISTS (
                    SELECT 1 FROM voice_episode_recordings er
                    JOIN voice_episodes e ON e.id = er.episode_id
                    WHERE er.recording_id = r.id AND e.status = 'failed'
                ) THEN 'failed'
                WHEN NOT EXISTS (
                    SELECT 1 FROM voice_episode_recordings er
                    JOIN voice_episodes e ON e.id = er.episode_id
                    WHERE er.recording_id = r.id AND e.status <> 'completed'
                ) THEN 'completed'
                ELSE 'memograph_pending'
            END
            ELSE r.status
       END AS effective_status,
       r.file_name, r.media_type, r.size_bytes, r.chunk_index, r.is_final,
       r.device_id, r.location, r.batch_id, r.stt_provider, r.stt_model,
       r.speaker_reference_ids, r.transcript, r.last_error, r.created_at, r.updated_at
FROM voice_recordings r
WHERE r.id = $1 AND r.owner_user_id = $2`
	var result service.VoiceRecordingDetail
	var transcript, referenceIDsJSON []byte
	var chunkIndex sql.NullInt64
	err := r.db.QueryRowContext(ctx, recordingQuery, id, ownerUserID).Scan(
		&result.ID, &result.SessionID, &result.GroupID, &result.MemoryID,
		&result.Status, &result.FileName, &result.MediaType, &result.SizeBytes,
		&chunkIndex, &result.IsFinal, &result.DeviceID, &result.Location,
		&result.BatchID, &result.STTProvider, &result.STTModel, &referenceIDsJSON,
		&transcript, &result.LastError,
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
	if err := json.Unmarshal(referenceIDsJSON, &result.SpeakerReferenceIDs); err != nil {
		return service.VoiceRecordingDetail{}, fmt.Errorf("decode voice speaker reference IDs: %w", err)
	}

	const episodesQuery = `
SELECT e.id, e.bucket_index, e.episode_index, e.start_time, e.end_time,
       e.description, e.confidence, e.status, e.memograph_response, e.last_error,
       e.segments, e.source_recording_ids, e.owner_utterance_count,
       e.other_utterance_count, e.unknown_utterance_count
FROM voice_episodes e
JOIN voice_episode_recordings er ON er.episode_id = e.id
WHERE er.recording_id = $1
ORDER BY e.episode_index`
	rows, err := r.db.QueryContext(ctx, episodesQuery, id)
	if err != nil {
		return service.VoiceRecordingDetail{}, fmt.Errorf("list voice episodes: %w", err)
	}
	defer rows.Close()
	result.Episodes = make([]service.VoiceEpisode, 0)
	for rows.Next() {
		var episode service.VoiceEpisode
		var confidence sql.NullFloat64
		var response, segmentsJSON, sourceIDsJSON []byte
		if err := rows.Scan(
			&episode.ID, &episode.BucketIndex, &episode.EpisodeIndex,
			&episode.StartTime, &episode.EndTime, &episode.Description, &confidence,
			&episode.Status, &response, &episode.LastError, &segmentsJSON, &sourceIDsJSON,
			&episode.OwnerUtteranceCount, &episode.OtherUtteranceCount,
			&episode.UnknownUtteranceCount,
		); err != nil {
			return service.VoiceRecordingDetail{}, fmt.Errorf("scan voice episode: %w", err)
		}
		if confidence.Valid {
			episode.Confidence = &confidence.Float64
		}
		if len(response) > 0 {
			episode.Response = json.RawMessage(response)
		}
		if err := json.Unmarshal(segmentsJSON, &episode.Segments); err != nil {
			return service.VoiceRecordingDetail{}, fmt.Errorf("decode voice episode segments: %w", err)
		}
		if err := json.Unmarshal(sourceIDsJSON, &episode.SourceRecordingIDs); err != nil {
			return service.VoiceRecordingDetail{}, fmt.Errorf("decode voice episode recording IDs: %w", err)
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
	SELECT gen_random_uuid()::text AS session_id, gen_random_uuid()::text AS batch_id
), inserted_batch AS (
	INSERT INTO voice_episode_batches (
		id, owner_user_id, session_id, group_id, memory_id, closed
	)
	SELECT batch_id, $1, session_id,
	       COALESCE(NULLIF($3, ''), 'account-owner:' || $1::text), $2, FALSE
	FROM generated
	RETURNING id
)
INSERT INTO voice_realtime_sessions (
	id, owner_user_id, memory_id, group_id, device_id, location,
	chunk_duration_seconds, batch_id
)
SELECT g.session_id, $1, $2,
       COALESCE(NULLIF($3, ''), 'account-owner:' || $1::text),
       $4, $5, $6, b.id
FROM generated g CROSS JOIN inserted_batch b
RETURNING id, memory_id, group_id, device_id, location,
          chunk_duration_seconds, status, created_at, updated_at,
          stopped_at, batch_id`
	var session service.RealtimeVoiceSession
	var stoppedAt sql.NullTime
	err := r.db.QueryRowContext(
		ctx, query, input.OwnerUserID, input.MemoryID, input.GroupID,
		input.DeviceID, input.Location, input.ChunkDurationSeconds,
	).Scan(
		&session.ID, &session.MemoryID, &session.GroupID, &session.DeviceID,
		&session.Location, &session.ChunkDurationSeconds, &session.Status,
		&session.CreatedAt, &session.UpdatedAt, &stoppedAt, &session.BatchID,
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
       status, created_at, updated_at, stopped_at, batch_id
FROM voice_realtime_sessions
WHERE id = $1 AND owner_user_id = $2`
	var result service.RealtimeVoiceSessionDetail
	var stoppedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, sessionQuery, id, ownerUserID).Scan(
		&result.ID, &result.MemoryID, &result.GroupID, &result.DeviceID,
		&result.Location, &result.ChunkDurationSeconds, &result.Status,
		&result.CreatedAt, &result.UpdatedAt, &stoppedAt, &result.BatchID,
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
       CASE WHEN EXISTS (
            SELECT 1 FROM voice_episode_recordings er WHERE er.recording_id = r.id
       )
            THEN CASE
                WHEN EXISTS (
                    SELECT 1 FROM voice_episode_recordings er
                    JOIN voice_episodes e ON e.id = er.episode_id
                    WHERE er.recording_id = r.id AND e.status = 'failed'
                ) THEN 'failed'
                WHEN NOT EXISTS (
                    SELECT 1 FROM voice_episode_recordings er
                    JOIN voice_episodes e ON e.id = er.episode_id
                    WHERE er.recording_id = r.id AND e.status <> 'completed'
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
		case "transcribing", "assembling", "memograph_pending":
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
	result.Episodes, err = r.listEpisodesByBatch(ctx, result.BatchID)
	if err != nil {
		return service.RealtimeVoiceSessionDetail{}, err
	}
	return result, nil
}

func (r *voiceRepository) listEpisodesByBatch(
	ctx context.Context,
	batchID string,
) ([]service.VoiceEpisode, error) {
	const query = `
SELECT id, bucket_index, episode_index, start_time, end_time, description,
       confidence, status, memograph_response, last_error, segments,
       source_recording_ids, owner_utterance_count, other_utterance_count,
       unknown_utterance_count
FROM voice_episodes
WHERE batch_id = $1
ORDER BY episode_index`
	rows, err := r.db.QueryContext(ctx, query, batchID)
	if err != nil {
		return nil, fmt.Errorf("list realtime voice episodes: %w", err)
	}
	defer rows.Close()
	result := make([]service.VoiceEpisode, 0)
	for rows.Next() {
		var episode service.VoiceEpisode
		var confidence sql.NullFloat64
		var response, segmentsJSON, sourceIDsJSON []byte
		if err := rows.Scan(
			&episode.ID, &episode.BucketIndex, &episode.EpisodeIndex,
			&episode.StartTime, &episode.EndTime, &episode.Description,
			&confidence, &episode.Status, &response, &episode.LastError,
			&segmentsJSON, &sourceIDsJSON, &episode.OwnerUtteranceCount,
			&episode.OtherUtteranceCount, &episode.UnknownUtteranceCount,
		); err != nil {
			return nil, fmt.Errorf("scan realtime voice episode: %w", err)
		}
		if confidence.Valid {
			episode.Confidence = &confidence.Float64
		}
		if len(response) > 0 {
			episode.Response = json.RawMessage(response)
		}
		if err := json.Unmarshal(segmentsJSON, &episode.Segments); err != nil {
			return nil, fmt.Errorf("decode realtime voice episode segments: %w", err)
		}
		if err := json.Unmarshal(sourceIDsJSON, &episode.SourceRecordingIDs); err != nil {
			return nil, fmt.Errorf("decode realtime voice episode recording IDs: %w", err)
		}
		result = append(result, episode)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate realtime voice episodes: %w", err)
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
          status, created_at, updated_at, stopped_at, batch_id`
	var session service.RealtimeVoiceSession
	var stoppedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, id, ownerUserID).Scan(
		&session.ID, &session.MemoryID, &session.GroupID, &session.DeviceID,
		&session.Location, &session.ChunkDurationSeconds, &session.Status,
		&session.CreatedAt, &session.UpdatedAt, &stoppedAt, &session.BatchID,
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
	if _, err := r.db.ExecContext(ctx, `
WITH closed_batch AS (
	UPDATE voice_episode_batches
	SET closed = TRUE, transcript_revision = transcript_revision + 1, updated_at = NOW()
	WHERE id = $1
)
UPDATE voice_jobs
SET status = CASE WHEN status = 'processing' THEN 'processing' ELSE 'queued' END,
    attempts = CASE WHEN status = 'processing' THEN attempts ELSE 0 END,
    run_at = NOW(), locked_at = CASE WHEN status = 'processing' THEN locked_at ELSE NULL END,
    updated_at = NOW()
WHERE kind = 'assemble' AND batch_id = $1`, session.BatchID); err != nil {
		return service.RealtimeVoiceSession{}, fmt.Errorf("close realtime voice episode batch: %w", err)
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
), recording_status AS (
	UPDATE voice_recordings r
	SET status = CASE WHEN c.kind = 'stt' THEN 'transcribing' ELSE r.status END,
	    updated_at = NOW()
	FROM claimed c
	LEFT JOIN voice_episodes e ON e.id = c.episode_id
	WHERE r.id = COALESCE(c.recording_id, e.recording_id)
), episode_status AS (
	UPDATE voice_episodes e
	SET status = 'writing', updated_at = NOW()
	FROM claimed c
	WHERE e.id = c.episode_id
)
SELECT c.id, c.kind, COALESCE(c.recording_id, ''), COALESCE(c.episode_id, ''),
       c.attempts, c.max_attempts,
       COALESCE(r.file_path, ''), COALESCE(r.file_name, ''), COALESCE(r.media_type, ''),
       b.session_id, b.group_id, b.memory_id,
       COALESCE(r.device_id, ''), COALESCE(r.location, ''), COALESCE(r.start_offset_seconds, 0),
	   COALESCE(e.description, ''), COALESCE(e.start_time, 0), COALESCE(e.end_time, 0),
	   COALESCE(e.confidence, r.default_confidence), b.id, b.owner_user_id, b.closed,
	   COALESCE(e.segments, '[]'::jsonb), COALESCE(e.source_recording_ids, '[]'::jsonb),
	   COALESCE(e.owner_utterance_count, 0), COALESCE(e.other_utterance_count, 0),
	   COALESCE(e.unknown_utterance_count, 0), COALESCE(e.graph_revision, 0)
FROM claimed c
LEFT JOIN voice_episodes e ON e.id = c.episode_id
LEFT JOIN voice_recordings r ON r.id = COALESCE(c.recording_id, e.recording_id)
JOIN voice_episode_batches b ON b.id = COALESCE(c.batch_id, e.batch_id, r.batch_id)`
	var job service.VoiceJob
	var confidence sql.NullFloat64
	var segmentsJSON, sourceIDsJSON []byte
	err := r.db.QueryRowContext(ctx, query).Scan(
		&job.ID, &job.Kind, &job.RecordingID, &job.EpisodeID,
		&job.Attempts, &job.MaxAttempts, &job.FilePath, &job.FileName,
		&job.MediaType, &job.SessionID, &job.GroupID, &job.MemoryID,
		&job.DeviceID, &job.Location, &job.StartOffset,
		&job.Description, &job.EpisodeStart, &job.EpisodeEnd, &confidence,
		&job.BatchID, &job.OwnerUserID, &job.BatchClosed,
		&segmentsJSON, &sourceIDsJSON, &job.OwnerUtteranceCount,
		&job.OtherUtteranceCount, &job.UnknownUtteranceCount, &job.GraphRevision,
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
	if err := json.Unmarshal(segmentsJSON, &job.EpisodeSegments); err != nil {
		return service.VoiceJob{}, false, fmt.Errorf("decode voice job episode segments: %w", err)
	}
	if err := json.Unmarshal(sourceIDsJSON, &job.SourceRecordingIDs); err != nil {
		return service.VoiceJob{}, false, fmt.Errorf("decode voice job source recording IDs: %w", err)
	}
	return job, true, nil
}

func (r *voiceRepository) SaveTranscriptAndQueueAssembly(
	ctx context.Context,
	job service.VoiceJob,
	transcript service.Transcript,
	referenceIDs []string,
	provider, model string,
	maxAttempts int,
) error {
	transcriptJSON, err := json.Marshal(transcript)
	if err != nil {
		return fmt.Errorf("encode transcript: %w", err)
	}
	referenceIDsJSON, err := json.Marshal(referenceIDs)
	if err != nil {
		return fmt.Errorf("encode speaker reference IDs: %w", err)
	}
	const query = `
WITH updated_recording AS (
	UPDATE voice_recordings
	SET transcript = $2::jsonb, stt_provider = $3, stt_model = $4,
	    speaker_reference_ids = $5::jsonb,
	    status = 'assembling', last_error = '', updated_at = NOW()
	WHERE id = $1
    RETURNING batch_id
), revised_batch AS (
	UPDATE voice_episode_batches b
	SET transcript_revision = transcript_revision + 1, updated_at = NOW()
	FROM updated_recording r
	WHERE b.id = r.batch_id
	RETURNING b.id
), assembly_job AS (
	INSERT INTO voice_jobs (kind, batch_id, max_attempts)
	SELECT 'assemble', id, $6 FROM revised_batch
	ON CONFLICT (batch_id) WHERE kind = 'assemble' DO UPDATE
	SET status = CASE WHEN voice_jobs.status = 'processing' THEN 'processing' ELSE 'queued' END,
	    attempts = CASE WHEN voice_jobs.status = 'processing' THEN voice_jobs.attempts ELSE 0 END,
	    max_attempts = EXCLUDED.max_attempts, run_at = NOW(),
	    locked_at = CASE WHEN voice_jobs.status = 'processing' THEN voice_jobs.locked_at ELSE NULL END,
	    last_error = '', updated_at = NOW()
)
UPDATE voice_jobs
SET status = 'completed', updated_at = NOW()
WHERE id = $7`
	if _, err := r.db.ExecContext(
		ctx, query, job.RecordingID, transcriptJSON, provider, model,
		referenceIDsJSON, maxAttempts, job.ID,
	); err != nil {
		return fmt.Errorf("save transcript and queue assembly: %w", err)
	}
	return nil
}

func (r *voiceRepository) LoadAssembly(
	ctx context.Context,
	job service.VoiceJob,
) (service.VoiceAssemblySnapshot, error) {
	const batchQuery = `
SELECT b.id, b.owner_user_id, b.session_id, b.group_id, b.memory_id, b.closed,
       b.transcript_revision,
       COALESCE((SELECT location FROM voice_recordings WHERE batch_id = b.id ORDER BY created_at LIMIT 1), ''),
       COALESCE((SELECT device_id FROM voice_recordings WHERE batch_id = b.id ORDER BY created_at LIMIT 1), ''),
       NOT EXISTS (
           SELECT 1 FROM voice_recordings r
           JOIN voice_jobs j ON j.recording_id = r.id AND j.kind = 'stt'
           WHERE r.batch_id = b.id AND j.status NOT IN ('completed', 'dead')
       )
FROM voice_episode_batches b
WHERE b.id = $1`
	var snapshot service.VoiceAssemblySnapshot
	if err := r.db.QueryRowContext(ctx, batchQuery, job.BatchID).Scan(
		&snapshot.BatchID, &snapshot.OwnerUserID, &snapshot.SessionID,
		&snapshot.GroupID, &snapshot.MemoryID, &snapshot.Closed,
		&snapshot.TranscriptRevision, &snapshot.Location, &snapshot.DeviceID,
		&snapshot.AllSTTTerminal,
	); errors.Is(err, sql.ErrNoRows) {
		return service.VoiceAssemblySnapshot{}, service.ErrNotFound
	} else if err != nil {
		return service.VoiceAssemblySnapshot{}, fmt.Errorf("load voice assembly batch: %w", err)
	}

	const recordingsQuery = `
SELECT id, start_offset_seconds, chunk_index, status, transcript
FROM voice_recordings
WHERE batch_id = $1 AND transcript IS NOT NULL
ORDER BY start_offset_seconds, chunk_index NULLS FIRST, created_at, id`
	rows, err := r.db.QueryContext(ctx, recordingsQuery, job.BatchID)
	if err != nil {
		return service.VoiceAssemblySnapshot{}, fmt.Errorf("load voice assembly recordings: %w", err)
	}
	defer rows.Close()
	snapshot.Recordings = make([]service.AssemblyRecording, 0)
	for rows.Next() {
		var recording service.AssemblyRecording
		var chunkIndex sql.NullInt64
		var transcriptJSON []byte
		if err := rows.Scan(
			&recording.ID, &recording.StartOffset, &chunkIndex,
			&recording.Status, &transcriptJSON,
		); err != nil {
			return service.VoiceAssemblySnapshot{}, fmt.Errorf("scan voice assembly recording: %w", err)
		}
		if chunkIndex.Valid {
			value := int(chunkIndex.Int64)
			recording.ChunkIndex = &value
		}
		if err := json.Unmarshal(transcriptJSON, &recording.Transcript); err != nil {
			return service.VoiceAssemblySnapshot{}, fmt.Errorf("decode voice assembly transcript: %w", err)
		}
		snapshot.Recordings = append(snapshot.Recordings, recording)
		end := recording.StartOffset + recording.Transcript.Duration
		if end > snapshot.Watermark {
			snapshot.Watermark = end
		}
	}
	if err := rows.Err(); err != nil {
		return service.VoiceAssemblySnapshot{}, fmt.Errorf("iterate voice assembly recordings: %w", err)
	}
	return snapshot, nil
}

func (r *voiceRepository) SaveAssembledEpisodes(
	ctx context.Context,
	job service.VoiceJob,
	snapshot service.VoiceAssemblySnapshot,
	episodes []service.EpisodeDraft,
	maxAttempts int,
) error {
	episodesJSON, err := json.Marshal(episodes)
	if err != nil {
		return fmt.Errorf("encode assembled voice episodes: %w", err)
	}
	const query = `
WITH episode_rows AS (
	SELECT * FROM jsonb_to_recordset($3::jsonb) AS x(
		bucket_index integer, episode_index integer,
		start_time double precision, end_time double precision,
		description text, confidence double precision, segments jsonb,
		source_recording_ids jsonb, owner_utterance_count integer,
		other_utterance_count integer, unknown_utterance_count integer
	)
), inserted_episodes AS (
	INSERT INTO voice_episodes (
		recording_id, batch_id, bucket_index, episode_index,
		start_time, end_time, description, confidence, segments,
		source_recording_ids, owner_utterance_count,
		other_utterance_count, unknown_utterance_count
	)
	SELECT source_recording_ids->>0, $1, bucket_index, episode_index,
	       start_time, end_time, description, confidence, segments,
	       source_recording_ids, owner_utterance_count,
	       other_utterance_count, unknown_utterance_count
	FROM episode_rows
	ON CONFLICT (batch_id, episode_index) DO NOTHING
	RETURNING id, source_recording_ids
), episode_recordings AS (
	INSERT INTO voice_episode_recordings (episode_id, recording_id)
	SELECT e.id, jsonb_array_elements_text(e.source_recording_ids)
	FROM inserted_episodes e
	ON CONFLICT DO NOTHING
	RETURNING recording_id
), memograph_jobs AS (
	INSERT INTO voice_jobs (kind, episode_id, max_attempts)
	SELECT 'memograph', id, $4 FROM inserted_episodes
	ON CONFLICT (episode_id) WHERE kind = 'memograph' DO NOTHING
), pending_recordings AS (
	UPDATE voice_recordings r
	SET status = 'memograph_pending', last_error = '', updated_at = NOW()
	WHERE r.id IN (SELECT recording_id FROM episode_recordings)
), revised_batch AS (
	UPDATE voice_episode_batches
	SET assembled_revision = GREATEST(assembled_revision, $2), updated_at = NOW()
	WHERE id = $1
	RETURNING transcript_revision
), completed_assembly AS (
	UPDATE voice_jobs
	SET status = CASE
	        WHEN (SELECT transcript_revision FROM revised_batch) > $2 THEN 'queued'
	        ELSE 'completed'
	    END,
	    attempts = CASE
	        WHEN (SELECT transcript_revision FROM revised_batch) > $2 THEN 0
	        ELSE attempts
	    END,
	    locked_at = NULL, last_error = '', updated_at = NOW()
	WHERE id = $5
)
UPDATE voice_recordings r
SET status = 'completed', last_error = '', updated_at = NOW()
WHERE r.batch_id = $1
  AND $6 = TRUE AND $7 = TRUE
  AND r.transcript IS NOT NULL
  AND NOT EXISTS (
	SELECT 1 FROM voice_episode_recordings er
	JOIN voice_episodes e ON e.id = er.episode_id
	WHERE er.recording_id = r.id AND e.status <> 'completed'
  )`
	if _, err := r.db.ExecContext(
		ctx, query, snapshot.BatchID, snapshot.TranscriptRevision, episodesJSON,
		maxAttempts, job.ID, snapshot.Closed, snapshot.AllSTTTerminal,
	); err != nil {
		return fmt.Errorf("save assembled voice episodes: %w", err)
	}
	return nil
}

func (r *voiceRepository) CompleteMemographEpisode(ctx context.Context, job service.VoiceJob, response json.RawMessage) error {
	const query = `
WITH completed_episode AS (
	UPDATE voice_episodes
	SET status = 'completed', memograph_response = $2::jsonb, last_error = '', updated_at = NOW()
	WHERE id = $1
	RETURNING id
), completed_job AS (
	UPDATE voice_jobs SET status = 'completed', updated_at = NOW() WHERE id = $3
)
UPDATE voice_recordings r
SET status = CASE WHEN NOT EXISTS (
		SELECT 1 FROM voice_episode_recordings er
		JOIN voice_episodes e ON e.id = er.episode_id
		WHERE er.recording_id = r.id AND e.status <> 'completed'
	) THEN 'completed' ELSE 'memograph_pending' END,
	last_error = '', updated_at = NOW()
WHERE r.id IN (
	SELECT recording_id FROM voice_episode_recordings WHERE episode_id = $1
)`
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
		if dead {
			if _, err := r.db.ExecContext(ctx, `
WITH revised_batch AS (
	UPDATE voice_episode_batches b
	SET transcript_revision = transcript_revision + 1, updated_at = NOW()
	FROM voice_recordings r
	WHERE r.id = $1 AND b.id = r.batch_id AND b.closed = TRUE
	RETURNING b.id
)
UPDATE voice_jobs j
SET status = CASE WHEN j.status = 'processing' THEN 'processing' ELSE 'queued' END,
    attempts = CASE WHEN j.status = 'processing' THEN j.attempts ELSE 0 END,
    run_at = NOW(),
    locked_at = CASE WHEN j.status = 'processing' THEN j.locked_at ELSE NULL END,
    updated_at = NOW()
FROM revised_batch b
WHERE j.kind = 'assemble' AND j.batch_id = b.id`, job.RecordingID); err != nil {
				return fmt.Errorf("requeue closed voice assembly after terminal STT failure: %w", err)
			}
		}
		return nil
	}
	if job.Kind == "assemble" {
		recordingStatus := "assembling"
		if dead {
			recordingStatus = "failed"
		}
		if _, err := r.db.ExecContext(ctx, `
UPDATE voice_recordings
SET status = $2, last_error = $3, updated_at = NOW()
WHERE batch_id = $1 AND transcript IS NOT NULL`, job.BatchID, recordingStatus, cause); err != nil {
			return fmt.Errorf("update failed voice assembly batch: %w", err)
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
UPDATE voice_recordings SET status = 'failed', last_error = $2, updated_at = NOW()
WHERE id IN (
	SELECT recording_id FROM voice_episode_recordings WHERE episode_id = $1
)`, job.EpisodeID, cause)
		if err != nil {
			return fmt.Errorf("update failed recording: %w", err)
		}
	}
	return nil
}

var _ VoiceRepository = (*voiceRepository)(nil)
