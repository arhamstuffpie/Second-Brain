package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arham/ai-second-brain/internal/service"
)

type activeSpeakerFusionRepository struct{ *base }

type ActiveSpeakerFusionRepository interface {
	service.ActiveSpeakerFusionRepository
}

func newActiveSpeakerFusionRepository(base *base) *activeSpeakerFusionRepository {
	return &activeSpeakerFusionRepository{base: base}
}

func (r *activeSpeakerFusionRepository) ClaimActiveSpeakerFusion(
	ctx context.Context,
	configurationProfile string,
	staleAfter time.Duration,
) (service.ActiveSpeakerFusionJob, bool, error) {
	if staleAfter <= 0 {
		return service.ActiveSpeakerFusionJob{}, false, fmt.Errorf("active-speaker job limits are invalid")
	}
	database, ok := r.db.(interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		return service.ActiveSpeakerFusionJob{}, false, fmt.Errorf("active-speaker claim requires transaction support")
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return service.ActiveSpeakerFusionJob{}, false, fmt.Errorf("begin active-speaker claim: %w", err)
	}
	defer tx.Rollback()
	const claim = `
WITH candidate AS (
    SELECT j.id FROM analysis_stage_jobs j
    WHERE j.stage='active_speaker_fusion'
      AND ((j.status IN ('queued','retryable_failed') AND j.run_at<=NOW())
        OR (j.status='processing' AND j.locked_at<NOW()-make_interval(secs => $1)))
      AND NOT EXISTS (
        SELECT 1 FROM unnest(j.depends_on) dependency(stage)
        LEFT JOIN analysis_stage_jobs prerequisite
          ON prerequisite.analysis_run_id=j.analysis_run_id AND prerequisite.stage=dependency.stage
        WHERE prerequisite.status IS DISTINCT FROM 'completed'
      )
    ORDER BY j.run_at,j.id FOR UPDATE SKIP LOCKED LIMIT 1
), claimed AS (
    UPDATE analysis_stage_jobs j
    SET status='processing',attempts=attempts+1,locked_at=NOW(),last_error='',updated_at=NOW()
    FROM candidate c WHERE j.id=c.id RETURNING j.*
), updated_run AS (
    UPDATE analysis_runs a SET status='processing',configuration_profile=$2,
        started_at=COALESCE(started_at,NOW()),last_error='',updated_at=NOW()
    FROM claimed j WHERE a.id=j.analysis_run_id
)
SELECT j.id,a.id,a.recording_id,a.owner_user_id,a.processing_version,
       j.attempts,j.max_attempts,r.file_path,r.file_name,r.media_type,r.transcript
FROM claimed j
JOIN analysis_runs a ON a.id=j.analysis_run_id
JOIN video_recordings r ON r.id=a.recording_id AND r.owner_user_id=a.owner_user_id`
	var job service.ActiveSpeakerFusionJob
	var transcriptJSON []byte
	err = tx.QueryRowContext(ctx, claim, int64(staleAfter/time.Second), configurationProfile).Scan(
		&job.ID, &job.AnalysisRunID, &job.RecordingID, &job.OwnerUserID,
		&job.ProcessingVersion, &job.Attempts, &job.MaxAttempts,
		&job.FilePath, &job.FileName, &job.MediaType, &transcriptJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ActiveSpeakerFusionJob{}, false, nil
	}
	if err != nil {
		return service.ActiveSpeakerFusionJob{}, false, fmt.Errorf("claim active-speaker fusion: %w", err)
	}
	if len(transcriptJSON) > 0 {
		if err := json.Unmarshal(transcriptJSON, &job.Transcript); err != nil {
			return service.ActiveSpeakerFusionJob{}, false, fmt.Errorf("decode active-speaker transcript: %w", err)
		}
	}
	tracks, err := newVideoRepository(r.base).loadDenseIdentityTracks(ctx, tx, service.VideoJob{
		OwnerUserID: job.OwnerUserID, RecordingID: job.RecordingID, ProcessingVersion: job.ProcessingVersion,
	})
	if err != nil {
		return service.ActiveSpeakerFusionJob{}, false, err
	}
	job.Tracks = tracks
	profiles, err := loadFusionVoiceProfiles(ctx, tx, job.OwnerUserID, job.Transcript)
	if err != nil {
		return service.ActiveSpeakerFusionJob{}, false, err
	}
	job.VoiceProfiles = profiles
	if err := tx.Commit(); err != nil {
		return service.ActiveSpeakerFusionJob{}, false, fmt.Errorf("commit active-speaker claim: %w", err)
	}
	return job, true, nil
}

