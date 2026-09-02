package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/arham/ai-second-brain/internal/service"
)

func (r *videoRepository) PipelineDebugOwners(ctx context.Context) ([]service.PipelineDebugOwner, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT u.id,u.email,COUNT(DISTINCT r.id),COUNT(DISTINCT a.id),
       MAX(COALESCE(a.updated_at,r.updated_at))
FROM users u
LEFT JOIN video_recordings r ON r.owner_user_id=u.id
LEFT JOIN analysis_runs a ON a.owner_user_id=u.id
GROUP BY u.id,u.email
ORDER BY MAX(COALESCE(a.updated_at,r.updated_at)) DESC NULLS LAST,u.email`)
	if err != nil {
		return nil, fmt.Errorf("load pipeline debug owners: %w", err)
	}
	defer rows.Close()
	owners := []service.PipelineDebugOwner{}
	for rows.Next() {
		var owner service.PipelineDebugOwner
		var lastActivity sql.NullTime
		if err := rows.Scan(
			&owner.ID, &owner.Email, &owner.RecordingCount, &owner.RunCount, &lastActivity,
		); err != nil {
			return nil, fmt.Errorf("scan pipeline debug owner: %w", err)
		}
		if lastActivity.Valid {
			owner.LastActivityAt = &lastActivity.Time
		}
		owners = append(owners, owner)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pipeline debug owners: %w", err)
	}
	return owners, nil
}

func (r *videoRepository) PipelineDebugAnalysisOverview(
	ctx context.Context,
	ownerUserID string,
) (service.PipelineDebugAnalysisOverview, error) {
	overview := service.PipelineDebugAnalysisOverview{
		OwnerID: ownerUserID,
		Runs:    []service.PipelineDebugAnalysisRun{},
	}
	rows, err := r.db.QueryContext(ctx, `
WITH selected_runs AS (
    SELECT a.id,a.recording_id,r.file_name,a.processing_version,a.status,a.is_active,
           a.configuration_profile,a.last_error,a.created_at,a.updated_at
    FROM analysis_runs a
    JOIN video_recordings r ON r.owner_user_id=a.owner_user_id AND r.id=a.recording_id
    WHERE a.owner_user_id=$1
    ORDER BY a.created_at DESC,a.processing_version DESC
    LIMIT 50
)
SELECT a.id,a.recording_id,a.file_name,a.processing_version,a.status,a.is_active,
       a.configuration_profile,a.last_error,a.created_at,a.updated_at,
       j.stage,j.required,j.status,j.attempts,j.max_attempts,to_jsonb(j.depends_on),
       j.checkpoint,j.result_provenance,j.last_error,j.run_at,j.updated_at
FROM selected_runs a
LEFT JOIN analysis_stage_jobs j ON j.analysis_run_id=a.id
ORDER BY a.created_at DESC,a.processing_version DESC,j.id`, ownerUserID)
	if err != nil {
		return service.PipelineDebugAnalysisOverview{}, fmt.Errorf("load pipeline analysis debug overview: %w", err)
	}
	defer rows.Close()
	indexes := make(map[string]int)
	for rows.Next() {
		var run service.PipelineDebugAnalysisRun
		var stage, stageStatus, stageError sql.NullString
		var required sql.NullBool
		var attempts, maxAttempts sql.NullInt64
		var dependsJSON, checkpointJSON, provenanceJSON []byte
		var runAt, stageUpdatedAt sql.NullTime
		if err := rows.Scan(
			&run.ID, &run.RecordingID, &run.FileName, &run.ProcessingVersion,
			&run.Status, &run.Active, &run.ConfigurationProfile, &run.LastError,
			&run.CreatedAt, &run.UpdatedAt, &stage, &required, &stageStatus,
			&attempts, &maxAttempts, &dependsJSON, &checkpointJSON, &provenanceJSON,
			&stageError, &runAt, &stageUpdatedAt,
		); err != nil {
			return service.PipelineDebugAnalysisOverview{}, fmt.Errorf("scan pipeline analysis debug overview: %w", err)
		}
		index, exists := indexes[run.ID]
		if !exists {
			run.Stages = []service.PipelineDebugAnalysisStage{}
			index = len(overview.Runs)
			indexes[run.ID] = index
			overview.Runs = append(overview.Runs, run)
		}
		if !stage.Valid {
			continue
		}
		item := service.PipelineDebugAnalysisStage{
			Stage: stage.String, Required: required.Bool, Status: stageStatus.String,
			Attempts: int(attempts.Int64), MaxAttempts: int(maxAttempts.Int64),
			LastError: stageError.String,
		}
		if runAt.Valid {
			item.RunAt = runAt.Time
		}
		if stageUpdatedAt.Valid {
			item.UpdatedAt = stageUpdatedAt.Time
		}
		if err := json.Unmarshal(dependsJSON, &item.DependsOn); err != nil {
			return service.PipelineDebugAnalysisOverview{}, fmt.Errorf("decode pipeline stage dependencies: %w", err)
		}
		if err := json.Unmarshal(checkpointJSON, &item.Checkpoint); err != nil {
			return service.PipelineDebugAnalysisOverview{}, fmt.Errorf("decode pipeline stage checkpoint: %w", err)
		}
		if err := json.Unmarshal(provenanceJSON, &item.ResultProvenance); err != nil {
			return service.PipelineDebugAnalysisOverview{}, fmt.Errorf("decode pipeline stage provenance: %w", err)
		}
		overview.Runs[index].Stages = append(overview.Runs[index].Stages, item)
	}
	if err := rows.Err(); err != nil {
		return service.PipelineDebugAnalysisOverview{}, fmt.Errorf("iterate pipeline analysis debug overview: %w", err)
	}
	return overview, nil
}

func (r *videoRepository) DensePipelineDebugOverview(
	ctx context.Context,
	ownerUserID string,
) (service.PipelineDebugDenseOverview, error) {
	overview := service.PipelineDebugDenseOverview{
		Worker:     service.PipelineDebugDenseWorker{Jobs: make(map[string]int)},
		Recordings: []service.PipelineDebugDenseRecording{},
	}
	var queued, processing, completed, retryableFailed, dead int
	var oldestQueued, lastCompleted sql.NullTime
	if err := r.db.QueryRowContext(ctx, `
SELECT
    COUNT(*) FILTER (WHERE j.status='queued'),
    COUNT(*) FILTER (WHERE j.status='processing'),
    COUNT(*) FILTER (WHERE j.status='completed'),
    COUNT(*) FILTER (WHERE j.status='retryable_failed'),
    COUNT(*) FILTER (WHERE j.status='dead'),
    MIN(j.run_at) FILTER (WHERE j.status IN ('queued','retryable_failed')),
    MAX(j.updated_at) FILTER (WHERE j.status='completed')
FROM analysis_stage_jobs j
JOIN analysis_runs a ON a.id=j.analysis_run_id
WHERE a.owner_user_id=$1 AND j.stage='dense_person_tracking'`, ownerUserID).Scan(
		&queued, &processing, &completed, &retryableFailed, &dead,
		&oldestQueued, &lastCompleted,
	); err != nil {
		return service.PipelineDebugDenseOverview{}, fmt.Errorf("load dense worker debug status: %w", err)
	}
	overview.Worker.Jobs["queued"] = queued
	overview.Worker.Jobs["processing"] = processing
	overview.Worker.Jobs["completed"] = completed
	overview.Worker.Jobs["retryable_failed"] = retryableFailed
	overview.Worker.Jobs["dead"] = dead
	if oldestQueued.Valid {
		overview.Worker.OldestQueuedAt = &oldestQueued.Time
	}
	if lastCompleted.Valid {
		overview.Worker.LastCompletedAt = &lastCompleted.Time
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT a.recording_id,r.file_name,a.processing_version,a.status,j.status,
       j.attempts,j.max_attempts,j.last_error,j.checkpoint,j.result_provenance,
       COUNT(DISTINCT t.id),COUNT(DISTINCT o.id),
       COUNT(DISTINCT o.id) FILTER (WHERE o.gallery_selected),
       COUNT(DISTINCT e.observation_id),a.created_at,j.updated_at
FROM analysis_runs a
JOIN video_recordings r ON r.id=a.recording_id AND r.owner_user_id=a.owner_user_id
JOIN analysis_stage_jobs j ON j.analysis_run_id=a.id AND j.stage='dense_person_tracking'
LEFT JOIN person_tracks t
  ON t.owner_user_id=a.owner_user_id AND t.recording_id=a.recording_id
 AND t.processing_version=a.processing_version
LEFT JOIN face_track_observations o
  ON o.owner_user_id=t.owner_user_id AND o.recording_id=t.recording_id
 AND o.person_track_id=t.id AND o.processing_version=t.processing_version
LEFT JOIN face_track_observation_embeddings e ON e.observation_id=o.id
WHERE a.owner_user_id=$1
GROUP BY a.recording_id,r.file_name,a.processing_version,a.status,j.status,
         j.attempts,j.max_attempts,j.last_error,j.checkpoint,j.result_provenance,
         a.created_at,j.updated_at
ORDER BY a.created_at DESC,a.processing_version DESC
LIMIT 50`, ownerUserID)
	if err != nil {
		return service.PipelineDebugDenseOverview{}, fmt.Errorf("load dense debug recordings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var recording service.PipelineDebugDenseRecording
		var checkpointJSON, provenanceJSON []byte
		if err := rows.Scan(
			&recording.RecordingID, &recording.FileName, &recording.ProcessingVersion,
			&recording.RunStatus, &recording.StageStatus, &recording.Attempts,
			&recording.MaxAttempts, &recording.LastError, &checkpointJSON, &provenanceJSON,
			&recording.TrackCount, &recording.ObservationCount, &recording.GalleryCount,
			&recording.EmbeddingCount, &recording.CreatedAt, &recording.UpdatedAt,
		); err != nil {
			return service.PipelineDebugDenseOverview{}, fmt.Errorf("scan dense debug recording: %w", err)
		}
		if err := json.Unmarshal(checkpointJSON, &recording.Checkpoint); err != nil {
			return service.PipelineDebugDenseOverview{}, fmt.Errorf("decode dense debug checkpoint: %w", err)
		}
		if err := json.Unmarshal(provenanceJSON, &recording.ResultProvenance); err != nil {
			return service.PipelineDebugDenseOverview{}, fmt.Errorf("decode dense debug provenance: %w", err)
		}
		overview.Recordings = append(overview.Recordings, recording)
	}
	if err := rows.Err(); err != nil {
		return service.PipelineDebugDenseOverview{}, fmt.Errorf("iterate dense debug recordings: %w", err)
	}
	return overview, nil
}

