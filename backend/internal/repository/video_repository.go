package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
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
), analysis_run AS (
	INSERT INTO analysis_runs (
		owner_user_id,recording_id,processing_version,status,configuration_profile
	)
	SELECT owner_user_id,id,GREATEST(processing_version,requested_processing_version),
	       'queued','pipeline:v3'
	FROM recording
	RETURNING id
), stage_jobs AS (
	INSERT INTO analysis_stage_jobs (
		analysis_run_id,stage,required,status,max_attempts,depends_on
	)
	SELECT analysis_run.id,definition.stage,TRUE,'queued',$17,definition.depends_on
	FROM analysis_run
	CROSS JOIN (VALUES
		('audio_analysis', ARRAY[]::TEXT[]),
		('dense_person_tracking', ARRAY[]::TEXT[]),
		('transcription', ARRAY['audio_analysis']::TEXT[]),
		('identity_matching', ARRAY['dense_person_tracking','transcription']::TEXT[]),
		('active_speaker_fusion', ARRAY['identity_matching']::TEXT[]),
		('episode_generation', ARRAY['active_speaker_fusion']::TEXT[]),
		('graph_persistence', ARRAY['episode_generation']::TEXT[])
	) definition(stage,depends_on)
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
), analysis_run AS (
	INSERT INTO analysis_runs (
		owner_user_id,recording_id,processing_version,status,configuration_profile
	)
	SELECT owner_user_id,id,GREATEST(processing_version,requested_processing_version),
	       'queued','pipeline:v3'
	FROM recording
	RETURNING id
), stage_jobs AS (
	INSERT INTO analysis_stage_jobs (
		analysis_run_id,stage,required,status,max_attempts,depends_on
	)
	SELECT analysis_run.id,definition.stage,TRUE,'queued',$13,definition.depends_on
	FROM analysis_run
	CROSS JOIN (VALUES
		('audio_analysis', ARRAY[]::TEXT[]),
		('dense_person_tracking', ARRAY[]::TEXT[]),
		('transcription', ARRAY['audio_analysis']::TEXT[]),
		('identity_matching', ARRAY['dense_person_tracking','transcription']::TEXT[]),
		('active_speaker_fusion', ARRAY['identity_matching']::TEXT[]),
		('episode_generation', ARRAY['active_speaker_fusion']::TEXT[]),
		('graph_persistence', ARRAY['episode_generation']::TEXT[])
	) definition(stage,depends_on)
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
SELECT r.id, r.session_id, r.group_id, r.memory_id, r.status, r.audio_status, r.visual_status,
       r.merge_status, r.file_name, r.media_type, r.size_bytes, COALESCE(r.client_chunk_id, ''),
       r.chunk_index, r.start_offset_seconds, r.is_final, r.device_id, r.location,
       r.stt_provider, r.stt_model, r.visual_provider, r.visual_model,
       r.speaker_reference_ids, r.transcript, r.visual_analysis, r.last_error,
	       r.media_asset_id, COALESCE(a.actual_duration_seconds, 0), r.processing_version,
	       r.created_at, r.updated_at
FROM video_recordings r LEFT JOIN media_assets a ON a.id = r.media_asset_id
WHERE r.id = $1 AND r.owner_user_id = $2`
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
		&result.MediaAssetID, &result.ActualDuration, &result.ProcessingVersion,
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
	   status, memograph_response, last_error, evidence_kind, processing_version,
	   COALESCE(media_asset_id, ''), observation_ids, frame_ids, supporting_episode_ids
FROM video_episodes WHERE recording_id = $1 ORDER BY bucket_index`, id)
	if err != nil {
		return service.VideoRecordingDetail{}, fmt.Errorf("list video episodes: %w", err)
	}
	defer rows.Close()
	result.Episodes = make([]service.VideoEpisode, 0)
	for rows.Next() {
		var episode service.VideoEpisode
		var confidence sql.NullFloat64
		var responseJSON, observationIDsJSON, frameIDsJSON, supportingIDsJSON []byte
		if err := rows.Scan(
			&episode.ID, &episode.BucketIndex, &episode.StartTime, &episode.EndTime,
			&episode.Description, &episode.VisualDescription,
			&episode.SpeechDescription, &episode.Location, &confidence, &episode.Status,
			&responseJSON, &episode.LastError, &episode.EvidenceKind,
			&episode.ProcessingVersion, &episode.MediaAssetID,
			&observationIDsJSON, &frameIDsJSON, &supportingIDsJSON,
		); err != nil {
			return service.VideoRecordingDetail{}, fmt.Errorf("scan video episode: %w", err)
		}
		if confidence.Valid {
			episode.Confidence = &confidence.Float64
		}
		if len(responseJSON) > 0 {
			episode.Response = json.RawMessage(responseJSON)
		}
		_ = json.Unmarshal(observationIDsJSON, &episode.ObservationIDs)
		_ = json.Unmarshal(frameIDsJSON, &episode.FrameIDs)
		_ = json.Unmarshal(supportingIDsJSON, &episode.SupportingEpisodeIDs)
		result.Episodes = append(result.Episodes, episode)
	}
	if err := rows.Err(); err != nil {
		return service.VideoRecordingDetail{}, fmt.Errorf("iterate video episodes: %w", err)
	}
	batchRows, err := r.db.QueryContext(ctx, `
SELECT id, recording_id, batch_index, start_time, end_time, frames, status,
	       attempts, provider, model, COALESCE(result, '{}'::jsonb), last_error,
	       processing_version, created_at, updated_at
FROM video_analysis_batches WHERE recording_id = $1 ORDER BY processing_version, batch_index`, id)
	if err != nil {
		return service.VideoRecordingDetail{}, fmt.Errorf("list video analysis batches: %w", err)
	}
	defer batchRows.Close()
	result.AnalysisBatches = []service.VideoAnalysisBatch{}
	for batchRows.Next() {
		var batch service.VideoAnalysisBatch
		var framesJSON, resultJSON []byte
		if err := batchRows.Scan(&batch.ID, &batch.RecordingID, &batch.BatchIndex,
			&batch.StartTime, &batch.EndTime, &framesJSON, &batch.Status,
			&batch.Attempts, &batch.Provider, &batch.Model, &resultJSON,
			&batch.LastError, &batch.ProcessingVersion, &batch.CreatedAt,
			&batch.UpdatedAt); err != nil {
			return service.VideoRecordingDetail{}, err
		}
		if err := json.Unmarshal(framesJSON, &batch.Frames); err != nil {
			return service.VideoRecordingDetail{}, err
		}
		if len(resultJSON) > 2 {
			_ = json.Unmarshal(resultJSON, &batch.Result)
		}
		result.AnalysisBatches = append(result.AnalysisBatches, batch)
	}
	return result, nil
}

