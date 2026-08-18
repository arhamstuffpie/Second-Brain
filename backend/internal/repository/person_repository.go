package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/arham/ai-second-brain/internal/service"
)

type PersonRepository interface {
	service.PersonRepository
}

type personRepository struct{ *base }

func newPersonRepository(base *base) *personRepository { return &personRepository{base: base} }

func (r *personRepository) EnrollFace(ctx context.Context, input service.EnrollFaceProfileInput) (service.PersonProfile, error) {
	if input.Pose.Bucket == "" {
		input.Pose.Bucket = "frontal"
	}
	if input.ObservedAt.IsZero() {
		input.ObservedAt = time.Now().UTC()
	}
	quality, err := json.Marshal(input.Quality)
	if err != nil {
		return service.PersonProfile{}, fmt.Errorf("encode face quality: %w", err)
	}
	const query = `
WITH account_lock AS MATERIALIZED (
    SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
), existing_person AS MATERIALIZED (
    SELECT p.* FROM person_profiles p, account_lock
    WHERE p.owner_user_id=$1 AND p.id=NULLIF($2, '')
      AND p.status NOT IN ('rejected','archived')
), created_person AS (
    INSERT INTO person_profiles (
        owner_user_id, status, display_name, relationship_category,
        relationship_label, consent_state, expires_at
    )
    SELECT $1, CASE WHEN NULLIF($3, '') IS NULL THEN 'provisional' ELSE 'confirmed' END,
           $3, $4, $5, COALESCE(NULLIF($17, ''), 'pending'),
           CASE WHEN NULLIF($3, '') IS NULL THEN NOW() + $16::interval ELSE NULL END
    FROM account_lock WHERE NULLIF($2, '') IS NULL
    RETURNING *
), selected_person AS MATERIALIZED (
    SELECT * FROM existing_person UNION ALL SELECT * FROM created_person
), existing_face AS MATERIALIZED (
    SELECT f.* FROM face_profiles f, selected_person p
    WHERE f.owner_user_id=$1 AND f.person_profile_id=p.id AND f.provider=$6
      AND f.embedding_model=$8 AND f.embedding_dimensions=cardinality($9::double precision[])
      AND f.status NOT IN ('rejected','archived')
), updated_face AS (
    UPDATE face_profiles f SET
        centroid=ARRAY(
            SELECT ((f.centroid[i] * f.sample_count) + ($9::double precision[])[i]) / (f.sample_count + 1)
            FROM generate_subscripts(f.centroid, 1) AS indexer(i)
        ), sample_count=f.sample_count+1, last_seen_at=NOW(), updated_at=NOW()
    FROM existing_face e WHERE f.id=e.id RETURNING f.*
), created_face AS (
    INSERT INTO face_profiles (
        owner_user_id, person_profile_id, status, provider, detector_model,
        embedding_model, embedding_dimensions, centroid, expires_at
    )
    SELECT $1, p.id, p.status, $6, $7, $8, cardinality($9::double precision[]),
           $9::double precision[], p.expires_at
    FROM selected_person p WHERE NOT EXISTS (SELECT 1 FROM updated_face)
    RETURNING *
), selected_face AS MATERIALIZED (
    SELECT * FROM updated_face UNION ALL SELECT * FROM created_face
), inserted_sample AS (
    INSERT INTO face_profile_samples (
        owner_user_id, face_profile_id, file_name, file_path, media_type,
        size_bytes, embedding, detection_score, quality, pose_bucket, yaw, pitch, roll,
        quality_score, source_recording_id, source_track_id, observed_at
    )
    SELECT $1, f.id, NULLIF($10,''), NULLIF($11,''), NULLIF($12,''), NULLIF($13,0),
           $9::double precision[], $14, $15::jsonb, $18, $19, $20, $21, $22,
           NULLIF($23,''), NULLIF($24,''), $25
    FROM selected_face f
    WHERE (SELECT COUNT(*) FROM face_profile_samples existing
           WHERE existing.owner_user_id=$1 AND existing.face_profile_id=f.id
             AND existing.pose_bucket=$18) < 5
      AND NOT EXISTS (
          SELECT 1 FROM face_profile_samples existing
          WHERE existing.owner_user_id=$1 AND existing.face_profile_id=f.id
            AND existing.pose_bucket=$18
            AND (SELECT SUM(pair.x * pair.y) /
                        NULLIF(SQRT(SUM(pair.x * pair.x)) * SQRT(SUM(pair.y * pair.y)), 0)
                 FROM unnest(existing.embedding, $9::double precision[]) AS pair(x,y)) >= 0.995
      )
    RETURNING id
), updated_existing_person AS (
    UPDATE person_profiles p SET last_seen_at=NOW(), updated_at=NOW(),
        consent_state=CASE WHEN $17='granted' THEN 'granted' ELSE p.consent_state END
    FROM existing_person selected
    WHERE p.id=selected.id AND p.owner_user_id=$1 RETURNING p.*
), final_person AS MATERIALIZED (
    SELECT created.* FROM created_person created
    UNION ALL SELECT * FROM updated_existing_person
)
SELECT p.id, p.status, p.display_name, p.relationship_category, p.relationship_label,
       p.consent_state, p.first_seen_at, p.last_seen_at, p.expires_at, p.created_at, p.updated_at,
       (SELECT COUNT(*) FROM face_profiles f WHERE f.owner_user_id=$1 AND f.person_profile_id=p.id),
       COALESCE((SELECT jsonb_agg(v.id ORDER BY v.id) FROM voice_speaker_profiles v
                 WHERE v.owner_user_id=$1 AND v.person_profile_id=p.id), '[]'::jsonb)
FROM final_person p`
	var person service.PersonProfile
	var voiceIDs []byte
	err = r.db.QueryRowContext(
		ctx, query, input.OwnerUserID, input.PersonProfileID, input.DisplayName,
		input.RelationshipCategory, input.RelationshipLabel, input.Provider,
		input.DetectorModel, input.EmbeddingModel, input.Embedding, input.FileName,
		input.FilePath, input.MediaType, input.SizeBytes, input.DetectionScore, quality,
		postgresInterval(input.ProvisionalTTL), input.ConsentState,
		input.Pose.Bucket, input.Pose.Yaw, input.Pose.Pitch, input.Pose.Roll,
		input.Quality.Score, input.SourceRecordingID, input.SourceTrackID, input.ObservedAt,
	).Scan(
		&person.ID, &person.Status, &person.DisplayName, &person.RelationshipCategory,
		&person.RelationshipLabel, &person.ConsentState, &person.FirstSeenAt,
		&person.LastSeenAt, &person.ExpiresAt, &person.CreatedAt, &person.UpdatedAt,
		&person.FaceProfileCount, &voiceIDs,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.PersonProfile{}, service.ErrNotFound
	}
	if err != nil {
		return service.PersonProfile{}, fmt.Errorf("enroll face profile: %w", err)
	}
	if err := json.Unmarshal(voiceIDs, &person.VoiceProfileIDs); err != nil {
		return service.PersonProfile{}, fmt.Errorf("decode linked voice profiles: %w", err)
	}
	return person, nil
}