func loadFusionVoiceProfiles(
	ctx context.Context,
	db DBTX,
	ownerUserID string,
	transcript service.Transcript,
) (map[string]service.ActiveSpeakerFusionVoiceProfile, error) {
	ids := []string{}
	seen := map[string]bool{}
	for _, segment := range transcript.Segments {
		if segment.SpeakerProfileID != "" && !seen[segment.SpeakerProfileID] {
			seen[segment.SpeakerProfileID] = true
			ids = append(ids, segment.SpeakerProfileID)
		}
	}
	profiles := make(map[string]service.ActiveSpeakerFusionVoiceProfile, len(ids))
	if len(ids) == 0 {
		return profiles, nil
	}
	rows, err := db.QueryContext(ctx, `
SELECT v.id,COALESCE(a.canonical_person_profile_id,v.person_profile_id,''),v.display_name,v.embedding_model
FROM voice_speaker_profiles v
LEFT JOIN person_profile_aliases a
  ON a.owner_user_id=v.owner_user_id AND a.alias_person_profile_id=v.person_profile_id
JOIN person_profiles p
  ON p.owner_user_id=v.owner_user_id AND p.id=COALESCE(a.canonical_person_profile_id,v.person_profile_id)
WHERE v.owner_user_id=$1 AND v.id=ANY($2::text[])
  AND v.status='confirmed' AND p.status='confirmed'`, ownerUserID, ids)
	if err != nil {
		return nil, fmt.Errorf("load fusion voice profiles: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var profile service.ActiveSpeakerFusionVoiceProfile
		if err := rows.Scan(&profile.ID, &profile.PersonProfileID, &profile.DisplayName, &profile.EmbeddingModel); err != nil {
			return nil, fmt.Errorf("scan fusion voice profile: %w", err)
		}
		profiles[profile.ID] = profile
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fusion voice profiles: %w", err)
	}
	return profiles, nil
}

func (r *activeSpeakerFusionRepository) CompleteActiveSpeakerFusion(
	ctx context.Context,
	job service.ActiveSpeakerFusionJob,
	result service.ActiveSpeakerFusionResult,
	options service.ActiveSpeakerFusionPersistenceOptions,
) error {
	database, ok := r.db.(interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		return fmt.Errorf("active-speaker persistence requires transaction support")
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin active-speaker persistence: %w", err)
	}
	defer tx.Rollback()

	if options.SaveEvidence {
		for _, evidence := range result.Evidence {
			if err := saveFusionEvidence(ctx, tx, job, result, evidence); err != nil {
				return err
			}
		}
	}
	linked := 0
	if options.AutoResolveTracks && options.SaveEvidence {
		linked, err = resolveFusionTracks(ctx, tx, job, options)
		if err != nil {
			return err
		}
	}
	checkpoint, err := json.Marshal(map[string]any{
		"evidence_count": len(result.Evidence), "linked_track_count": linked,
		"save_evidence": options.SaveEvidence, "auto_resolve_tracks": options.AutoResolveTracks,
		"auto_bootstrap_faces": options.AutoBootstrapFaces, "auto_modify_graph": options.AutoModifyGraph,
		"warning": result.Warning,
	})
	if err != nil {
		return fmt.Errorf("encode active-speaker checkpoint: %w", err)
	}
	provenance, err := json.Marshal(map[string]any{
		"provider": result.Provider, "model": result.Model, "version": result.Version,
	})
	if err != nil {
		return fmt.Errorf("encode active-speaker provenance: %w", err)
	}
	stageResult, err := tx.ExecContext(ctx, `
UPDATE analysis_stage_jobs
SET status='completed',locked_at=NULL,checkpoint=$3::jsonb,result_provenance=$4::jsonb,
    last_error='',updated_at=NOW()
WHERE id=$1 AND analysis_run_id=$2 AND status='processing'`,
		job.ID, job.AnalysisRunID, checkpoint, provenance)
	if err != nil {
		return fmt.Errorf("complete active-speaker stage: %w", err)
	}
	if affected, err := stageResult.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("active-speaker stage is no longer claimable")
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE analysis_runs a
SET status=CASE WHEN EXISTS (
        SELECT 1 FROM analysis_stage_jobs j WHERE j.analysis_run_id=a.id AND j.required AND j.status<>'completed'
    ) THEN 'processing' ELSE 'completed' END,
    completed_at=CASE WHEN NOT EXISTS (
        SELECT 1 FROM analysis_stage_jobs j WHERE j.analysis_run_id=a.id AND j.required AND j.status<>'completed'
    ) THEN NOW() ELSE NULL END,
    last_error='',updated_at=NOW()
WHERE a.id=$1`, job.AnalysisRunID); err != nil {
		return fmt.Errorf("complete active-speaker analysis run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit active-speaker fusion: %w", err)
	}
	return nil
}

func saveFusionEvidence(
	ctx context.Context,
	tx *sql.Tx,
	job service.ActiveSpeakerFusionJob,
	result service.ActiveSpeakerFusionResult,
	evidence service.ActiveSpeakerFusionEvidence,
) error {
	segmentIDs, _ := json.Marshal([]string{evidence.SegmentID})
	reasons, _ := json.Marshal(evidence.Reasons)
	raw, _ := json.Marshal(evidence.Raw)
	faceProvider, faceModel := "dense-person-tracking", "unknown"
	for _, track := range job.Tracks {
		if track.ID == evidence.PersonTrackID {
			faceProvider, faceModel = track.Provider, track.EmbeddingModel
			break
		}
	}
	model, _ := json.Marshal(map[string]any{
		"active_speaker_provider": result.Provider, "active_speaker_model": result.Model,
		"active_speaker_version": result.Version, "dense_face_model": faceModel,
		"voice_embedding_model": job.VoiceProfiles[evidence.VoiceSpeakerProfileID].EmbeddingModel,
	})
	_, err := tx.ExecContext(ctx, `
INSERT INTO identity_link_evidence (
    deterministic_key,owner_user_id,recording_id,face_profile_id,person_track_id,
    voice_speaker_profile_id,canonical_person_profile_id,diarized_segment_ids,
    segment_id,segment_start_time,segment_end_time,active_speaker_score,
    active_speaker_runner_up_score,decision_margin,visible_mouth_coverage,
    voice_match_score,temporal_coverage,overlapping_speaker_conflict,overlap_group_id,
    separation_status,separation_score,mouth_activity_score,physical_presence_confidence,
    combined_score,decision,conflict_reasons,face_provider,face_model,
    active_speaker_provider,active_speaker_model,model_provenance,raw_evidence,processing_version
) VALUES (
    $1,$2,$3,NULL,NULLIF($4,''),$5,$6,$7::jsonb,$8,$9,$10,$11,$12,$13,$14,
    $15,$16,$17,NULLIF($18,''),NULLIF($19,''),$20,$21,$22,$23,$24,$25::jsonb,
    $26,$27,$28,$29,$30::jsonb,$31::jsonb,$32
)
ON CONFLICT (deterministic_key) DO UPDATE SET
    active_speaker_score=EXCLUDED.active_speaker_score,
    active_speaker_runner_up_score=EXCLUDED.active_speaker_runner_up_score,
    decision_margin=EXCLUDED.decision_margin,visible_mouth_coverage=EXCLUDED.visible_mouth_coverage,
    temporal_coverage=EXCLUDED.temporal_coverage,mouth_activity_score=EXCLUDED.mouth_activity_score,
    physical_presence_confidence=EXCLUDED.physical_presence_confidence,
    combined_score=EXCLUDED.combined_score,decision=EXCLUDED.decision,
    conflict_reasons=EXCLUDED.conflict_reasons,model_provenance=EXCLUDED.model_provenance,
    raw_evidence=EXCLUDED.raw_evidence,updated_at=NOW()`,
		evidence.DeterministicKey, job.OwnerUserID, job.RecordingID, evidence.PersonTrackID,
		evidence.VoiceSpeakerProfileID, evidence.CanonicalPersonID, segmentIDs,
		evidence.SegmentID, evidence.SegmentStartTime, evidence.SegmentEndTime,
		evidence.ActiveSpeakerScore, evidence.RunnerUpScore, evidence.DecisionMargin,
		evidence.MouthVisibleCoverage, evidence.VoiceConfidence, evidence.TemporalCoverage,
		evidence.OverlappingConflict, evidence.OverlapGroupID, evidence.SeparationStatus,
		evidence.SeparationScore, evidence.MouthActivity, evidence.PhysicalPresence,
		evidence.CombinedScore, evidence.Decision, reasons, faceProvider, faceModel,
		result.Provider, result.Model, model, raw, job.ProcessingVersion,
	)
	if err != nil {
		return fmt.Errorf("save active-speaker evidence %q: %w", evidence.DeterministicKey, err)
	}
	return nil
}

func resolveFusionTracks(
	ctx context.Context,
	tx *sql.Tx,
	job service.ActiveSpeakerFusionJob,
	options service.ActiveSpeakerFusionPersistenceOptions,
) (int, error) {
	minimum := options.MinimumEvidence
	if minimum < 2 {
		minimum = 2
	}
	minimumSpan := options.MinimumEvidenceSpanSeconds
	if minimumSpan <= 0 {
		minimumSpan = .5
	}
	rows, err := tx.QueryContext(ctx, `
SELECT e.person_track_id,e.canonical_person_profile_id,COUNT(DISTINCT e.segment_id),
       AVG(e.combined_score),jsonb_agg(DISTINCT e.id)
FROM identity_link_evidence e
WHERE e.owner_user_id=$1 AND e.recording_id=$2 AND e.processing_version=$3
  AND e.decision='accepted' AND e.person_track_id IS NOT NULL
GROUP BY e.person_track_id,e.canonical_person_profile_id
HAVING COUNT(DISTINCT e.segment_id)>=$4
   AND MAX(e.segment_end_time)-MIN(e.segment_start_time)>=$5`,
		job.OwnerUserID, job.RecordingID, job.ProcessingVersion, minimum, minimumSpan)
	if err != nil {
		return 0, fmt.Errorf("load repeated active-speaker evidence: %w", err)
	}
	type target struct {
		trackID, personID string
		count             int
		confidence        float64
		evidenceJSON      []byte
	}
	var targets []target
	for rows.Next() {
		var item target
		if err := rows.Scan(&item.trackID, &item.personID, &item.count, &item.confidence, &item.evidenceJSON); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan repeated active-speaker evidence: %w", err)
		}
		targets = append(targets, item)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close repeated active-speaker evidence: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate repeated active-speaker evidence: %w", err)
	}
	linked := 0
	for _, target := range targets {
		var competing int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT canonical_person_profile_id)
FROM identity_link_evidence
WHERE owner_user_id=$1 AND recording_id=$2 AND processing_version=$3
  AND person_track_id=$4 AND decision='accepted'`,
			job.OwnerUserID, job.RecordingID, job.ProcessingVersion, target.trackID).Scan(&competing); err != nil {
			return linked, fmt.Errorf("check active-speaker identity conflicts: %w", err)
		}
		if competing > 1 {
			if _, err := tx.ExecContext(ctx, `
UPDATE identity_link_evidence
SET decision='ambiguous',conflict_reasons=conflict_reasons || '["different_known_voices_same_track"]'::jsonb,updated_at=NOW()
WHERE owner_user_id=$1 AND recording_id=$2 AND processing_version=$3 AND person_track_id=$4 AND decision='accepted'`,
				job.OwnerUserID, job.RecordingID, job.ProcessingVersion, target.trackID); err != nil {
				return linked, fmt.Errorf("save active-speaker identity conflict: %w", err)
			}
			continue
		}
		if competing != 1 {
			continue
		}
		update, err := tx.ExecContext(ctx, `
UPDATE person_tracks t SET
    resolved_person_profile_id=$5,resolution_method='labelled_voice_active_speaker',
    resolution_evidence_count=$6,resolution_confidence=$7,
    resolution_processing_version=$3,resolved_at=NOW(),updated_at=NOW()
WHERE t.owner_user_id=$1 AND t.recording_id=$2 AND t.processing_version=$3 AND t.id=$4
  AND (t.resolved_person_profile_id IS NULL OR t.resolved_person_profile_id=$5 OR EXISTS (
      SELECT 1 FROM person_profiles current
      WHERE current.owner_user_id=t.owner_user_id AND current.id=t.resolved_person_profile_id
        AND current.status='provisional'
  ))`, job.OwnerUserID, job.RecordingID, job.ProcessingVersion, target.trackID,
			target.personID, target.count, target.confidence)
		if err != nil {
			return linked, fmt.Errorf("resolve dense track from labelled voice: %w", err)
		}
		affected, _ := update.RowsAffected()
		if affected != 1 {
			if _, err := tx.ExecContext(ctx, `
UPDATE identity_link_evidence SET decision='ambiguous',
    conflict_reasons=conflict_reasons || '["existing_confirmed_identity_conflict"]'::jsonb,updated_at=NOW()
WHERE owner_user_id=$1 AND recording_id=$2 AND processing_version=$3
  AND person_track_id=$4 AND canonical_person_profile_id=$5`,
				job.OwnerUserID, job.RecordingID, job.ProcessingVersion, target.trackID, target.personID); err != nil {
				return linked, fmt.Errorf("save confirmed identity conflict: %w", err)
			}
			continue
		}
		if err := canonicalizeFusionSceneTrack(ctx, tx, job, target.trackID, target.personID); err != nil {
			return linked, err
		}
		if options.AutoBootstrapFaces {
			if err := bootstrapFusionFace(ctx, tx, job, target.trackID, target.personID, target.evidenceJSON); err != nil {
				return linked, err
			}
		}
		linked++
	}
	return linked, nil
}