func (r *videoRepository) QueueVideoReprocessing(
	ctx context.Context,
	id, ownerUserID string,
) (service.VideoRecording, error) {
	const query = `
WITH reset_recording AS (
	UPDATE video_recordings r
	SET processing_version = processing_version + 1,
	    requested_processing_version = GREATEST(requested_processing_version, processing_version + 1),
	    visual_analysis = NULL, visual_provider = '', visual_model = '',
	    visual_status = 'queued', merge_status = 'waiting', status = 'processing',
	    last_error = '', updated_at = NOW()
	FROM media_assets a
	WHERE r.id = $1 AND r.owner_user_id = $2 AND a.id = r.media_asset_id
	  AND a.status NOT IN ('deleting','deleted','failed')
	RETURNING r.*
), reset_visual_job AS (
	UPDATE video_jobs j SET status = 'queued', attempts = 0, run_at = NOW(),
	    locked_at = NULL, last_error = '', updated_at = NOW()
	FROM reset_recording r WHERE j.recording_id = r.id AND j.kind = 'visual'
), analysis_run AS (
	INSERT INTO analysis_runs (
		owner_user_id,recording_id,processing_version,status,configuration_profile
	)
	SELECT owner_user_id,id,GREATEST(processing_version,requested_processing_version),
	       'queued','pipeline:v3'
	FROM reset_recording
	ON CONFLICT (recording_id,processing_version) DO UPDATE
	SET status='queued',last_error='',completed_at=NULL,updated_at=NOW()
	RETURNING id,recording_id
), stage_jobs AS (
	INSERT INTO analysis_stage_jobs (
		analysis_run_id,stage,required,status,max_attempts,depends_on
	)
	SELECT a.id,definition.stage,TRUE,'queued',COALESCE((
		SELECT MAX(j.max_attempts)
		FROM analysis_stage_jobs j
		JOIN analysis_runs old_run ON old_run.id=j.analysis_run_id
		WHERE old_run.recording_id=a.recording_id
	),5),definition.depends_on
	FROM analysis_run a
	CROSS JOIN (VALUES
		('audio_analysis', ARRAY[]::TEXT[]),
		('dense_person_tracking', ARRAY[]::TEXT[]),
		('transcription', ARRAY['audio_analysis']::TEXT[]),
		('identity_matching', ARRAY['dense_person_tracking','transcription']::TEXT[]),
		('active_speaker_fusion', ARRAY['identity_matching']::TEXT[]),
		('episode_generation', ARRAY['active_speaker_fusion']::TEXT[]),
		('graph_persistence', ARRAY['episode_generation']::TEXT[])
	) definition(stage,depends_on)
	ON CONFLICT (analysis_run_id,stage) DO UPDATE
	SET required=TRUE,status='queued',attempts=0,run_at=NOW(),locked_at=NULL,
	    depends_on=EXCLUDED.depends_on,last_error='',checkpoint='{}'::jsonb,
	    result_provenance='{}'::jsonb,updated_at=NOW()
)
SELECT id, session_id, group_id, memory_id, status, audio_status, visual_status,
       merge_status, file_name, media_type, size_bytes, COALESCE(client_chunk_id, ''),
       chunk_index, start_offset_seconds, is_final, created_at
FROM reset_recording`
	var result service.VideoRecording
	var chunkIndex sql.NullInt64
	err := r.db.QueryRowContext(ctx, query, id, ownerUserID).Scan(
		&result.ID, &result.SessionID, &result.GroupID, &result.MemoryID,
		&result.Status, &result.AudioStatus, &result.VisualStatus, &result.MergeStatus,
		&result.FileName, &result.MediaType, &result.SizeBytes, &result.ClientChunkID,
		&chunkIndex, &result.StartTime, &result.IsFinal, &result.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.VideoRecording{}, service.ErrNotFound
	}
	if err != nil {
		return service.VideoRecording{}, fmt.Errorf("queue video reprocessing: %w", err)
	}
	if chunkIndex.Valid {
		value := int(chunkIndex.Int64)
		result.ChunkIndex = &value
	}
	return result, nil
}