func (r *personRepository) MatchFace(ctx context.Context, input service.MatchFaceProfileInput) (service.FaceMatch, error) {
	const query = `
WITH sample_scores AS MATERIALIZED (
    SELECT f.person_profile_id, s.pose_bucket,
           (SELECT SUM(pair.x * pair.y) /
                          NULLIF(SQRT(SUM(pair.x * pair.x)) * SQRT(SUM(pair.y * pair.y)), 0)
            FROM unnest(s.embedding, $4::double precision[]) AS pair(x, y)) AS score
    FROM face_profiles f JOIN face_profile_samples s
      ON s.owner_user_id=f.owner_user_id AND s.face_profile_id=f.id
    WHERE f.owner_user_id=$1 AND f.provider=$2 AND f.embedding_model=$3
      AND f.embedding_dimensions=cardinality($4::double precision[])
      AND f.person_profile_id IS NOT NULL AND f.status NOT IN ('rejected','archived')
      AND (f.expires_at IS NULL OR f.expires_at > NOW())
), candidates AS MATERIALIZED (
    SELECT DISTINCT ON (person_profile_id) person_profile_id, score
    FROM sample_scores
    ORDER BY person_profile_id,
      score + CASE
        WHEN pose_bucket=$5 THEN 0.02
        WHEN split_part(pose_bucket,'_',1)=split_part($5,'_',1) THEN 0.01
        ELSE 0
      END DESC NULLS LAST
), ranked AS MATERIALIZED (
    SELECT person_profile_id, score, ROW_NUMBER() OVER (ORDER BY score DESC NULLS LAST, person_profile_id) rank
    FROM candidates
)
SELECT p.id, p.display_name, p.status, best.score,
       (SELECT score FROM ranked WHERE rank=2),
       best.score >= $6 AND best.score - COALESCE((SELECT score FROM ranked WHERE rank=2), -1) >= $7
FROM ranked best JOIN person_profiles p ON p.owner_user_id=$1 AND p.id=best.person_profile_id
WHERE best.rank=1`
	var match service.FaceMatch
	var accepted bool
	err := r.db.QueryRowContext(
		ctx, query, input.OwnerUserID, input.Provider, input.EmbeddingModel,
		input.Embedding, input.PoseBucket, input.MatchThreshold, input.AmbiguousMargin,
	).Scan(
		&match.PersonProfileID, &match.DisplayName, &match.IdentityStatus,
		&match.Similarity, &match.RunnerUpSimilarity, &accepted,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.FaceMatch{Reasons: []string{"no_compatible_face_profiles"}}, nil
	}
	if err != nil {
		return service.FaceMatch{}, fmt.Errorf("match face profile: %w", err)
	}
	match.Matched = accepted
	match.Ambiguous = !accepted
	if !accepted {
		match.PersonProfileID, match.DisplayName, match.IdentityStatus = "", "", ""
		match.Reasons = []string{"match_threshold_or_runner_up_margin_not_met"}
	}
	return match, nil
}