func canonicalizeFusionSceneTrack(
	ctx context.Context,
	tx *sql.Tx,
	job service.ActiveSpeakerFusionJob,
	trackID, personID string,
) error {
	_, err := tx.ExecContext(ctx, `
UPDATE video_recordings r SET visual_analysis=jsonb_set(
  COALESCE(r.visual_analysis,'{}'::jsonb),'{observations}',COALESCE((
    SELECT jsonb_agg(observation.value || jsonb_build_object('people',COALESCE((
      SELECT jsonb_agg(CASE WHEN person.value->>'person_track_id'=$3 THEN
        person.value || jsonb_build_object(
          'temporary_visual_label',COALESCE(NULLIF(person.value->>'temporary_visual_label',''),person.value->>'visual_label'),
          'person_profile_id',$4::text,'person_name',profile.display_name,'person_identity_status','confirmed'
        ) ELSE person.value END ORDER BY person.ordinality)
      FROM jsonb_array_elements(COALESCE(observation.value->'people','[]'::jsonb)) WITH ORDINALITY person(value,ordinality)
    ),'[]'::jsonb)) ORDER BY observation.ordinality)
    FROM jsonb_array_elements(COALESCE(r.visual_analysis->'observations','[]'::jsonb)) WITH ORDINALITY observation(value,ordinality)
    JOIN person_profiles profile ON profile.owner_user_id=r.owner_user_id AND profile.id=$4
  ),'[]'::jsonb),true),updated_at=NOW()
WHERE r.owner_user_id=$1 AND r.id=$2`, job.OwnerUserID, job.RecordingID, trackID, personID)
	if err != nil {
		return fmt.Errorf("canonicalize sampled scene identity: %w", err)
	}
	return nil
}