func (r *videoRepository) GetVideoSourceObject(
	ctx context.Context,
	id, ownerUserID string,
) (string, service.StoredObject, error) {
	const query = `
SELECT a.id, a.storage_provider, a.bucket, a.object_key, a.size_bytes, a.sha256
FROM video_recordings r JOIN media_assets a ON a.id = r.media_asset_id
WHERE r.id = $1 AND r.owner_user_id = $2 AND a.status NOT IN ('deleting','deleted','failed')`
	var assetID string
	var object service.StoredObject
	err := r.db.QueryRowContext(ctx, query, id, ownerUserID).Scan(
		&assetID, &object.Provider, &object.Bucket, &object.Key, &object.SizeBytes, &object.SHA256,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return "", service.StoredObject{}, service.ErrNotFound
	}
	return assetID, object, err
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
SELECT target.id, target.kind, target.target_recording_id,
	       COALESCE(target.episode_id, ''), target.source, COALESCE(e.source_identity, ''), r.owner_user_id,
       target.attempts, target.max_attempts,
	       r.file_path, r.file_name, r.media_type,
	       r.stt_provider, r.stt_model, r.visual_provider, r.visual_model,
	       r.session_id, r.group_id, r.memory_id,
       r.device_id, COALESCE(NULLIF(e.location, ''), r.location),
	       COALESCE(r.client_chunk_id, ''),
	       COALESCE(r.media_asset_id, ''), COALESCE(a.actual_duration_seconds, 0),
	       r.start_offset_seconds, r.frame_interval_seconds, r.processing_version,
       r.transcript, r.visual_analysis, COALESCE(e.description, ''),
	   COALESCE(e.visual_description, ''), COALESCE(e.speech_description, ''),
	   COALESCE(e.start_time, 0), COALESCE(e.end_time, 0),
	   COALESCE(e.confidence, r.default_confidence), COALESCE(e.graph_revision, 0),
	   COALESCE(e.visual_observations, '[]'::jsonb),
	   COALESCE(e.observation_ids, '[]'::jsonb), COALESCE(e.frame_ids, '[]'::jsonb),
	   COALESCE(e.supporting_episode_ids, '[]'::jsonb)
FROM target
JOIN video_recordings r ON r.id = target.target_recording_id
LEFT JOIN media_assets a ON a.id = r.media_asset_id
LEFT JOIN video_episodes e ON e.id = target.episode_id`
	var job service.VideoJob
	var transcriptJSON, visualJSON, episodeVisualJSON, observationIDsJSON, frameIDsJSON, supportingIDsJSON []byte
	var confidence sql.NullFloat64
	err := r.db.QueryRowContext(ctx, query).Scan(
		&job.ID, &job.Kind, &job.RecordingID, &job.EpisodeID,
		&job.MemographSource, &job.SourceIdentity, &job.OwnerUserID,
		&job.Attempts, &job.MaxAttempts, &job.FilePath, &job.FileName,
		&job.MediaType, &job.STTProvider, &job.STTModel,
		&job.VisualProvider, &job.VisualModel,
		&job.SessionID, &job.GroupID, &job.MemoryID,
		&job.DeviceID, &job.Location, &job.ClientChunkID,
		&job.MediaAssetID, &job.ActualDuration,
		&job.StartOffset, &job.FrameInterval, &job.ProcessingVersion,
		&transcriptJSON, &visualJSON, &job.Description,
		&job.VisualDescription, &job.SpeechDescription, &job.EpisodeStart,
		&job.EpisodeEnd, &confidence, &job.GraphRevision,
		&episodeVisualJSON, &observationIDsJSON, &frameIDsJSON, &supportingIDsJSON,
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
	if err := json.Unmarshal(episodeVisualJSON, &job.EpisodeVisual); err != nil {
		return service.VideoJob{}, false, fmt.Errorf("decode episode visual observations: %w", err)
	}
	if err := json.Unmarshal(observationIDsJSON, &job.ObservationIDs); err != nil {
		return service.VideoJob{}, false, fmt.Errorf("decode episode observation IDs: %w", err)
	}
	if err := json.Unmarshal(frameIDsJSON, &job.FrameIDs); err != nil {
		return service.VideoJob{}, false, fmt.Errorf("decode episode frame IDs: %w", err)
	}
	if err := json.Unmarshal(supportingIDsJSON, &job.SupportingEpisodeIDs); err != nil {
		return service.VideoJob{}, false, fmt.Errorf("decode supporting episode IDs: %w", err)
	}
	if confidence.Valid {
		job.Confidence = &confidence.Float64
	}
	return job, true, nil
}

func (r *videoRepository) ClaimIdentityJob(ctx context.Context) (service.VideoJob, bool, error) {
	job, found, err := r.claimDenseIdentityJob(ctx)
	if err != nil || found {
		return job, found, err
	}
	return r.claimLegacyIdentityJob(ctx)
}

func (r *videoRepository) claimDenseIdentityJob(ctx context.Context) (service.VideoJob, bool, error) {
	database, ok := r.db.(interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		return service.VideoJob{}, false, fmt.Errorf("dense identity claim requires transaction support")
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return service.VideoJob{}, false, fmt.Errorf("begin dense identity claim: %w", err)
	}
	defer tx.Rollback()
	const query = `
WITH candidate AS (
    SELECT j.id
    FROM analysis_stage_jobs j
    WHERE j.stage='identity_matching'
      AND (
        (j.status IN ('queued','retryable_failed') AND j.run_at<=NOW())
        OR (j.status='processing' AND j.locked_at<NOW()-INTERVAL '10 minutes')
      )
      AND NOT EXISTS (
        SELECT 1
        FROM unnest(j.depends_on) dependency(stage)
        LEFT JOIN analysis_stage_jobs prerequisite
          ON prerequisite.analysis_run_id=j.analysis_run_id
         AND prerequisite.stage=dependency.stage
        WHERE prerequisite.status IS DISTINCT FROM 'completed'
      )
    ORDER BY j.run_at,j.id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
), claimed AS (
    UPDATE analysis_stage_jobs j
    SET status='processing',attempts=attempts+1,locked_at=NOW(),last_error='',updated_at=NOW()
    FROM candidate c WHERE j.id=c.id
    RETURNING j.*
), updated_run AS (
    UPDATE analysis_runs a
    SET status='processing',started_at=COALESCE(started_at,NOW()),last_error='',updated_at=NOW()
    FROM claimed j WHERE a.id=j.analysis_run_id
)
SELECT j.id,a.id,a.recording_id,a.owner_user_id,j.attempts,j.max_attempts,
       r.file_path,r.file_name,r.media_type,a.processing_version,r.transcript,r.visual_analysis
FROM claimed j
JOIN analysis_runs a ON a.id=j.analysis_run_id
JOIN video_recordings r ON r.id=a.recording_id AND r.owner_user_id=a.owner_user_id`
	var job service.VideoJob
	var transcriptJSON, visualJSON []byte
	err = tx.QueryRowContext(ctx, query).Scan(
		&job.ID, &job.AnalysisRunID, &job.RecordingID, &job.OwnerUserID,
		&job.Attempts, &job.MaxAttempts, &job.FilePath, &job.FileName, &job.MediaType,
		&job.ProcessingVersion, &transcriptJSON, &visualJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.VideoJob{}, false, nil
	}
	if err != nil {
		return service.VideoJob{}, false, fmt.Errorf("claim dense identity job: %w", err)
	}
	if len(transcriptJSON) > 0 {
		if err := json.Unmarshal(transcriptJSON, &job.Transcript); err != nil {
			return service.VideoJob{}, false, fmt.Errorf("decode dense identity transcript: %w", err)
		}
	}
	if len(visualJSON) > 0 {
		if err := json.Unmarshal(visualJSON, &job.VisualAnalysis); err != nil {
			return service.VideoJob{}, false, fmt.Errorf("decode dense identity visual analysis: %w", err)
		}
	}
	tracks, err := r.loadDenseIdentityTracks(ctx, tx, job)
	if err != nil {
		return service.VideoJob{}, false, err
	}
	job.DenseIdentityTracks = tracks
	if err := tx.Commit(); err != nil {
		return service.VideoJob{}, false, fmt.Errorf("commit dense identity claim: %w", err)
	}
	return job, true, nil
}

func (r *videoRepository) loadDenseIdentityTracks(ctx context.Context, db DBTX, job service.VideoJob) ([]service.DenseIdentityTrack, error) {
	rows, err := db.QueryContext(ctx, `
SELECT t.id,t.face_track_provider_ref,t.lifecycle_status,t.first_frame,t.last_frame,
       t.start_time,t.end_time,t.observation_count,t.tracking_confidence,
       t.quality_summary,t.model_provenance,
       o.id,o.frame_index,o.observed_at_seconds,o.face_box,o.detection_score,
       o.quality,o.pose,o.embedding_model,o.mouth_visible,COALESCE(o.mouth_activity,0),
       o.gallery_selected,COALESCE(to_jsonb(e.embedding),'[]'::jsonb)
FROM person_tracks t
JOIN face_track_observations o
  ON o.owner_user_id=t.owner_user_id AND o.recording_id=t.recording_id
 AND o.person_track_id=t.id AND o.processing_version=t.processing_version
LEFT JOIN face_track_observation_embeddings e ON e.observation_id=o.id
WHERE t.owner_user_id=$1 AND t.recording_id=$2 AND t.processing_version=$3
ORDER BY t.start_time,t.id,o.observed_at_seconds,o.frame_index`,
		job.OwnerUserID, job.RecordingID, job.ProcessingVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("load dense identity tracks: %w", err)
	}
	defer rows.Close()

	tracks := make([]service.DenseIdentityTrack, 0)
	trackIndexes := make(map[string]int)
	for rows.Next() {
		var trackID, providerReference, lifecycleStatus, observationID, embeddingModel string
		var firstFrame, lastFrame, observationCount, frameIndex int
		var startTime, endTime, trackingConfidence, timestamp, detectionScore, mouthActivity float64
		var mouthVisible, gallerySelected bool
		var qualityJSON, provenanceJSON, boxJSON, observationQualityJSON, poseJSON, embeddingJSON []byte
		if err := rows.Scan(
			&trackID, &providerReference, &lifecycleStatus, &firstFrame, &lastFrame,
			&startTime, &endTime, &observationCount, &trackingConfidence,
			&qualityJSON, &provenanceJSON, &observationID, &frameIndex, &timestamp,
			&boxJSON, &detectionScore, &observationQualityJSON, &poseJSON, &embeddingModel,
			&mouthVisible, &mouthActivity, &gallerySelected, &embeddingJSON,
		); err != nil {
			return nil, fmt.Errorf("scan dense identity track: %w", err)
		}
		index, exists := trackIndexes[trackID]
		if !exists {
			var quality service.PersonTrackQuality
			var provenance service.ModelProvenance
			if err := json.Unmarshal(qualityJSON, &quality); err != nil {
				return nil, fmt.Errorf("decode dense track quality: %w", err)
			}
			if err := json.Unmarshal(provenanceJSON, &provenance); err != nil {
				return nil, fmt.Errorf("decode dense track provenance: %w", err)
			}
			provider := strings.SplitN(embeddingModel, "/", 2)[0]
			tracks = append(tracks, service.DenseIdentityTrack{
				DensePersonTrack: service.DensePersonTrack{
					ID: trackID, ProviderTrackReference: providerReference,
					LifecycleStatus: lifecycleStatus, FirstFrame: firstFrame, LastFrame: lastFrame,
					StartTime: startTime, EndTime: endTime, ObservationCount: observationCount,
					TrackingConfidence: trackingConfidence, Quality: quality,
				},
				Provider: provider, DetectorModel: provenance.DetectorModel,
				EmbeddingModel: embeddingModel,
			})
			index = len(tracks) - 1
			trackIndexes[trackID] = index
		} else if tracks[index].EmbeddingModel != embeddingModel {
			return nil, fmt.Errorf("dense identity track %q mixes embedding models", trackID)
		}
		var observation service.DenseFaceObservation
		observation.ObservationID = observationID
		observation.FrameIndex = frameIndex
		observation.Timestamp = timestamp
		observation.DetectionScore = detectionScore
		observation.MouthVisible = mouthVisible
		observation.MouthActivity = mouthActivity
		if err := json.Unmarshal(boxJSON, &observation.Box); err != nil {
			return nil, fmt.Errorf("decode dense face box: %w", err)
		}
		if err := json.Unmarshal(observationQualityJSON, &observation.Quality); err != nil {
			return nil, fmt.Errorf("decode dense face quality: %w", err)
		}
		if err := json.Unmarshal(poseJSON, &observation.Pose); err != nil {
			return nil, fmt.Errorf("decode dense face pose: %w", err)
		}
		if gallerySelected {
			if err := json.Unmarshal(embeddingJSON, &observation.Embedding); err != nil {
				return nil, fmt.Errorf("decode dense face embedding: %w", err)
			}
			if len(observation.Embedding) == 0 {
				return nil, fmt.Errorf("dense gallery observation %q has no embedding", observationID)
			}
			tracks[index].GalleryObservationIDs = append(tracks[index].GalleryObservationIDs, observationID)
		}
		tracks[index].Observations = append(tracks[index].Observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dense identity tracks: %w", err)
	}
	return tracks, nil
}

func (r *videoRepository) claimLegacyIdentityJob(ctx context.Context) (service.VideoJob, bool, error) {
	const query = `
WITH candidate AS (
    SELECT id FROM temporal_jobs
    WHERE (status='queued' AND run_at<=NOW())
       OR (status='processing' AND locked_at<NOW()-INTERVAL '10 minutes')
    ORDER BY run_at,id FOR UPDATE SKIP LOCKED LIMIT 1
), claimed AS (
    UPDATE temporal_jobs job SET status='processing',attempts=attempts+1,
        locked_at=NOW(),updated_at=NOW()
    FROM candidate WHERE job.id=candidate.id RETURNING job.*
)
SELECT claimed.id,claimed.recording_id,recording.owner_user_id,
       claimed.attempts,claimed.max_attempts,recording.file_path,recording.file_name,
       recording.media_type,recording.processing_version,recording.transcript,recording.visual_analysis
FROM claimed JOIN video_recordings recording ON recording.id=claimed.recording_id`
	var job service.VideoJob
	var transcriptJSON, visualJSON []byte
	err := r.db.QueryRowContext(ctx, query).Scan(
		&job.ID, &job.RecordingID, &job.OwnerUserID, &job.Attempts, &job.MaxAttempts,
		&job.FilePath, &job.FileName, &job.MediaType, &job.ProcessingVersion,
		&transcriptJSON, &visualJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.VideoJob{}, false, nil
	}
	if err != nil {
		return service.VideoJob{}, false, fmt.Errorf("claim identity job: %w", err)
	}
	if err := json.Unmarshal(transcriptJSON, &job.Transcript); err != nil {
		return service.VideoJob{}, false, fmt.Errorf("decode identity transcript: %w", err)
	}
	if err := json.Unmarshal(visualJSON, &job.VisualAnalysis); err != nil {
		return service.VideoJob{}, false, fmt.Errorf("decode identity visual analysis: %w", err)
	}
	return job, true, nil
}

func (r *videoRepository) CompleteIdentityJob(ctx context.Context, job service.VideoJob, warning string) error {
	if job.AnalysisRunID != "" {
		return r.completeDenseIdentityJob(ctx, job, warning)
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE temporal_jobs SET status='completed',locked_at=NULL,last_error='',warning=$2,updated_at=NOW()
WHERE id=$1`, job.ID, warning)
	if err != nil {
		return fmt.Errorf("complete identity job: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return service.ErrNotFound
	}
	return nil
}