func (r *videoRepository) DensePipelineDebugRecording(
	ctx context.Context,
	ownerUserID, recordingID string,
	processingVersion int,
) (service.PipelineDebugDenseRecordingDetail, error) {
	detail := service.PipelineDebugDenseRecordingDetail{Tracks: []service.PipelineDebugDenseTrack{}}
	var checkpointJSON, provenanceJSON, visualJSON []byte
	err := r.db.QueryRowContext(ctx, `
SELECT a.recording_id,r.file_name,a.processing_version,a.status,j.status,
       j.attempts,j.max_attempts,j.last_error,j.checkpoint,j.result_provenance,
       (SELECT COUNT(*) FROM person_tracks t
        WHERE t.owner_user_id=a.owner_user_id AND t.recording_id=a.recording_id
          AND t.processing_version=a.processing_version),
       (SELECT COUNT(*) FROM face_track_observations o
        WHERE o.owner_user_id=a.owner_user_id AND o.recording_id=a.recording_id
          AND o.processing_version=a.processing_version),
       (SELECT COUNT(*) FROM face_track_observations o
        WHERE o.owner_user_id=a.owner_user_id AND o.recording_id=a.recording_id
          AND o.processing_version=a.processing_version AND o.gallery_selected),
       (SELECT COUNT(*) FROM face_track_observation_embeddings e
        WHERE e.owner_user_id=a.owner_user_id AND e.recording_id=a.recording_id
          AND e.processing_version=a.processing_version),
       a.created_at,j.updated_at,r.visual_analysis
FROM analysis_runs a
JOIN video_recordings r ON r.id=a.recording_id AND r.owner_user_id=a.owner_user_id
JOIN analysis_stage_jobs j ON j.analysis_run_id=a.id AND j.stage='dense_person_tracking'
WHERE a.owner_user_id=$1 AND a.recording_id=$2
  AND ($3=0 OR a.processing_version=$3)
ORDER BY a.processing_version DESC
LIMIT 1`, ownerUserID, recordingID, processingVersion).Scan(
		&detail.Recording.RecordingID, &detail.Recording.FileName,
		&detail.Recording.ProcessingVersion, &detail.Recording.RunStatus,
		&detail.Recording.StageStatus, &detail.Recording.Attempts,
		&detail.Recording.MaxAttempts, &detail.Recording.LastError,
		&checkpointJSON, &provenanceJSON, &detail.Recording.TrackCount,
		&detail.Recording.ObservationCount, &detail.Recording.GalleryCount,
		&detail.Recording.EmbeddingCount, &detail.Recording.CreatedAt,
		&detail.Recording.UpdatedAt, &visualJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.PipelineDebugDenseRecordingDetail{}, service.ErrNotFound
	}
	if err != nil {
		return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("load dense debug recording: %w", err)
	}
	if err := json.Unmarshal(checkpointJSON, &detail.Recording.Checkpoint); err != nil {
		return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("decode dense debug checkpoint: %w", err)
	}
	if err := json.Unmarshal(provenanceJSON, &detail.Recording.ResultProvenance); err != nil {
		return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("decode dense debug provenance: %w", err)
	}
	if len(visualJSON) > 0 {
		if err := json.Unmarshal(visualJSON, &detail.VisualAnalysis); err != nil {
			return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("decode dense debug visual analysis: %w", err)
		}
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT t.id,t.face_track_provider_ref,t.temporary_visual_label,
       COALESCE(t.resolved_person_profile_id,''),COALESCE(p.display_name,''),COALESCE(p.status,''),
       t.lifecycle_status,t.first_frame,t.last_frame,t.start_time,t.end_time,
       t.observation_count,t.tracking_confidence,t.quality_summary,
       t.evidence_frame_ids,t.model_provenance,t.created_at,t.updated_at
FROM person_tracks t
LEFT JOIN person_profiles p
  ON p.owner_user_id=t.owner_user_id AND p.id=t.resolved_person_profile_id
WHERE t.owner_user_id=$1 AND t.recording_id=$2 AND t.processing_version=$3
ORDER BY t.start_time,t.id`, ownerUserID, recordingID, detail.Recording.ProcessingVersion)
	if err != nil {
		return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("load dense debug tracks: %w", err)
	}
	trackIndexes := make(map[string]int)
	for rows.Next() {
		var track service.PipelineDebugDenseTrack
		var firstFrame, lastFrame sql.NullInt64
		var qualityJSON, evidenceJSON, modelJSON []byte
		if err := rows.Scan(
			&track.ID, &track.ProviderTrackReference, &track.TemporaryVisualLabel,
			&track.ResolvedPersonProfileID, &track.ResolvedPersonName,
			&track.ResolvedPersonStatus, &track.LifecycleStatus, &firstFrame, &lastFrame,
			&track.StartTime, &track.EndTime, &track.ObservationCount,
			&track.TrackingConfidence, &qualityJSON, &evidenceJSON, &modelJSON,
			&track.CreatedAt, &track.UpdatedAt,
		); err != nil {
			rows.Close()
			return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("scan dense debug track: %w", err)
		}
		if firstFrame.Valid {
			track.FirstFrame = int(firstFrame.Int64)
		}
		if lastFrame.Valid {
			track.LastFrame = int(lastFrame.Int64)
		}
		if err := json.Unmarshal(qualityJSON, &track.Quality); err != nil {
			rows.Close()
			return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("decode dense debug track quality: %w", err)
		}
		if err := json.Unmarshal(evidenceJSON, &track.EvidenceFrameIDs); err != nil {
			rows.Close()
			return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("decode dense debug track evidence: %w", err)
		}
		if err := json.Unmarshal(modelJSON, &track.ModelProvenance); err != nil {
			rows.Close()
			return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("decode dense debug track model: %w", err)
		}
		track.Observations = []service.PipelineDebugDenseObservation{}
		trackIndexes[track.ID] = len(detail.Tracks)
		detail.Tracks = append(detail.Tracks, track)
	}
	if err := rows.Close(); err != nil {
		return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("close dense debug tracks: %w", err)
	}
	if err := rows.Err(); err != nil {
		return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("iterate dense debug tracks: %w", err)
	}

	observationRows, err := r.db.QueryContext(ctx, `
SELECT o.person_track_id,o.id,o.frame_index,o.observed_at_seconds,o.face_box,
       o.landmarks,o.detection_score,o.quality,o.pose,COALESCE(o.embedding_reference,''),
       o.embedding_model,COALESCE(e.embedding_dimensions,0),
       COALESCE(to_jsonb(e.embedding),'[]'::jsonb),o.mouth_visible,
       COALESCE(o.mouth_activity,0),o.gallery_selected,o.created_at
FROM face_track_observations o
LEFT JOIN face_track_observation_embeddings e ON e.observation_id=o.id
WHERE o.owner_user_id=$1 AND o.recording_id=$2 AND o.processing_version=$3
ORDER BY o.person_track_id,o.observed_at_seconds,o.frame_index`,
		ownerUserID, recordingID, detail.Recording.ProcessingVersion,
	)
	if err != nil {
		return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("load dense debug observations: %w", err)
	}
	defer observationRows.Close()
	for observationRows.Next() {
		var trackID string
		var observation service.PipelineDebugDenseObservation
		var boxJSON, landmarksJSON, qualityJSON, poseJSON, embeddingJSON []byte
		if err := observationRows.Scan(
			&trackID, &observation.ObservationID, &observation.FrameIndex,
			&observation.Timestamp, &boxJSON, &landmarksJSON,
			&observation.DetectionScore, &qualityJSON, &poseJSON,
			&observation.EmbeddingReference, &observation.EmbeddingModel,
			&observation.EmbeddingDimensions, &embeddingJSON,
			&observation.MouthVisible, &observation.MouthActivity,
			&observation.GallerySelected, &observation.CreatedAt,
		); err != nil {
			return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("scan dense debug observation: %w", err)
		}
		if err := json.Unmarshal(boxJSON, &observation.Box); err != nil {
			return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("decode dense debug face box: %w", err)
		}
		if err := json.Unmarshal(landmarksJSON, &observation.Landmarks); err != nil {
			return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("decode dense debug landmarks: %w", err)
		}
		if err := json.Unmarshal(qualityJSON, &observation.Quality); err != nil {
			return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("decode dense debug observation quality: %w", err)
		}
		if err := json.Unmarshal(poseJSON, &observation.Pose); err != nil {
			return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("decode dense debug observation pose: %w", err)
		}
		if err := json.Unmarshal(embeddingJSON, &observation.Embedding); err != nil {
			return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("decode dense debug embedding: %w", err)
		}
		if index, exists := trackIndexes[trackID]; exists {
			detail.Tracks[index].Observations = append(detail.Tracks[index].Observations, observation)
		}
	}
	if err := observationRows.Err(); err != nil {
		return service.PipelineDebugDenseRecordingDetail{}, fmt.Errorf("iterate dense debug observations: %w", err)
	}
	return detail, nil
}

func (r *videoRepository) DensePipelineDebugFaceSource(
	ctx context.Context,
	ownerUserID, recordingID, trackID, observationID string,
	processingVersion int,
) (service.PipelineDebugDenseFaceSource, error) {
	var source service.PipelineDebugDenseFaceSource
	var boxJSON []byte
	err := r.db.QueryRowContext(ctx, `
SELECT r.file_path,r.file_name,r.media_type,o.observed_at_seconds,o.face_box
FROM face_track_observations o
JOIN person_tracks t
  ON t.owner_user_id=o.owner_user_id AND t.recording_id=o.recording_id
 AND t.id=o.person_track_id AND t.processing_version=o.processing_version
JOIN video_recordings r ON r.owner_user_id=o.owner_user_id AND r.id=o.recording_id
WHERE o.owner_user_id=$1 AND o.recording_id=$2 AND o.person_track_id=$3
  AND o.id=$4 AND ($5=0 OR o.processing_version=$5)
ORDER BY o.processing_version DESC
LIMIT 1`,
		ownerUserID, recordingID, trackID, observationID, processingVersion,
	).Scan(&source.FilePath, &source.FileName, &source.MediaType, &source.Timestamp, &boxJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return service.PipelineDebugDenseFaceSource{}, service.ErrNotFound
	}
	if err != nil {
		return service.PipelineDebugDenseFaceSource{}, fmt.Errorf("load dense face preview source: %w", err)
	}
	if err := json.Unmarshal(boxJSON, &source.Box); err != nil {
		return service.PipelineDebugDenseFaceSource{}, fmt.Errorf("decode dense face preview box: %w", err)
	}
	return source, nil
}

var _ service.PipelineDebugRepository = (*videoRepository)(nil)