func bootstrapFusionFace(
	ctx context.Context,
	tx *sql.Tx,
	job service.ActiveSpeakerFusionJob,
	trackID, personID string,
	evidenceJSON []byte,
) error {
	rows, err := tx.QueryContext(ctx, `
SELECT o.id,o.embedding_model,COALESCE(t.model_provenance->>'detector_model','dense-person-detector'),
       o.detection_score,o.quality,o.pose,COALESCE(to_jsonb(e.embedding),'[]'::jsonb),o.observed_at_seconds
FROM face_track_observations o
JOIN person_tracks t ON t.owner_user_id=o.owner_user_id AND t.recording_id=o.recording_id
 AND t.id=o.person_track_id AND t.processing_version=o.processing_version
JOIN face_track_observation_embeddings e ON e.observation_id=o.id
WHERE o.owner_user_id=$1 AND o.recording_id=$2 AND o.processing_version=$3
  AND o.person_track_id=$4 AND o.gallery_selected
  AND COALESCE((o.quality->>'usable')::boolean,FALSE)
  AND COALESCE(o.pose->>'bucket','') IN ('frontal','left_three_quarter','right_three_quarter')
ORDER BY COALESCE((o.quality->>'score')::double precision,0) DESC,o.observed_at_seconds`,
		job.OwnerUserID, job.RecordingID, job.ProcessingVersion, trackID)
	if err != nil {
		return fmt.Errorf("load fusion face gallery: %w", err)
	}
	defer rows.Close()
	type sample struct {
		id, model, detector      string
		detection                float64
		quality, pose, embedding []byte
		timestamp                float64
	}
	var samples []sample
	model := ""
	for rows.Next() {
		var item sample
		if err := rows.Scan(&item.id, &item.model, &item.detector, &item.detection, &item.quality, &item.pose, &item.embedding, &item.timestamp); err != nil {
			return fmt.Errorf("scan fusion face gallery: %w", err)
		}
		if model != "" && model != item.model {
			return fmt.Errorf("fusion face gallery model checksum mismatch")
		}
		model = item.model
		samples = append(samples, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate fusion face gallery: %w", err)
	}
	if len(samples) == 0 {
		return nil
	}
	provider := strings.SplitN(model, "/", 2)[0]
	var faceProfileID string
	for _, item := range samples {
		var embedding []float64
		var pose service.FacePose
		if err := json.Unmarshal(item.embedding, &embedding); err != nil || len(embedding) == 0 {
			return fmt.Errorf("decode fusion face embedding %q", item.id)
		}
		if err := json.Unmarshal(item.pose, &pose); err != nil {
			return fmt.Errorf("decode fusion face pose %q: %w", item.id, err)
		}
		if faceProfileID == "" {
			err := tx.QueryRowContext(ctx, `
INSERT INTO face_profiles (
  owner_user_id,person_profile_id,status,provider,detector_model,embedding_model,
  embedding_dimensions,centroid,sample_count,expires_at,enrollment_source,source_evidence_ids
) VALUES ($1,$2,'confirmed',$3,$4,$5,cardinality($6::double precision[]),$6::double precision[],1,NULL,
          'labelled_voice_active_speaker',$7::jsonb)
ON CONFLICT (owner_user_id,person_profile_id,provider,embedding_model) DO UPDATE SET
  status='confirmed',expires_at=NULL,enrollment_source='labelled_voice_active_speaker',
  source_evidence_ids=EXCLUDED.source_evidence_ids,updated_at=NOW()
RETURNING id`, job.OwnerUserID, personID, provider, item.detector, model, embedding, evidenceJSON).Scan(&faceProfileID)
			if err != nil {
				return fmt.Errorf("create fusion face profile: %w", err)
			}
		}
		var duplicate bool
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM face_profile_samples existing
  WHERE existing.owner_user_id=$1 AND existing.face_profile_id=$2
    AND (SELECT SUM(pair.x*pair.y)/NULLIF(SQRT(SUM(pair.x*pair.x))*SQRT(SUM(pair.y*pair.y)),0)
         FROM unnest(existing.embedding,$3::double precision[]) pair(x,y)) >= 0.995
)`, job.OwnerUserID, faceProfileID, embedding).Scan(&duplicate); err != nil {
			return fmt.Errorf("check duplicate fusion face sample: %w", err)
		}
		if duplicate {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO face_profile_samples (
  id,owner_user_id,face_profile_id,embedding,detection_score,quality,pose_bucket,
  yaw,pitch,roll,quality_score,source_recording_id,source_track_id,observed_at
) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,COALESCE(($6::jsonb->>'score')::double precision,0),$11,$12,NOW())
ON CONFLICT (id) DO NOTHING`, "fusion-face-sample:"+item.id, job.OwnerUserID, faceProfileID,
			embedding, item.detection, item.quality, pose.Bucket, pose.Yaw, pose.Pitch, pose.Roll,
			job.RecordingID, trackID); err != nil {
			return fmt.Errorf("save fusion face sample %q: %w", item.id, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE face_profiles SET sample_count=(
  SELECT COUNT(*) FROM face_profile_samples WHERE owner_user_id=$1 AND face_profile_id=$2
),last_seen_at=NOW(),updated_at=NOW() WHERE owner_user_id=$1 AND id=$2`, job.OwnerUserID, faceProfileID); err != nil {
		return fmt.Errorf("finish fusion face profile: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE face_profiles f SET status='archived',updated_at=NOW()
WHERE f.owner_user_id=$1 AND f.person_profile_id<>$2 AND f.status='provisional'
  AND EXISTS (
    SELECT 1 FROM face_profile_samples sample
    WHERE sample.owner_user_id=f.owner_user_id AND sample.face_profile_id=f.id
      AND sample.source_recording_id=$3 AND sample.source_track_id=$4
  )`, job.OwnerUserID, personID, job.RecordingID, trackID); err != nil {
		return fmt.Errorf("archive superseded provisional face profile: %w", err)
	}
	return nil
}

func (r *activeSpeakerFusionRepository) RetryActiveSpeakerFusion(
	ctx context.Context,
	job service.ActiveSpeakerFusionJob,
	cause string,
	runAt time.Time,
	dead bool,
) error {
	status := "retryable_failed"
	if dead {
		status = "dead"
	}
	_, err := r.db.ExecContext(ctx, `
WITH updated_job AS (
  UPDATE analysis_stage_jobs SET status=$3,run_at=$4,locked_at=NULL,last_error=$5,updated_at=NOW()
  WHERE id=$1 AND analysis_run_id=$2 AND status='processing' RETURNING analysis_run_id
)
UPDATE analysis_runs a SET status=$3,last_error=$5,updated_at=NOW()
FROM updated_job j WHERE a.id=j.analysis_run_id`, job.ID, job.AnalysisRunID, status, runAt, cause)
	if err != nil {
		return fmt.Errorf("retry active-speaker fusion: %w", err)
	}
	return nil
}

var _ service.ActiveSpeakerFusionRepository = (*activeSpeakerFusionRepository)(nil)