func (r *videoRepository) completeDenseIdentityJob(ctx context.Context, job service.VideoJob, warning string) error {
	database, ok := r.db.(interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		return fmt.Errorf("dense identity completion requires transaction support")
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dense identity completion: %w", err)
	}
	defer tx.Rollback()
	visualJSON, err := json.Marshal(job.VisualAnalysis)
	if err != nil {
		return fmt.Errorf("encode dense identity visual analysis: %w", err)
	}
	checkpoint, err := json.Marshal(map[string]any{
		"track_count": len(job.DenseIdentityTracks), "warning": warning,
	})
	if err != nil {
		return fmt.Errorf("encode dense identity checkpoint: %w", err)
	}
	recordingResult, err := tx.ExecContext(ctx, `
UPDATE video_recordings SET visual_analysis=$3::jsonb,updated_at=NOW()
WHERE id=$1 AND owner_user_id=$2`, job.RecordingID, job.OwnerUserID, visualJSON)
	if err != nil {
		return fmt.Errorf("save dense identity scene mapping: %w", err)
	}
	if affected, err := recordingResult.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("dense identity recording is no longer available")
	}
	result, err := tx.ExecContext(ctx, `
UPDATE analysis_stage_jobs
SET status='completed',locked_at=NULL,checkpoint=$3::jsonb,last_error='',updated_at=NOW()
WHERE id=$1 AND analysis_run_id=$2 AND status='processing'`,
		job.ID, job.AnalysisRunID, checkpoint,
	)
	if err != nil {
		return fmt.Errorf("complete dense identity stage: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("dense identity stage is no longer claimable")
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE analysis_runs a
SET status=CASE WHEN EXISTS (
        SELECT 1 FROM analysis_stage_jobs j
        WHERE j.analysis_run_id=a.id AND j.required AND j.status<>'completed'
    ) THEN 'processing' ELSE 'completed' END,
    completed_at=CASE WHEN NOT EXISTS (
        SELECT 1 FROM analysis_stage_jobs j
        WHERE j.analysis_run_id=a.id AND j.required AND j.status<>'completed'
    ) THEN NOW() ELSE NULL END,
    last_error='',updated_at=NOW()
WHERE a.id=$1`, job.AnalysisRunID); err != nil {
		return fmt.Errorf("complete dense identity analysis run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dense identity completion: %w", err)
	}
	return nil
}

func (r *videoRepository) RetryIdentityJob(
	ctx context.Context, job service.VideoJob, cause string, runAt time.Time, dead bool,
) error {
	if job.AnalysisRunID != "" {
		status := "retryable_failed"
		if dead {
			status = "dead"
		}
		_, err := r.db.ExecContext(ctx, `
WITH updated_job AS (
    UPDATE analysis_stage_jobs
    SET status=$3,run_at=$4,locked_at=NULL,last_error=$5,updated_at=NOW()
    WHERE id=$1 AND analysis_run_id=$2 AND status='processing'
    RETURNING analysis_run_id
)
UPDATE analysis_runs a SET status=$3,last_error=$5,updated_at=NOW()
FROM updated_job j WHERE a.id=j.analysis_run_id`,
			job.ID, job.AnalysisRunID, status, runAt, cause,
		)
		if err != nil {
			return fmt.Errorf("retry dense identity job: %w", err)
		}
		return nil
	}
	status := "queued"
	if dead {
		status = "dead"
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE temporal_jobs SET status=$2,run_at=$3,locked_at=NULL,last_error=$4,updated_at=NOW()
WHERE id=$1`, job.ID, status, runAt, cause)
	if err != nil {
		return fmt.Errorf("retry identity job: %w", err)
	}
	return nil
}