func (r *personRepository) SavePersonTrack(ctx context.Context, input service.SavePersonTrackInput) error {
	evidence, err := json.Marshal(input.EvidenceFrameIDs)
	if err != nil {
		return fmt.Errorf("encode person track evidence: %w", err)
	}
	const query = `
INSERT INTO person_tracks (
    id,owner_user_id,recording_id,start_time,end_time,face_track_provider_ref,
    temporary_visual_label,resolved_person_profile_id,tracking_confidence,
    evidence_frame_ids,processing_version
) VALUES ($1,$2,$3,$4,$5,'local-face-gallery-v1',$6,NULLIF($7,''),$8,$9::jsonb,$10)
ON CONFLICT (id) DO UPDATE SET
    end_time=GREATEST(person_tracks.end_time,EXCLUDED.end_time),
    resolved_person_profile_id=COALESCE(person_tracks.resolved_person_profile_id,EXCLUDED.resolved_person_profile_id),
    tracking_confidence=LEAST(person_tracks.tracking_confidence,EXCLUDED.tracking_confidence),
    evidence_frame_ids=EXCLUDED.evidence_frame_ids,updated_at=NOW()`
	if _, err := r.db.ExecContext(
		ctx, query, input.ID, input.OwnerUserID, input.RecordingID, input.StartTime,
		input.EndTime, input.TemporaryVisualLabel, input.ResolvedPersonProfileID,
		input.TrackingConfidence, evidence, input.ProcessingVersion,
	); err != nil {
		return fmt.Errorf("save person track: %w", err)
	}
	return nil
}

