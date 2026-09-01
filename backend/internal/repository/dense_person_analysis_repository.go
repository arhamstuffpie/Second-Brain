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

type DensePersonAnalysisRepository interface {
	service.DensePersonAnalysisRepository
}

type densePersonAnalysisRepository struct {
	*base
}

func newDensePersonAnalysisRepository(base *base) *densePersonAnalysisRepository {
	return &densePersonAnalysisRepository{base: base}
}

func (r *densePersonAnalysisRepository) ClaimDensePersonAnalysis(
	ctx context.Context,
	configurationProfile string,
	staleAfter time.Duration,
) (service.DensePersonAnalysisJob, bool, error) {
	if staleAfter <= 0 {
		return service.DensePersonAnalysisJob{}, false, fmt.Errorf("dense person job limits are invalid")
	}
	const claim = `
WITH candidate AS (
    SELECT j.id
    FROM analysis_stage_jobs j
    WHERE j.stage='dense_person_tracking'
      AND (
        (j.status IN ('queued','retryable_failed') AND j.run_at<=NOW())
        OR (j.status='processing' AND j.locked_at<NOW()-make_interval(secs => $1))
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
    SET status='processing',configuration_profile=$2,
        started_at=COALESCE(started_at,NOW()),last_error='',updated_at=NOW()
    FROM claimed j WHERE a.id=j.analysis_run_id
)
SELECT j.id,a.id,a.recording_id,a.owner_user_id,a.processing_version,
       j.attempts,j.max_attempts,r.file_path,r.file_name,r.media_type
FROM claimed j
JOIN analysis_runs a ON a.id=j.analysis_run_id
JOIN video_recordings r ON r.id=a.recording_id AND r.owner_user_id=a.owner_user_id`
	var job service.DensePersonAnalysisJob
	err := r.db.QueryRowContext(ctx, claim, int64(staleAfter/time.Second), configurationProfile).Scan(
		&job.ID, &job.AnalysisRunID, &job.RecordingID, &job.OwnerUserID,
		&job.ProcessingVersion, &job.Attempts, &job.MaxAttempts,
		&job.FilePath, &job.FileName, &job.MediaType,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.DensePersonAnalysisJob{}, false, nil
	}
	if err != nil {
		return service.DensePersonAnalysisJob{}, false, fmt.Errorf("claim dense person analysis: %w", err)
	}
	return job, true, nil
}