func (r *videoRepository) CreateVideoAnalysisBatches(
	ctx context.Context,
	job service.VideoJob,
	durationSeconds float64,
	frames []service.VideoFrame,
) error {
	if len(frames) == 0 {
		return fmt.Errorf("create video analysis batches: no selected frames")
	}
	batches := make([]service.VideoAnalysisBatch, 0, (len(frames)+7)/8)
	for start := 0; start < len(frames); start += 8 {
		end := min(start+8, len(frames))
		batchFrames := append([]service.VideoFrame(nil), frames[start:end]...)
		batches = append(batches, service.VideoAnalysisBatch{
			BatchIndex: len(batches), StartTime: batchFrames[0].Timestamp,
			EndTime: batchFrames[len(batchFrames)-1].Timestamp,
			Frames:  batchFrames, ProcessingVersion: job.ProcessingVersion,
		})
	}
	payload, err := json.Marshal(batches)
	if err != nil {
		return fmt.Errorf("encode video analysis batches: %w", err)
	}
	const query = `
WITH rows AS (
	SELECT * FROM jsonb_to_recordset($2::jsonb) AS x(
		batch_index integer, start_time double precision, end_time double precision,
		frames jsonb, processing_version integer
	)
), inserted AS (
	INSERT INTO video_analysis_batches (
		recording_id, batch_index, start_time, end_time, frames, processing_version
	)
	SELECT $1, batch_index, start_time, end_time, frames, processing_version FROM rows
	ON CONFLICT (recording_id, batch_index, processing_version) DO NOTHING
), updated_asset AS (
	UPDATE media_assets SET actual_duration_seconds = $3, updated_at = NOW()
	WHERE id = $4
)
UPDATE video_jobs SET last_error = '', updated_at = NOW() WHERE id = $5`
	if _, err := r.db.ExecContext(ctx, query, job.RecordingID, payload, durationSeconds, job.MediaAssetID, job.ID); err != nil {
		return fmt.Errorf("create video analysis batches: %w", err)
	}
	return nil
}

