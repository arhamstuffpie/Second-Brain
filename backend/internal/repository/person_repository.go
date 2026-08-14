package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/arham/ai-second-brain/internal/service"
)

type PersonRepository interface {
	service.PersonRepository
}

type personRepository struct{ *base }

func newPersonRepository(base *base) *personRepository { return &personRepository{base: base} }

func (r *personRepository) EnrollFace(ctx context.Context, input service.EnrollFaceProfileInput) (service.PersonProfile, error) {
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
           $3, $4, $5, 'granted',
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
        size_bytes, embedding, detection_score, quality
    )
    SELECT $1, f.id, $10, $11, $12, $13, $9::double precision[], $14, $15::jsonb
    FROM selected_face f RETURNING id
), updated_existing_person AS (
    UPDATE person_profiles p SET last_seen_at=NOW(), updated_at=NOW()
    FROM existing_person selected, inserted_sample
    WHERE p.id=selected.id AND p.owner_user_id=$1 RETURNING p.*
), final_person AS MATERIALIZED (
    SELECT created.* FROM created_person created, inserted_sample
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
		postgresInterval(input.ProvisionalTTL),
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
WITH candidates AS MATERIALIZED (
    SELECT f.person_profile_id,
           (SELECT SUM(pair.x * pair.y) /
                          NULLIF(SQRT(SUM(pair.x * pair.x)) * SQRT(SUM(pair.y * pair.y)), 0)
            FROM unnest(f.centroid, $4::double precision[]) AS pair(x, y)) AS score
    FROM face_profiles f
    WHERE f.owner_user_id=$1 AND f.provider=$2 AND f.embedding_model=$3
      AND f.embedding_dimensions=cardinality($4::double precision[])
      AND f.person_profile_id IS NOT NULL AND f.status NOT IN ('rejected','archived')
      AND (f.expires_at IS NULL OR f.expires_at > NOW())
), ranked AS MATERIALIZED (
    SELECT person_profile_id, score, ROW_NUMBER() OVER (ORDER BY score DESC NULLS LAST, person_profile_id) rank
    FROM candidates
)
SELECT p.id, p.display_name, p.status, best.score,
       (SELECT score FROM ranked WHERE rank=2),
       best.score >= $5 AND best.score - COALESCE((SELECT score FROM ranked WHERE rank=2), -1) >= $6
FROM ranked best JOIN person_profiles p ON p.owner_user_id=$1 AND p.id=best.person_profile_id
WHERE best.rank=1`
	var match service.FaceMatch
	var accepted bool
	err := r.db.QueryRowContext(
		ctx, query, input.OwnerUserID, input.Provider, input.EmbeddingModel,
		input.Embedding, input.MatchThreshold, input.AmbiguousMargin,
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