func (r *densePersonAnalysisRepository) CompleteDensePersonAnalysis(
	ctx context.Context,
	job service.DensePersonAnalysisJob,
	analysis service.DensePersonAnalysis,
) error {
	if analysis.RecordingID != job.RecordingID || analysis.ProcessingVersion != job.ProcessingVersion {
		return fmt.Errorf("dense person analysis result does not match its job")
	}
	database, ok := r.db.(interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		return fmt.Errorf("dense person analysis persistence requires transaction support")
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dense person analysis persistence: %w", err)
	}
	defer tx.Rollback()

	provenance, err := json.Marshal(analysis.Provenance)
	if err != nil {
		return fmt.Errorf("encode dense person provenance: %w", err)
	}
	checkpoint, err := json.Marshal(map[string]any{
		"duration_seconds": analysis.DurationSeconds,
		"analyzed_fps":     analysis.AnalyzedFPS,
		"track_count":      len(analysis.Tracks),
		"warnings":         analysis.Warnings,
	})
	if err != nil {
		return fmt.Errorf("encode dense person checkpoint: %w", err)
	}

	trackStatement, err := tx.PrepareContext(ctx, `
INSERT INTO person_tracks (
    id,owner_user_id,recording_id,start_time,end_time,face_track_provider_ref,
    temporary_visual_label,tracking_confidence,evidence_frame_ids,processing_version,
    lifecycle_status,first_frame,last_frame,observation_count,quality_summary,model_provenance
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,$13,$14,$15::jsonb,$16::jsonb)`)
	if err != nil {
		return fmt.Errorf("prepare dense person track insert: %w", err)
	}
	defer trackStatement.Close()
	observationStatement, err := tx.PrepareContext(ctx, `
INSERT INTO face_track_observations (
    id,owner_user_id,recording_id,person_track_id,processing_version,frame_index,
    observed_at_seconds,face_box,landmarks,detection_score,quality,pose,
    embedding_reference,embedding_model,mouth_visible,mouth_activity,gallery_selected
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10,$11::jsonb,$12::jsonb,
          $13,$14,$15,$16,$17)`)
	if err != nil {
		return fmt.Errorf("prepare dense face observation insert: %w", err)
	}
	defer observationStatement.Close()
	embeddingStatement, err := tx.PrepareContext(ctx, `
INSERT INTO face_track_observation_embeddings (
    observation_id,owner_user_id,recording_id,processing_version,
    embedding_model,embedding_dimensions,embedding
) VALUES ($1,$2,$3,$4,$5,$6,$7)`)
	if err != nil {
		return fmt.Errorf("prepare dense face embedding insert: %w", err)
	}
	defer embeddingStatement.Close()

	for trackIndex, track := range analysis.Tracks {
		quality, marshalErr := json.Marshal(track.Quality)
		if marshalErr != nil {
			return fmt.Errorf("encode dense person track quality: %w", marshalErr)
		}
		galleryIDs, marshalErr := json.Marshal(track.GalleryObservationIDs)
		if marshalErr != nil {
			return fmt.Errorf("encode dense person gallery: %w", marshalErr)
		}
		label := fmt.Sprintf("dense-person-%d", trackIndex+1)
		if _, err := trackStatement.ExecContext(
			ctx, track.ID, job.OwnerUserID, job.RecordingID, track.StartTime, track.EndTime,
			track.ProviderTrackReference, label, track.TrackingConfidence, galleryIDs,
			job.ProcessingVersion, track.LifecycleStatus, track.FirstFrame, track.LastFrame,
			track.ObservationCount, quality, provenance,
		); err != nil {
			return fmt.Errorf("insert dense person track %q: %w", track.ID, err)
		}

		gallery := make(map[string]bool, len(track.GalleryObservationIDs))
		for _, observationID := range track.GalleryObservationIDs {
			gallery[observationID] = true
		}
		for _, observation := range track.Observations {
			box, marshalErr := json.Marshal(observation.Box)
			if marshalErr != nil {
				return fmt.Errorf("encode face observation box: %w", marshalErr)
			}
			landmarks, marshalErr := json.Marshal(observation.Landmarks)
			if marshalErr != nil {
				return fmt.Errorf("encode face observation landmarks: %w", marshalErr)
			}
			quality, marshalErr := json.Marshal(observation.Quality)
			if marshalErr != nil {
				return fmt.Errorf("encode face observation quality: %w", marshalErr)
			}
			pose, marshalErr := json.Marshal(observation.Pose)
			if marshalErr != nil {
				return fmt.Errorf("encode face observation pose: %w", marshalErr)
			}
			selected := gallery[observation.ObservationID]
			var embeddingReference any
			if selected && len(observation.Embedding) > 0 {
				embeddingReference = "database:face_track_observation_embeddings/" + observation.ObservationID
			}
			if _, err := observationStatement.ExecContext(
				ctx, observation.ObservationID, job.OwnerUserID, job.RecordingID, track.ID,
				job.ProcessingVersion, observation.FrameIndex, observation.Timestamp,
				box, landmarks, observation.DetectionScore, quality, pose, embeddingReference,
				analysis.Provenance.EmbeddingModel, observation.MouthVisible,
				observation.MouthActivity, selected,
			); err != nil {
				return fmt.Errorf("insert dense face observation %q: %w", observation.ObservationID, err)
			}
			if embeddingReference != nil {
				if _, err := embeddingStatement.ExecContext(
					ctx, observation.ObservationID, job.OwnerUserID, job.RecordingID,
					job.ProcessingVersion, analysis.Provenance.EmbeddingModel,
					len(observation.Embedding), observation.Embedding,
				); err != nil {
					return fmt.Errorf("insert dense face embedding %q: %w", observation.ObservationID, err)
				}
			}
		}
	}

	result, err := tx.ExecContext(ctx, `
UPDATE analysis_stage_jobs
SET status='completed',locked_at=NULL,checkpoint=$3::jsonb,result_provenance=$4::jsonb,
    last_error='',updated_at=NOW()
WHERE id=$1 AND analysis_run_id=$2 AND status='processing'`,
		job.ID, job.AnalysisRunID, checkpoint, provenance,
	)
	if err != nil {
		return fmt.Errorf("complete dense person stage: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("dense person stage is no longer claimable")
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
		return fmt.Errorf("complete dense person analysis run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dense person analysis persistence: %w", err)
	}
	return nil
}

func (r *densePersonAnalysisRepository) RetryDensePersonAnalysis(
	ctx context.Context,
	job service.DensePersonAnalysisJob,
	cause string,
	runAt time.Time,
	dead bool,
) error {
	status := "retryable_failed"
	if dead {
		status = "dead"
	}
	const query = `
WITH updated_job AS (
    UPDATE analysis_stage_jobs
    SET status=$3,run_at=$4,locked_at=NULL,last_error=$5,updated_at=NOW()
    WHERE id=$1 AND analysis_run_id=$2 AND status='processing'
    RETURNING analysis_run_id
)
UPDATE analysis_runs a
SET status=$3,last_error=$5,updated_at=NOW()
FROM updated_job j WHERE a.id=j.analysis_run_id`
	if _, err := r.db.ExecContext(ctx, query, job.ID, job.AnalysisRunID, status, runAt, cause); err != nil {
		return fmt.Errorf("retry dense person analysis: %w", err)
	}
	return nil
}

var _ DensePersonAnalysisRepository = (*densePersonAnalysisRepository)(nil)