func (r *videoRepository) ClaimVideoAnalysisBatch(
	ctx context.Context,
	job service.VideoJob,
) (service.VideoAnalysisBatch, bool, error) {
	const query = `
WITH candidate AS (
	SELECT id FROM video_analysis_batches
	WHERE recording_id = $1 AND processing_version = $2
	  AND status IN ('queued', 'processing')
	ORDER BY batch_index FOR UPDATE SKIP LOCKED LIMIT 1
)
UPDATE video_analysis_batches b
SET status = 'processing', attempts = attempts + 1, updated_at = NOW()
FROM candidate WHERE b.id = candidate.id
RETURNING b.id, b.recording_id, b.batch_index, b.start_time, b.end_time,
	          b.frames, b.status, b.attempts, b.provider, b.model,
	          COALESCE(b.result, '{}'::jsonb), b.last_error, b.processing_version,
	          b.created_at, b.updated_at`
	var batch service.VideoAnalysisBatch
	var framesJSON, resultJSON []byte
	err := r.db.QueryRowContext(ctx, query, job.RecordingID, job.ProcessingVersion).Scan(
		&batch.ID, &batch.RecordingID, &batch.BatchIndex, &batch.StartTime,
		&batch.EndTime, &framesJSON, &batch.Status, &batch.Attempts,
		&batch.Provider, &batch.Model, &resultJSON, &batch.LastError,
		&batch.ProcessingVersion, &batch.CreatedAt, &batch.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.VideoAnalysisBatch{}, false, nil
	}
	if err != nil {
		return service.VideoAnalysisBatch{}, false, fmt.Errorf("claim video analysis batch: %w", err)
	}
	if err := json.Unmarshal(framesJSON, &batch.Frames); err != nil {
		return service.VideoAnalysisBatch{}, false, fmt.Errorf("decode video analysis batch frames: %w", err)
	}
	return batch, true, nil
}