func (r *personRepository) ConfirmIdentity(
	ctx context.Context,
	input service.ConfirmPersonIdentityInput,
) (service.PersonProfile, error) {
	database, ok := r.db.(interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		return service.PersonProfile{}, fmt.Errorf("identity confirmation requires transaction support")
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return service.PersonProfile{}, fmt.Errorf("begin identity confirmation: %w", err)
	}
	defer tx.Rollback()

	person, err := linkSpeakerToPerson(ctx, tx, input)
	if err != nil {
		return service.PersonProfile{}, err
	}
	videoRecordingIDs, err := canonicalizeSpeakerInVideo(ctx, tx, input, person)
	if err != nil {
		return service.PersonProfile{}, err
	}
	if err := canonicalizeSpeakerInVoice(ctx, tx, input, person); err != nil {
		return service.PersonProfile{}, err
	}
	for _, recordingID := range input.RecordingIDs {
		if err := confirmVisualPerson(ctx, tx, input, person, recordingID); err != nil {
			return service.PersonProfile{}, err
		}
	}
	videoRecordingIDs = append(videoRecordingIDs, input.RecordingIDs...)
	if err := requeueVideoGraphs(ctx, tx, uniqueStrings(videoRecordingIDs)); err != nil {
		return service.PersonProfile{}, err
	}
	if err := tx.Commit(); err != nil {
		return service.PersonProfile{}, fmt.Errorf("commit identity confirmation: %w", err)
	}
	return person, nil
}

func linkSpeakerToPerson(
	ctx context.Context,
	tx *sql.Tx,
	input service.ConfirmPersonIdentityInput,
) (service.PersonProfile, error) {
	const query = `
WITH speaker AS MATERIALIZED (
    SELECT * FROM voice_speaker_profiles
    WHERE id=$2 AND owner_user_id=$1 AND status='confirmed' AND display_name<>''
    FOR UPDATE
), existing_person AS MATERIALIZED (
    SELECT p.* FROM person_profiles p JOIN speaker s ON s.person_profile_id=p.id
    WHERE p.owner_user_id=$1 AND p.status NOT IN ('rejected','archived')
), created_person AS (
    INSERT INTO person_profiles (
        owner_user_id,status,display_name,relationship_category,relationship_label,consent_state
    )
    SELECT $1,'confirmed',s.display_name,s.relationship_category,s.relationship_label,'pending'
    FROM speaker s WHERE s.person_profile_id IS NULL
    RETURNING *
), selected_person AS MATERIALIZED (
    SELECT * FROM existing_person UNION ALL SELECT * FROM created_person
), linked_speaker AS (
    UPDATE voice_speaker_profiles s SET person_profile_id=p.id,updated_at=NOW()
    FROM selected_person p WHERE s.id=$2 AND s.owner_user_id=$1 RETURNING s.id
)
SELECT p.id,p.status,p.display_name,p.relationship_category,p.relationship_label,
       p.consent_state,p.first_seen_at,p.last_seen_at,p.expires_at,p.created_at,p.updated_at,
       (SELECT COUNT(*) FROM face_profiles f WHERE f.owner_user_id=$1 AND f.person_profile_id=p.id),
       COALESCE((SELECT jsonb_agg(v.id ORDER BY v.id) FROM voice_speaker_profiles v,linked_speaker
                 WHERE v.owner_user_id=$1 AND v.person_profile_id=p.id),'[]'::jsonb)
FROM selected_person p`
	var person service.PersonProfile
	var voiceIDs []byte
	err := tx.QueryRowContext(ctx, query, input.OwnerUserID, input.VoiceSpeakerProfileID).Scan(
		&person.ID, &person.Status, &person.DisplayName, &person.RelationshipCategory,
		&person.RelationshipLabel, &person.ConsentState, &person.FirstSeenAt,
		&person.LastSeenAt, &person.ExpiresAt, &person.CreatedAt, &person.UpdatedAt,
		&person.FaceProfileCount, &voiceIDs,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.PersonProfile{}, service.ErrNotFound
	}
	if err != nil {
		return service.PersonProfile{}, fmt.Errorf("link speaker to person: %w", err)
	}
	if err := json.Unmarshal(voiceIDs, &person.VoiceProfileIDs); err != nil {
		return service.PersonProfile{}, fmt.Errorf("decode linked voice profiles: %w", err)
	}
	return person, nil
}

