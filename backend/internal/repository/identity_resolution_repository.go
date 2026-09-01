package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/arham/ai-second-brain/internal/service"
)

func (r *personRepository) ResolveAutomaticIdentity(
	ctx context.Context,
	input service.AutomaticIdentityEvidenceInput,
) (service.AutomaticIdentityResolution, error) {
	database, ok := r.db.(interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		return service.AutomaticIdentityResolution{}, fmt.Errorf("automatic identity resolution requires transaction support")
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return service.AutomaticIdentityResolution{}, fmt.Errorf("begin automatic identity resolution: %w", err)
	}
	defer tx.Rollback()

	segmentIDs, err := json.Marshal(input.SegmentIDs)
	if err != nil {
		return service.AutomaticIdentityResolution{}, fmt.Errorf("encode identity segment evidence: %w", err)
	}
	const saveEvidence = `
INSERT INTO identity_link_evidence (
    owner_user_id,recording_id,face_profile_id,person_track_id,voice_speaker_profile_id,
    diarized_segment_ids,active_speaker_score,visible_mouth_coverage,temporal_coverage,
    overlapping_speaker_conflict,decision,face_provider,face_model,
    active_speaker_provider,active_speaker_model,processing_version
)
SELECT $1,$2,(
        SELECT f.id FROM person_tracks t
        JOIN face_profiles f ON f.owner_user_id=t.owner_user_id
          AND f.person_profile_id=t.resolved_person_profile_id
        WHERE t.owner_user_id=$1 AND t.recording_id=$2 AND t.id=$3
        ORDER BY f.updated_at DESC LIMIT 1
    ),$3,$4,$5::jsonb,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15
ON CONFLICT (owner_user_id,recording_id,person_track_id,voice_speaker_profile_id,processing_version)
DO UPDATE SET diarized_segment_ids=EXCLUDED.diarized_segment_ids,
    active_speaker_score=EXCLUDED.active_speaker_score,
    visible_mouth_coverage=EXCLUDED.visible_mouth_coverage,
    temporal_coverage=EXCLUDED.temporal_coverage,
    overlapping_speaker_conflict=EXCLUDED.overlapping_speaker_conflict,
    decision=CASE WHEN identity_link_evidence.decision='accepted' THEN 'accepted' ELSE EXCLUDED.decision END,
    updated_at=NOW()
RETURNING id`
	var evidenceID string
	if err := tx.QueryRowContext(
		ctx, saveEvidence, input.OwnerUserID, input.RecordingID, input.PersonTrackID,
		input.VoiceSpeakerProfileID, segmentIDs, input.ActiveSpeakerScore,
		input.VisibleMouthCoverage, input.TemporalCoverage, input.OverlappingConflict,
		input.Decision, input.FaceProvider, input.FaceModel, input.ActiveSpeakerProvider,
		input.ActiveSpeakerModel, input.ProcessingVersion,
	).Scan(&evidenceID); err != nil {
		return service.AutomaticIdentityResolution{}, fmt.Errorf("store automatic identity evidence: %w", err)
	}
	resolution := service.AutomaticIdentityResolution{
		PersonTrackID: input.PersonTrackID, VoiceSpeakerProfileID: input.VoiceSpeakerProfileID,
		Decision: input.Decision,
	}
	if input.Decision != "accepted" {
		if err := tx.Commit(); err != nil {
			return resolution, fmt.Errorf("commit identity suggestion: %w", err)
		}
		return resolution, nil
	}

	facePersonID, speakerPersonID, err := automaticIdentityTargets(ctx, tx, input)
	if err != nil {
		return resolution, err
	}
	if facePersonID == "" {
		return resolution, fmt.Errorf("automatic identity track has no resolved face person")
	}
	if speakerPersonID == "" {
		person, err := confirmAutomaticallyLinkedPerson(
			ctx, tx, input.OwnerUserID, facePersonID, input.VoiceSpeakerProfileID,
		)
		if err != nil {
			return resolution, err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE voice_speaker_profiles SET person_profile_id=$3,updated_at=NOW()
WHERE owner_user_id=$1 AND id=$2 AND person_profile_id IS NULL`, input.OwnerUserID, input.VoiceSpeakerProfileID, facePersonID); err != nil {
			return resolution, fmt.Errorf("link automatic speaker identity: %w", err)
		}
		confirmation := service.ConfirmPersonIdentityInput{
			OwnerUserID: input.OwnerUserID, VoiceSpeakerProfileID: input.VoiceSpeakerProfileID,
		}
		videoIDs, err := canonicalizeSpeakerInVideo(ctx, tx, confirmation, person)
		if err != nil {
			return resolution, err
		}
		if err := canonicalizeSpeakerInVoice(ctx, tx, confirmation, person); err != nil {
			return resolution, err
		}
		if err := requeueVideoGraphs(ctx, tx, uniqueStrings(videoIDs)); err != nil {
			return resolution, err
		}
		resolution.PersonProfileID, resolution.DisplayName = person.ID, person.DisplayName
		if err := tx.Commit(); err != nil {
			return resolution, fmt.Errorf("commit automatic identity link: %w", err)
		}
		return resolution, nil
	}
	if facePersonID == speakerPersonID {
		person, err := loadIdentityPerson(ctx, tx, input.OwnerUserID, facePersonID)
		if err != nil {
			return resolution, err
		}
		confirmation := service.ConfirmPersonIdentityInput{
			OwnerUserID: input.OwnerUserID, VoiceSpeakerProfileID: input.VoiceSpeakerProfileID,
		}
		videoIDs, err := canonicalizeSpeakerInVideo(ctx, tx, confirmation, person)
		if err != nil {
			return resolution, err
		}
		if err := canonicalizeSpeakerInVoice(ctx, tx, confirmation, person); err != nil {
			return resolution, err
		}
		if err := requeueVideoGraphs(ctx, tx, uniqueStrings(videoIDs)); err != nil {
			return resolution, err
		}
		resolution.PersonProfileID, resolution.DisplayName = person.ID, person.DisplayName
		if err := tx.Commit(); err != nil {
			return resolution, fmt.Errorf("commit matching identity evidence: %w", err)
		}
		return resolution, nil
	}

	facePerson, err := loadIdentityPerson(ctx, tx, input.OwnerUserID, facePersonID)
	if err != nil {
		return resolution, err
	}
	speakerPerson, err := loadIdentityPerson(ctx, tx, input.OwnerUserID, speakerPersonID)
	if err != nil {
		return resolution, err
	}
	target, source := preferredCanonicalPerson(facePerson, speakerPerson)
	candidateID, candidateSourceID, candidateTargetID, evidenceCount, err := upsertMergeCandidate(
		ctx, tx, input.OwnerUserID, source.ID, target.ID, evidenceID, input.RecordingID,
	)
	if err != nil {
		return resolution, err
	}
	if candidateSourceID != source.ID || candidateTargetID != target.ID {
		target, err = loadIdentityPerson(ctx, tx, input.OwnerUserID, candidateTargetID)
		if err != nil {
			return resolution, err
		}
		source, err = loadIdentityPerson(ctx, tx, input.OwnerUserID, candidateSourceID)
		if err != nil {
			return resolution, err
		}
	}
	resolution.PersonProfileID, resolution.DisplayName = target.ID, target.DisplayName
	if !input.AutoMerge || evidenceCount < input.MergeEvidenceRequirement {
		resolution.Decision = "suggested"
		if err := tx.Commit(); err != nil {
			return resolution, fmt.Errorf("commit person merge candidate: %w", err)
		}
		return resolution, nil
	}
	if err := mergePersonAlias(ctx, tx, input.OwnerUserID, source.ID, target, candidateID); err != nil {
		return resolution, err
	}
	resolution.MergedFromProfileID, resolution.Decision = source.ID, "accepted"
	if err := tx.Commit(); err != nil {
		return resolution, fmt.Errorf("commit automatic person merge: %w", err)
	}
	return resolution, nil
}

func confirmAutomaticallyLinkedPerson(
	ctx context.Context, tx *sql.Tx, ownerUserID, personID, speakerID string,
) (service.PersonProfile, error) {
	const query = `
WITH speaker AS MATERIALIZED (
    SELECT * FROM voice_speaker_profiles WHERE owner_user_id=$1 AND id=$3
), updated AS (
    UPDATE person_profiles person SET
        status='confirmed',display_name=COALESCE(NULLIF(person.display_name,''),speaker.display_name),
        relationship_category=COALESCE(NULLIF(person.relationship_category,''),speaker.relationship_category),
        relationship_label=COALESCE(NULLIF(person.relationship_label,''),speaker.relationship_label),
        expires_at=NULL,updated_at=NOW()
    FROM speaker WHERE person.owner_user_id=$1 AND person.id=$2
      AND person.status NOT IN ('rejected','archived') RETURNING person.*
), faces AS (
    UPDATE face_profiles face SET status='confirmed',expires_at=NULL,updated_at=NOW()
    FROM updated person WHERE face.owner_user_id=$1 AND face.person_profile_id=person.id RETURNING face.id
)
SELECT id,status,display_name,relationship_category,relationship_label,consent_state,
       first_seen_at,last_seen_at,expires_at,created_at,updated_at FROM updated`
	var person service.PersonProfile
	err := tx.QueryRowContext(ctx, query, ownerUserID, personID, speakerID).Scan(
		&person.ID, &person.Status, &person.DisplayName, &person.RelationshipCategory,
		&person.RelationshipLabel, &person.ConsentState, &person.FirstSeenAt,
		&person.LastSeenAt, &person.ExpiresAt, &person.CreatedAt, &person.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.PersonProfile{}, service.ErrNotFound
	}
	if err != nil {
		return service.PersonProfile{}, fmt.Errorf("confirm automatically linked person: %w", err)
	}
	return person, nil
}

func automaticIdentityTargets(
	ctx context.Context, tx *sql.Tx, input service.AutomaticIdentityEvidenceInput,
) (string, string, error) {
	const query = `
SELECT COALESCE(face_alias.canonical_person_profile_id,t.resolved_person_profile_id,''),
       COALESCE(voice_alias.canonical_person_profile_id,s.person_profile_id,'')
FROM person_tracks t
JOIN voice_speaker_profiles s ON s.owner_user_id=t.owner_user_id AND s.id=$4
LEFT JOIN person_profile_aliases face_alias
  ON face_alias.owner_user_id=t.owner_user_id AND face_alias.alias_person_profile_id=t.resolved_person_profile_id
LEFT JOIN person_profile_aliases voice_alias
  ON voice_alias.owner_user_id=s.owner_user_id AND voice_alias.alias_person_profile_id=s.person_profile_id
WHERE t.owner_user_id=$1 AND t.recording_id=$2 AND t.id=$3 AND t.processing_version=$5`
	var facePersonID, speakerPersonID string
	err := tx.QueryRowContext(
		ctx, query, input.OwnerUserID, input.RecordingID, input.PersonTrackID,
		input.VoiceSpeakerProfileID, input.ProcessingVersion,
	).Scan(&facePersonID, &speakerPersonID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", service.ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("resolve automatic identity targets: %w", err)
	}
	return facePersonID, speakerPersonID, nil
}

func loadIdentityPerson(ctx context.Context, tx *sql.Tx, ownerUserID, personID string) (service.PersonProfile, error) {
	const query = `
SELECT id,status,display_name,relationship_category,relationship_label,consent_state,
       first_seen_at,last_seen_at,expires_at,created_at,updated_at
FROM person_profiles
WHERE owner_user_id=$1 AND id=$2 AND status NOT IN ('rejected','archived') FOR UPDATE`
	var person service.PersonProfile
	err := tx.QueryRowContext(ctx, query, ownerUserID, personID).Scan(
		&person.ID, &person.Status, &person.DisplayName, &person.RelationshipCategory,
		&person.RelationshipLabel, &person.ConsentState, &person.FirstSeenAt,
		&person.LastSeenAt, &person.ExpiresAt, &person.CreatedAt, &person.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.PersonProfile{}, service.ErrNotFound
	}
	if err != nil {
		return service.PersonProfile{}, fmt.Errorf("load identity person: %w", err)
	}
	return person, nil
}

func preferredCanonicalPerson(left, right service.PersonProfile) (service.PersonProfile, service.PersonProfile) {
	score := func(person service.PersonProfile) int {
		value := 0
		if person.Status == "confirmed" {
			value += 2
		}
		if person.DisplayName != "" {
			value++
		}
		return value
	}
	leftScore, rightScore := score(left), score(right)
	if rightScore > leftScore || (rightScore == leftScore && right.CreatedAt.Before(left.CreatedAt)) {
		return right, left
	}
	return left, right
}

func upsertMergeCandidate(
	ctx context.Context, tx *sql.Tx, ownerUserID, sourceID, targetID, evidenceID, recordingID string,
) (string, string, string, int, error) {
	const query = `
WITH candidate AS (
    INSERT INTO person_merge_candidates (owner_user_id,source_person_profile_id,target_person_profile_id)
    VALUES ($1,$2,$3)
    ON CONFLICT DO UPDATE SET updated_at=NOW()
    RETURNING id,source_person_profile_id,target_person_profile_id
), evidence AS (
    INSERT INTO person_merge_candidate_evidence (candidate_id,identity_link_evidence_id,recording_id)
    SELECT id,$4,$5 FROM candidate
    ON CONFLICT (candidate_id,recording_id) DO NOTHING
	RETURNING candidate_id
), counted AS (
    UPDATE person_merge_candidates merge SET evidence_count=(
        SELECT COUNT(*) FROM person_merge_candidate_evidence item WHERE item.candidate_id=merge.id
	)+(SELECT COUNT(*) FROM evidence),updated_at=NOW()
    FROM candidate WHERE merge.id=candidate.id
    RETURNING merge.id,merge.evidence_count
)
SELECT counted.id,candidate.source_person_profile_id,candidate.target_person_profile_id,counted.evidence_count
FROM counted,candidate WHERE counted.id=candidate.id`
	var candidateID, candidateSourceID, candidateTargetID string
	var evidenceCount int
	if err := tx.QueryRowContext(ctx, query, ownerUserID, sourceID, targetID, evidenceID, recordingID).Scan(
		&candidateID, &candidateSourceID, &candidateTargetID, &evidenceCount,
	); err != nil {
		return "", "", "", 0, fmt.Errorf("upsert person merge candidate: %w", err)
	}
	return candidateID, candidateSourceID, candidateTargetID, evidenceCount, nil
}

func mergePersonAlias(
	ctx context.Context, tx *sql.Tx, ownerUserID, sourceID string,
	target service.PersonProfile, candidateID string,
) error {
	const merge = `
WITH flattened AS (
    UPDATE person_profile_aliases SET canonical_person_profile_id=$3
    WHERE owner_user_id=$1 AND canonical_person_profile_id=$2 RETURNING alias_person_profile_id
), inserted_alias AS (
    INSERT INTO person_profile_aliases (
        owner_user_id,alias_person_profile_id,canonical_person_profile_id,merge_candidate_id
    ) VALUES ($1,$2,$3,$4)
    ON CONFLICT (owner_user_id,alias_person_profile_id) DO UPDATE SET
        canonical_person_profile_id=EXCLUDED.canonical_person_profile_id,
        merge_candidate_id=EXCLUDED.merge_candidate_id
    RETURNING alias_person_profile_id
), archived AS (
    UPDATE person_profiles SET status='archived',expires_at=NULL,updated_at=NOW()
    WHERE owner_user_id=$1 AND id=$2 RETURNING id
), tracks AS (
    UPDATE person_tracks SET resolved_person_profile_id=$3,updated_at=NOW()
    WHERE owner_user_id=$1 AND resolved_person_profile_id=$2 RETURNING id
), actions AS (
    UPDATE action_events SET resolved_person_profile_id=$3,updated_at=NOW()
    WHERE owner_user_id=$1 AND resolved_person_profile_id=$2 RETURNING id
), accepted AS (
    UPDATE person_merge_candidates SET state='accepted',updated_at=NOW()
    WHERE id=$4 RETURNING id
)
SELECT COUNT(*) FROM accepted`
	var merged int
	if err := tx.QueryRowContext(ctx, merge, ownerUserID, sourceID, target.ID, candidateID).Scan(&merged); err != nil {
		return fmt.Errorf("merge canonical person alias: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
UPDATE voice_speaker_profiles SET person_profile_id=$3,updated_at=NOW()
WHERE owner_user_id=$1 AND person_profile_id=$2 RETURNING id`, ownerUserID, sourceID, target.ID)
	if err != nil {
		return fmt.Errorf("move merged voice profiles: %w", err)
	}
	var speakerIDs []string
	for rows.Next() {
		var speakerID string
		if err := rows.Scan(&speakerID); err != nil {
			rows.Close()
			return err
		}
		speakerIDs = append(speakerIDs, speakerID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, speakerID := range speakerIDs {
		confirmation := service.ConfirmPersonIdentityInput{OwnerUserID: ownerUserID, VoiceSpeakerProfileID: speakerID}
		videoIDs, err := canonicalizeSpeakerInVideo(ctx, tx, confirmation, target)
		if err != nil {
			return err
		}
		if err := canonicalizeSpeakerInVoice(ctx, tx, confirmation, target); err != nil {
			return err
		}
		if err := requeueVideoGraphs(ctx, tx, uniqueStrings(videoIDs)); err != nil {
			return err
		}
	}
	return canonicalizeMergedVisualPerson(ctx, tx, ownerUserID, sourceID, target)
}

func canonicalizeMergedVisualPerson(
	ctx context.Context, tx *sql.Tx, ownerUserID, sourceID string, target service.PersonProfile,
) error {
	const query = `
WITH recordings AS MATERIALIZED (
    SELECT id FROM video_recordings r
    WHERE r.owner_user_id=$1 AND EXISTS (
        SELECT 1 FROM jsonb_array_elements(COALESCE(r.visual_analysis->'observations','[]'::jsonb)) observation,
                      jsonb_array_elements(COALESCE(observation->'people','[]'::jsonb)) person
        WHERE person->>'person_profile_id'=$2
    )
), updated_recordings AS (
    UPDATE video_recordings r SET visual_analysis=jsonb_set(
        r.visual_analysis,'{observations}',(
            SELECT jsonb_agg(jsonb_set(observation.value,'{people}',COALESCE((
                SELECT jsonb_agg(CASE WHEN person.value->>'person_profile_id'=$2
                    THEN person.value||jsonb_build_object(
                        'person_profile_id',$3::text,'person_name',$4::text,'person_identity_status','confirmed'
                    ) ELSE person.value END ORDER BY person.ordinality)
                FROM jsonb_array_elements(COALESCE(observation.value->'people','[]'::jsonb))
                     WITH ORDINALITY person(value,ordinality)
            ),'[]'::jsonb),true) ORDER BY observation.ordinality)
            FROM jsonb_array_elements(COALESCE(r.visual_analysis->'observations','[]'::jsonb))
                 WITH ORDINALITY observation(value,ordinality)
        ),true
    ),status='memograph_pending',updated_at=NOW()
    FROM recordings WHERE r.id=recordings.id RETURNING r.id
), updated_episodes AS (
    UPDATE video_episodes e SET visual_observations=(
        SELECT jsonb_agg(jsonb_set(observation.value,'{people}',COALESCE((
            SELECT jsonb_agg(CASE WHEN person.value->>'person_profile_id'=$2
                THEN person.value||jsonb_build_object(
                    'person_profile_id',$3::text,'person_name',$4::text,'person_identity_status','confirmed'
                ) ELSE person.value END ORDER BY person.ordinality)
            FROM jsonb_array_elements(COALESCE(observation.value->'people','[]'::jsonb))
                 WITH ORDINALITY person(value,ordinality)
        ),'[]'::jsonb),true) ORDER BY observation.ordinality)
        FROM jsonb_array_elements(COALESCE(e.visual_observations,'[]'::jsonb))
             WITH ORDINALITY observation(value,ordinality)
    ),graph_revision=e.graph_revision+1,status='queued',last_error='',updated_at=NOW()
    WHERE e.recording_id IN (SELECT id FROM updated_recordings) RETURNING e.id
), jobs AS (
    UPDATE video_jobs job SET status='queued',attempts=0,run_at=NOW(),locked_at=NULL,last_error='',updated_at=NOW()
    FROM updated_episodes episode WHERE job.kind='memograph' AND job.episode_id=episode.id RETURNING job.id
)
SELECT COUNT(*) FROM jobs`
	var count int
	if err := tx.QueryRowContext(ctx, query, ownerUserID, sourceID, target.ID, target.DisplayName).Scan(&count); err != nil {
		return fmt.Errorf("canonicalize merged visual person: %w", err)
	}
	return nil
}