func (r *videoRepository) CompleteVideoAnalysisBatch(
	ctx context.Context,
	job service.VideoJob,
	batch service.VideoAnalysisBatch,
	analysis service.VisualAnalysis,
) error {
	payload, err := json.Marshal(analysis)
	if err != nil {
		return err
	}
	const query = `
UPDATE video_analysis_batches
SET status = 'completed', provider = $3, model = $4, result = $5::jsonb,
    last_error = '', updated_at = NOW()
WHERE id = $1 AND recording_id = $2`
	_, err = r.db.ExecContext(ctx, query, batch.ID, job.RecordingID, analysis.Provider, analysis.Model, payload)
	return err
}

func (r *videoRepository) RetryVideoAnalysisBatch(
	ctx context.Context,
	job service.VideoJob,
	batch service.VideoAnalysisBatch,
	cause string,
	dead bool,
) error {
	status := "queued"
	if dead {
		status = "dead"
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE video_analysis_batches SET status = $3, last_error = $4, updated_at = NOW()
WHERE id = $1 AND recording_id = $2`, batch.ID, job.RecordingID, status, cause)
	return err
}

func (r *videoRepository) FinishVideoAnalysis(
	ctx context.Context,
	job service.VideoJob,
	durationSeconds float64,
	provider, model string,
	maxAttempts int,
) (bool, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT start_time, end_time, status, COALESCE(result, '{}'::jsonb), last_error
FROM video_analysis_batches
WHERE recording_id = $1 AND processing_version = $2 ORDER BY batch_index`,
		job.RecordingID, job.ProcessingVersion)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	analysis := service.VisualAnalysis{Observations: []service.VideoObservation{}, Provider: provider, Model: model, ProcessingVersion: job.ProcessingVersion}
	warnings := make([]string, 0)
	terminal := true
	count := 0
	for rows.Next() {
		count++
		var start, end float64
		var status, lastError string
		var resultJSON []byte
		if err := rows.Scan(&start, &end, &status, &resultJSON, &lastError); err != nil {
			return false, err
		}
		switch status {
		case "completed":
			var result service.VisualAnalysis
			if err := json.Unmarshal(resultJSON, &result); err != nil {
				return false, err
			}
			analysis.Observations = append(analysis.Observations, result.Observations...)
			if strings.TrimSpace(result.Warning) != "" {
				warnings = append(warnings, result.Warning)
			}
		case "dead":
			warnings = append(warnings, fmt.Sprintf("uncovered visual range %.2fs-%.2fs: %s", start, end, lastError))
		default:
			terminal = false
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}
	if !terminal {
		_, err := r.db.ExecContext(ctx, `
UPDATE video_jobs SET status = 'queued', attempts = 0, locked_at = NULL, run_at = NOW(), updated_at = NOW()
WHERE id = $1`, job.ID)
		return false, err
	}
	analysis.Warning = strings.Join(warnings, "; ")
	sort.SliceStable(analysis.Observations, func(i, j int) bool {
		return analysis.Observations[i].StartTime < analysis.Observations[j].StartTime
	})
	if durationSeconds <= 0 {
		durationSeconds = job.ActualDuration
	}
	return true, r.SaveVideoAnalysis(ctx, job, durationSeconds, analysis, provider, model, maxAttempts)
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
	RETURNING id,owner_user_id,processing_version,visual_status
), merge_job AS (
	INSERT INTO video_jobs (kind, recording_id, max_attempts)
	SELECT 'merge', id, $6 FROM updated WHERE visual_status = 'completed'
	ON CONFLICT (recording_id, kind) WHERE recording_id IS NOT NULL DO NOTHING
), identity_job AS (
	INSERT INTO temporal_jobs (owner_user_id,recording_id,max_attempts,processing_version)
	SELECT owner_user_id,id,$6,processing_version FROM updated WHERE visual_status='completed'
	ON CONFLICT (recording_id,processing_version) DO UPDATE SET
		status='queued',attempts=0,run_at=NOW(),locked_at=NULL,last_error='',warning='',updated_at=NOW()
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
	durationSeconds float64,
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
	RETURNING id,owner_user_id,processing_version,audio_status,media_asset_id
), updated_asset AS (
	UPDATE media_assets a
	SET actual_duration_seconds = $7, updated_at = NOW()
	FROM updated WHERE a.id = updated.media_asset_id
), merge_job AS (
	INSERT INTO video_jobs (kind, recording_id, max_attempts)
	SELECT 'merge', id, $5 FROM updated WHERE audio_status = 'completed'
	ON CONFLICT (recording_id, kind) WHERE recording_id IS NOT NULL DO UPDATE
	SET status = 'queued', attempts = 0, run_at = NOW(), locked_at = NULL,
	    last_error = '', updated_at = NOW()
), identity_job AS (
	INSERT INTO temporal_jobs (owner_user_id,recording_id,max_attempts,processing_version)
	SELECT owner_user_id,id,$5,processing_version FROM updated WHERE audio_status='completed'
	ON CONFLICT (recording_id,processing_version) DO UPDATE SET
		status='queued',attempts=0,run_at=NOW(),locked_at=NULL,last_error='',warning='',updated_at=NOW()
)
UPDATE video_jobs SET status = 'completed', locked_at = NULL, updated_at = NOW()
WHERE id = $6`
	if _, err := r.db.ExecContext(
		ctx, query, job.RecordingID, payload, provider, model, maxAttempts, job.ID,
		durationSeconds,
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
		visual_observations jsonb, evidence_kind text, source_identity text,
		processing_version integer, media_asset_id text,
		observation_ids jsonb, frame_ids jsonb, supporting_episode_ids jsonb
	)
), inserted AS (
	INSERT INTO video_episodes (
		recording_id, bucket_index, start_time, end_time, description,
		visual_description, speech_description, location, confidence,
		visual_observations, evidence_kind, source_identity, processing_version,
		media_asset_id, observation_ids, frame_ids
		, supporting_episode_ids
	)
	SELECT $1, bucket_index, start_time, end_time, description,
	       visual_description, speech_description, location, confidence,
	       COALESCE(visual_observations, '[]'::jsonb),
	       COALESCE(NULLIF(evidence_kind, ''), 'context_summary'),
	       COALESCE(NULLIF(source_identity, ''), 'legacy:' || md5(description)),
	       COALESCE(NULLIF(processing_version, 0), 2),
	       NULLIF(media_asset_id, ''), COALESCE(observation_ids, '[]'::jsonb),
	       COALESCE(frame_ids, '[]'::jsonb), COALESCE(supporting_episode_ids, '[]'::jsonb)
	FROM episode_rows
	ON CONFLICT (recording_id, evidence_kind, source_identity, processing_version) DO UPDATE
	SET start_time = EXCLUDED.start_time, end_time = EXCLUDED.end_time,
	    description = EXCLUDED.description, location = EXCLUDED.location,
	    visual_description = EXCLUDED.visual_description,
	    speech_description = EXCLUDED.speech_description,
	    confidence = EXCLUDED.confidence,
	    visual_observations = EXCLUDED.visual_observations,
	    media_asset_id = EXCLUDED.media_asset_id,
	    observation_ids = EXCLUDED.observation_ids, frame_ids = EXCLUDED.frame_ids,
	    supporting_episode_ids = EXCLUDED.supporting_episode_ids,
	    updated_at = NOW()
	RETURNING id, evidence_kind, source_identity, description,
	          visual_description, speech_description
), memograph_branches AS (
	SELECT inserted.id, branch.source
	FROM inserted
	CROSS JOIN LATERAL (VALUES
		(CASE WHEN inserted.source_identity LIKE 'legacy:%' THEN 'visual' ELSE inserted.evidence_kind END,
		 CASE WHEN inserted.source_identity LIKE 'legacy:%' THEN NULLIF(BTRIM(inserted.visual_description), '') ELSE inserted.description END),
		('speech', CASE WHEN inserted.source_identity LIKE 'legacy:%' THEN NULLIF(BTRIM(inserted.speech_description), '') END),
		('legacy', CASE WHEN inserted.source_identity LIKE 'legacy:%'
		                 AND BTRIM(inserted.visual_description) = ''
		                 AND BTRIM(inserted.speech_description) = ''
		                THEN NULLIF(BTRIM(inserted.description), '') END)
	) AS branch(source, data)
	WHERE branch.data IS NOT NULL
), memograph_jobs AS (
	INSERT INTO video_jobs (kind, episode_id, source, max_attempts)
	SELECT 'memograph', id, source, $3 FROM memograph_branches
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
	RETURNING e.id, e.recording_id, e.status, e.processing_version
)
UPDATE video_recordings r
	SET status = CASE
	  WHEN completed_episode.status = 'failed' THEN 'failed'
	  WHEN completed_episode.status = 'completed' AND NOT EXISTS (
		SELECT 1 FROM video_episodes e
		WHERE e.recording_id = r.id AND e.id <> completed_episode.id
		  AND e.processing_version = completed_episode.processing_version
		  AND e.evidence_kind <> 'context_summary' AND e.status <> 'completed'
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
		requiredEvidenceFailed := dead && job.MemographSource != "context_summary"
		if _, err := r.db.ExecContext(
			ctx, query, job.ID, jobStatus, runAt, cause, job.EpisodeID,
			episodeStatus, requiredEvidenceFailed,
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