func canonicalizeSpeakerInVideo(
	ctx context.Context,
	tx *sql.Tx,
	input service.ConfirmPersonIdentityInput,
	person service.PersonProfile,
) ([]string, error) {
	const query = `
UPDATE video_recordings r SET transcript=jsonb_set(
    r.transcript,'{segments}',(
        SELECT jsonb_agg(
            CASE WHEN segment.value->>'speaker_profile_id'=$2
                 THEN segment.value||jsonb_build_object(
                     'person_profile_id',$3::text,'speaker_name',$4::text,'speaker_identity_status','confirmed'
                 ) ELSE segment.value END ORDER BY segment.ordinality
        ) FROM jsonb_array_elements(COALESCE(r.transcript->'segments','[]'::jsonb))
             WITH ORDINALITY segment(value,ordinality)
    ),true
),updated_at=NOW()
WHERE r.owner_user_id=$1 AND r.transcript IS NOT NULL
  AND EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(r.transcript->'segments','[]'::jsonb)) s
              WHERE s->>'speaker_profile_id'=$2)
RETURNING r.id`
	rows, err := tx.QueryContext(
		ctx, query, input.OwnerUserID, input.VoiceSpeakerProfileID, person.ID, person.DisplayName,
	)
	if err != nil {
		return nil, fmt.Errorf("canonicalize video speaker: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func canonicalizeSpeakerInVoice(
	ctx context.Context,
	tx *sql.Tx,
	input service.ConfirmPersonIdentityInput,
	person service.PersonProfile,
) error {
	const query = `
WITH updated_recordings AS (
    UPDATE voice_recordings r SET transcript=jsonb_set(
        r.transcript,'{segments}',(
            SELECT jsonb_agg(
                CASE WHEN segment.value->>'speaker_profile_id'=$2
                     THEN segment.value||jsonb_build_object(
                         'person_profile_id',$3::text,'speaker_name',$4::text,'speaker_identity_status','confirmed'
                     ) ELSE segment.value END ORDER BY segment.ordinality
            ) FROM jsonb_array_elements(COALESCE(r.transcript->'segments','[]'::jsonb))
                 WITH ORDINALITY segment(value,ordinality)
        ),true
    ),status='memograph_pending',updated_at=NOW()
    WHERE r.owner_user_id=$1 AND r.transcript IS NOT NULL
      AND EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(r.transcript->'segments','[]'::jsonb)) s
                  WHERE s->>'speaker_profile_id'=$2)
    RETURNING r.id
), updated_episodes AS (
    UPDATE voice_episodes e SET segments=(
        SELECT jsonb_agg(
            CASE WHEN segment.value->>'speaker_profile_id'=$2
                 THEN segment.value||jsonb_build_object(
                     'person_profile_id',$3::text,'speaker_name',$4::text,'speaker_identity_status','confirmed'
                 ) ELSE segment.value END ORDER BY segment.ordinality
        ) FROM jsonb_array_elements(COALESCE(e.segments,'[]'::jsonb))
             WITH ORDINALITY segment(value,ordinality)
    ),graph_revision=e.graph_revision+1,status='queued',last_error='',updated_at=NOW()
    FROM voice_episode_batches b
    WHERE e.batch_id=b.id AND b.owner_user_id=$1
      AND EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(e.segments,'[]'::jsonb)) s
                  WHERE s->>'speaker_profile_id'=$2)
    RETURNING e.id
), requeued_jobs AS (
    UPDATE voice_jobs j SET status='queued',attempts=0,run_at=NOW(),locked_at=NULL,last_error='',updated_at=NOW()
    FROM updated_episodes e WHERE j.kind='memograph' AND j.episode_id=e.id RETURNING j.id
)
SELECT COUNT(*) FROM requeued_jobs`
	var count int
	if err := tx.QueryRowContext(
		ctx, query, input.OwnerUserID, input.VoiceSpeakerProfileID, person.ID, person.DisplayName,
	).Scan(&count); err != nil {
		return fmt.Errorf("canonicalize voice speaker: %w", err)
	}
	return nil
}

func confirmVisualPerson(
	ctx context.Context,
	tx *sql.Tx,
	input service.ConfirmPersonIdentityInput,
	person service.PersonProfile,
	recordingID string,
) error {
	const updateRecording = `
UPDATE video_recordings r SET visual_analysis=jsonb_set(
    r.visual_analysis,'{observations}',(
        SELECT jsonb_agg(
            jsonb_set(observation.value,'{people}',COALESCE((
                SELECT jsonb_agg(
                    CASE WHEN visual.value->>'visual_label'=$3
                         THEN visual.value||jsonb_build_object(
                             'person_profile_id',$4::text,'person_identity_status','confirmed','person_name',$5::text
                         ) ELSE visual.value END ORDER BY visual.ordinality
                ) FROM jsonb_array_elements(COALESCE(observation.value->'people','[]'::jsonb))
                     WITH ORDINALITY visual(value,ordinality)
            ),'[]'::jsonb),true) ORDER BY observation.ordinality
        ) FROM jsonb_array_elements(COALESCE(r.visual_analysis->'observations','[]'::jsonb))
             WITH ORDINALITY observation(value,ordinality)
    ),true
),updated_at=NOW()
WHERE r.id=$2 AND r.owner_user_id=$1 AND r.visual_analysis IS NOT NULL
  AND EXISTS (
      SELECT 1 FROM jsonb_array_elements(COALESCE(r.visual_analysis->'observations','[]'::jsonb)) o,
                    jsonb_array_elements(COALESCE(o->'people','[]'::jsonb)) p
      WHERE p->>'visual_label'=$3
  )
RETURNING r.processing_version,
    (SELECT MIN((o->>'start_time')::double precision)
     FROM jsonb_array_elements(r.visual_analysis->'observations') o
     WHERE EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(o->'people','[]'::jsonb)) p
                   WHERE p->>'visual_label'=$3)),
    (SELECT MAX((o->>'end_time')::double precision)
     FROM jsonb_array_elements(r.visual_analysis->'observations') o
     WHERE EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(o->'people','[]'::jsonb)) p
                   WHERE p->>'visual_label'=$3)),
    (SELECT COALESCE(jsonb_agg(o->>'frame_id'),'[]'::jsonb)
     FROM jsonb_array_elements(r.visual_analysis->'observations') o
     WHERE EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(o->'people','[]'::jsonb)) p
                   WHERE p->>'visual_label'=$3))`
	var version int
	var startTime, endTime float64
	var frameIDs []byte
	err := tx.QueryRowContext(
		ctx, updateRecording, input.OwnerUserID, recordingID, input.VisualLabel,
		person.ID, person.DisplayName,
	).Scan(&version, &startTime, &endTime, &frameIDs)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("confirm visual person in recording %s: %w", recordingID, err)
	}

	digest := sha256.Sum256([]byte(input.OwnerUserID + "\x00" + recordingID + "\x00" + input.VisualLabel))
	trackID := fmt.Sprintf("manual-person-track:%x", digest[:16])
	const saveEvidence = `
WITH track AS (
    INSERT INTO person_tracks (
        id,owner_user_id,recording_id,start_time,end_time,temporary_visual_label,
        resolved_person_profile_id,tracking_confidence,evidence_frame_ids,processing_version
    ) VALUES ($1,$2,$3,$4,$5,$6,$7,1,$8::jsonb,$9)
    ON CONFLICT (id) DO UPDATE SET
        resolved_person_profile_id=EXCLUDED.resolved_person_profile_id,
        start_time=EXCLUDED.start_time,end_time=EXCLUDED.end_time,
        tracking_confidence=1,evidence_frame_ids=EXCLUDED.evidence_frame_ids,updated_at=NOW()
    RETURNING id
)
INSERT INTO identity_link_evidence (
    owner_user_id,recording_id,person_track_id,voice_speaker_profile_id,
    diarized_segment_ids,temporal_coverage,decision,face_provider,face_model,
    active_speaker_provider,active_speaker_model,processing_version
)
SELECT $2,$3,track.id,$10,'[]'::jsonb,1,'accepted','manual-confirmation','none',
       'manual-confirmation','explicit-v1',$9 FROM track
ON CONFLICT (owner_user_id,recording_id,person_track_id,voice_speaker_profile_id,processing_version)
DO UPDATE SET decision='accepted',updated_at=NOW()`
	if _, err := tx.ExecContext(
		ctx, saveEvidence, trackID, input.OwnerUserID, recordingID, startTime, endTime,
		input.VisualLabel, person.ID, frameIDs, version, input.VoiceSpeakerProfileID,
	); err != nil {
		return fmt.Errorf("store confirmed identity evidence: %w", err)
	}

	const updateEpisodes = `
UPDATE video_episodes e SET visual_observations=(
    SELECT jsonb_agg(
        jsonb_set(observation.value,'{people}',COALESCE((
            SELECT jsonb_agg(
                CASE WHEN visual.value->>'visual_label'=$2
                     THEN visual.value||jsonb_build_object(
                         'person_profile_id',$3::text,'person_identity_status','confirmed','person_name',$4::text
                     ) ELSE visual.value END ORDER BY visual.ordinality
            ) FROM jsonb_array_elements(COALESCE(observation.value->'people','[]'::jsonb))
                 WITH ORDINALITY visual(value,ordinality)
        ),'[]'::jsonb),true) ORDER BY observation.ordinality
    ) FROM jsonb_array_elements(COALESCE(e.visual_observations,'[]'::jsonb))
         WITH ORDINALITY observation(value,ordinality)
),updated_at=NOW()
WHERE e.recording_id=$1 AND EXISTS (
    SELECT 1 FROM jsonb_array_elements(COALESCE(e.visual_observations,'[]'::jsonb)) o,
                  jsonb_array_elements(COALESCE(o->'people','[]'::jsonb)) p
    WHERE p->>'visual_label'=$2
)`
	if _, err := tx.ExecContext(ctx, updateEpisodes, recordingID, input.VisualLabel, person.ID, person.DisplayName); err != nil {
		return fmt.Errorf("canonicalize visual episodes: %w", err)
	}
	return nil
}

func requeueVideoGraphs(ctx context.Context, tx *sql.Tx, recordingIDs []string) error {
	const query = `
WITH updated_episodes AS (
    UPDATE video_episodes SET graph_revision=graph_revision+1,status='queued',last_error='',updated_at=NOW()
    WHERE recording_id=ANY($1::text[]) RETURNING id,recording_id
), requeued_jobs AS (
    UPDATE video_jobs j SET status='queued',attempts=0,run_at=NOW(),locked_at=NULL,last_error='',updated_at=NOW()
    FROM updated_episodes e WHERE j.kind='memograph' AND j.episode_id=e.id RETURNING j.id
)
UPDATE video_recordings SET status='memograph_pending',last_error='',updated_at=NOW()
WHERE id=ANY($1::text[])`
	if _, err := tx.ExecContext(ctx, query, recordingIDs); err != nil {
		return fmt.Errorf("requeue canonical person graphs: %w", err)
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func (r *personRepository) ListPeople(ctx context.Context, ownerUserID string) ([]service.PersonProfile, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT p.id, p.status, p.display_name, p.relationship_category, p.relationship_label,
       p.consent_state, p.first_seen_at, p.last_seen_at, p.expires_at, p.created_at, p.updated_at,
       (SELECT COUNT(*) FROM face_profiles f WHERE f.owner_user_id=$1 AND f.person_profile_id=p.id
          AND f.status NOT IN ('rejected','archived')),
       COALESCE((SELECT jsonb_agg(v.id ORDER BY v.id) FROM voice_speaker_profiles v
                 WHERE v.owner_user_id=$1 AND v.person_profile_id=p.id AND v.status <> 'archived'), '[]'::jsonb)
FROM person_profiles p WHERE p.owner_user_id=$1 AND p.status NOT IN ('rejected','archived')
  AND (p.expires_at IS NULL OR p.expires_at > NOW())
ORDER BY CASE p.status WHEN 'provisional' THEN 0 ELSE 1 END, p.last_seen_at DESC, p.id`, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("list people: %w", err)
	}
	defer rows.Close()
	people := make([]service.PersonProfile, 0)
	for rows.Next() {
		var person service.PersonProfile
		var voiceIDs []byte
		if err := rows.Scan(
			&person.ID, &person.Status, &person.DisplayName, &person.RelationshipCategory,
			&person.RelationshipLabel, &person.ConsentState, &person.FirstSeenAt,
			&person.LastSeenAt, &person.ExpiresAt, &person.CreatedAt, &person.UpdatedAt,
			&person.FaceProfileCount, &voiceIDs,
		); err != nil {
			return nil, fmt.Errorf("scan person profile: %w", err)
		}
		if err := json.Unmarshal(voiceIDs, &person.VoiceProfileIDs); err != nil {
			return nil, fmt.Errorf("decode person voice profiles: %w", err)
		}
		people = append(people, person)
	}
	return people, rows.Err()
}

func (r *personRepository) UpdatePerson(ctx context.Context, input service.UpdatePersonInput) (service.PersonProfile, error) {
	const query = `
WITH updated AS (
    UPDATE person_profiles SET status='confirmed', display_name=$3,
        relationship_category=$4, relationship_label=$5, expires_at=NULL, updated_at=NOW()
    WHERE id=$1 AND owner_user_id=$2 AND status NOT IN ('rejected','archived')
    RETURNING *
)
SELECT p.id, p.status, p.display_name, p.relationship_category, p.relationship_label,
       p.consent_state, p.first_seen_at, p.last_seen_at, p.expires_at, p.created_at, p.updated_at,
       (SELECT COUNT(*) FROM face_profiles f WHERE f.owner_user_id=$2 AND f.person_profile_id=p.id),
       COALESCE((SELECT jsonb_agg(v.id ORDER BY v.id) FROM voice_speaker_profiles v
                 WHERE v.owner_user_id=$2 AND v.person_profile_id=p.id), '[]'::jsonb)
FROM updated p`
	var person service.PersonProfile
	var voiceIDs []byte
	err := r.db.QueryRowContext(ctx, query, input.ID, input.OwnerUserID, input.DisplayName,
		input.RelationshipCategory, input.RelationshipLabel).Scan(
		&person.ID, &person.Status, &person.DisplayName, &person.RelationshipCategory,
		&person.RelationshipLabel, &person.ConsentState, &person.FirstSeenAt,
		&person.LastSeenAt, &person.ExpiresAt, &person.CreatedAt, &person.UpdatedAt,
		&person.FaceProfileCount, &voiceIDs,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.PersonProfile{}, service.ErrNotFound
	}
	if err != nil {
		return service.PersonProfile{}, fmt.Errorf("update person: %w", err)
	}
	if err := json.Unmarshal(voiceIDs, &person.VoiceProfileIDs); err != nil {
		return service.PersonProfile{}, err
	}
	return person, nil
}

func (r *personRepository) DeletePerson(ctx context.Context, id, ownerUserID string) ([]string, error) {
	const query = `
WITH paths AS MATERIALIZED (
    SELECT COALESCE(jsonb_agg(s.file_path), '[]'::jsonb) value
    FROM face_profile_samples s JOIN face_profiles f
      ON f.owner_user_id=s.owner_user_id AND f.id=s.face_profile_id
    WHERE f.owner_user_id=$2 AND f.person_profile_id=$1
), deleted AS (
    DELETE FROM person_profiles WHERE id=$1 AND owner_user_id=$2 RETURNING id
)
SELECT paths.value FROM paths, deleted`
	var payload []byte
	err := r.db.QueryRowContext(ctx, query, id, ownerUserID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("delete person: %w", err)
	}
	var paths []string
	if err := json.Unmarshal(payload, &paths); err != nil {
		return nil, fmt.Errorf("decode deleted face sample paths: %w", err)
	}
	return paths, nil
}

var _ PersonRepository = (*personRepository)(nil)
