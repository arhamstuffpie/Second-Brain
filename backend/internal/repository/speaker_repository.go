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

type SpeakerProfileRepository interface {
	service.SpeakerProfileRepository
}

type speakerProfileRepository struct {
	*base
}

func newSpeakerProfileRepository(base *base) *speakerProfileRepository {
	return &speakerProfileRepository{base: base}
}

func (r *speakerProfileRepository) FindSpeakerObservation(
	ctx context.Context, ownerUserID, sourceKind, sourceRecordingID, providerSpeaker string,
) (service.SpeakerObservation, bool, error) {
	const query = `
SELECT id, profile_id, outcome, similarity, runner_up_similarity
FROM voice_speaker_observations
WHERE owner_user_id=$1 AND source_kind=$2 AND source_recording_id=$3 AND provider_speaker=$4`
	var observation service.SpeakerObservation
	err := r.db.QueryRowContext(ctx, query, ownerUserID, sourceKind, sourceRecordingID, providerSpeaker).Scan(
		&observation.ID, &observation.ProfileID, &observation.Outcome,
		&observation.Similarity, &observation.RunnerUpSimilarity,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.SpeakerObservation{}, false, nil
	}
	if err != nil {
		return service.SpeakerObservation{}, false, fmt.Errorf("find speaker observation: %w", err)
	}
	return observation, true, nil
}

func (r *speakerProfileRepository) ResolveSpeakerProfile(
	ctx context.Context, input service.ResolveSpeakerProfileInput,
) (service.SpeakerProfileResolution, error) {
	const query = `
WITH account_lock AS MATERIALIZED (
    SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
), candidates AS MATERIALIZED (
    SELECT p.id,
           (SELECT SUM(pair.x * pair.y) /
                          NULLIF(SQRT(SUM(pair.x * pair.x)) * SQRT(SUM(pair.y * pair.y)), 0)
            FROM unnest(p.centroid, $3::double precision[]) AS pair(x, y)) AS score
    FROM voice_speaker_profiles p
    CROSS JOIN account_lock
    WHERE p.owner_user_id=$1
      AND p.embedding_model=$2
      AND p.embedding_dimensions=cardinality($3::double precision[])
      AND p.status <> 'archived'
      AND (p.expires_at IS NULL OR p.expires_at > NOW())
), ranked AS MATERIALIZED (
    SELECT id, score, ROW_NUMBER() OVER (ORDER BY score DESC NULLS LAST, id) AS rank
    FROM candidates
), selected AS MATERIALIZED (
    SELECT best.id, best.score,
           (SELECT score FROM ranked WHERE rank=2) AS runner_up_score
    FROM ranked best
    WHERE best.rank=1
      AND best.score >= $4
      AND best.score - COALESCE((SELECT score FROM ranked WHERE rank=2), -1) >= $5
), matched AS (
    UPDATE voice_speaker_profiles p
    SET centroid = ARRAY(
            SELECT ((p.centroid[i] * p.sample_count) + ($3::double precision[])[i]) /
                   (p.sample_count + 1)
            FROM generate_subscripts(p.centroid, 1) AS indexer(i)
        ),
        sample_count = p.sample_count + 1,
        last_seen_at = NOW(),
        expires_at = CASE WHEN p.status='provisional' THEN NOW() + $6::interval ELSE NULL END,
        updated_at = NOW()
    FROM selected s
    WHERE p.id=s.id
	    RETURNING p.id, p.status, p.display_name, p.relationship_category,
	              p.relationship_label, COALESCE(p.person_profile_id,''), p.embedding_model, p.embedding_dimensions,
              p.sample_count, p.first_seen_at, p.last_seen_at, p.expires_at,
              false AS created, s.score, s.runner_up_score
), inserted AS (
    INSERT INTO voice_speaker_profiles (
        owner_user_id, embedding_model, embedding_dimensions, centroid, expires_at
    )
    SELECT $1, $2, cardinality($3::double precision[]), $3::double precision[], NOW() + $6::interval
    FROM account_lock
    WHERE NOT EXISTS (SELECT 1 FROM matched)
      AND (SELECT COUNT(*) FROM voice_speaker_profiles
           WHERE owner_user_id=$1 AND status <> 'archived') < 20
	    RETURNING id, status, display_name, relationship_category, relationship_label,
	              COALESCE(person_profile_id,''),
              embedding_model, embedding_dimensions, sample_count, first_seen_at,
              last_seen_at, expires_at, true AS created,
              NULL::double precision AS score, NULL::double precision AS runner_up_score
)
SELECT * FROM matched
UNION ALL
SELECT * FROM inserted`

	interval := postgresInterval(input.ProvisionalTTL)
	var resolution service.SpeakerProfileResolution
	profile := &resolution.Profile
	err := r.db.QueryRowContext(
		ctx, query, input.OwnerUserID, input.EmbeddingModel, input.Embedding,
		input.MatchThreshold, input.AmbiguousMargin, interval,
	).Scan(
		&profile.ID, &profile.Status, &profile.DisplayName,
		&profile.RelationshipCategory, &profile.RelationshipLabel,
		&profile.PersonProfileID,
		&profile.EmbeddingModel, &profile.EmbeddingDimensions, &profile.SampleCount,
		&profile.FirstSeenAt, &profile.LastSeenAt, &profile.ExpiresAt,
		&resolution.Created, &resolution.Similarity, &resolution.RunnerUpSimilarity,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.SpeakerProfileResolution{}, service.ErrConflict
	}
	if err != nil {
		return service.SpeakerProfileResolution{}, fmt.Errorf("resolve speaker profile: %w", err)
	}
	profile.Samples = []service.SpeakerSample{}
	return resolution, nil
}

func (r *speakerProfileRepository) CreateSpeakerSample(
	ctx context.Context, input service.CreateSpeakerSampleInput,
) (service.SpeakerSample, error) {
	const query = `
INSERT INTO voice_speaker_samples (
    owner_user_id, profile_id, source_kind, source_recording_id, provider_speaker,
    file_name, file_path, media_type, size_bytes, duration_seconds
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (owner_user_id, source_kind, source_recording_id, provider_speaker)
DO UPDATE SET profile_id=voice_speaker_samples.profile_id
RETURNING id, profile_id, file_name, file_path, media_type, size_bytes, duration_seconds, created_at`
	var sample service.SpeakerSample
	err := r.db.QueryRowContext(ctx, query,
		input.OwnerUserID, input.ProfileID, input.SourceKind, input.SourceRecordingID,
		input.ProviderSpeaker, input.FileName, input.FilePath, input.MediaType,
		input.SizeBytes, input.DurationSeconds,
	).Scan(
		&sample.ID, &sample.ProfileID, &sample.FileName, &sample.FilePath,
		&sample.MediaType, &sample.SizeBytes, &sample.DurationSeconds, &sample.CreatedAt,
	)
	if err != nil {
		return service.SpeakerSample{}, fmt.Errorf("create speaker sample: %w", err)
	}
	if sample.ProfileID != input.ProfileID {
		return service.SpeakerSample{}, service.ErrConflict
	}
	return sample, nil
}

func (r *speakerProfileRepository) CreateSpeakerObservation(
	ctx context.Context, input service.CreateSpeakerObservationInput,
) (service.SpeakerObservation, error) {
	segmentIDs, err := json.Marshal(input.SegmentIDs)
	if err != nil {
		return service.SpeakerObservation{}, fmt.Errorf("encode speaker observation segments: %w", err)
	}
	const query = `
INSERT INTO voice_speaker_observations (
    owner_user_id, profile_id, source_kind, source_recording_id, provider_speaker,
    segment_ids, outcome, similarity, runner_up_similarity
) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9)
ON CONFLICT (owner_user_id, source_kind, source_recording_id, provider_speaker)
DO UPDATE SET profile_id=voice_speaker_observations.profile_id
RETURNING id, profile_id, outcome, similarity, runner_up_similarity`
	var observation service.SpeakerObservation
	err = r.db.QueryRowContext(ctx, query,
		input.OwnerUserID, input.ProfileID, input.SourceKind, input.SourceRecordingID,
		input.ProviderSpeaker, segmentIDs, input.Outcome, input.Similarity, input.RunnerUpSimilarity,
	).Scan(
		&observation.ID, &observation.ProfileID, &observation.Outcome,
		&observation.Similarity, &observation.RunnerUpSimilarity,
	)
	if err != nil {
		return service.SpeakerObservation{}, fmt.Errorf("create speaker observation: %w", err)
	}
	if observation.ProfileID != input.ProfileID {
		return service.SpeakerObservation{}, service.ErrConflict
	}
	return observation, nil
}

func (r *speakerProfileRepository) ListSpeakerProfiles(
	ctx context.Context, ownerUserID string,
) ([]service.SpeakerProfile, error) {
	const query = `
SELECT id, status, display_name, relationship_category, relationship_label,
	       COALESCE(person_profile_id,''), embedding_model, embedding_dimensions, sample_count, first_seen_at,
       last_seen_at, expires_at
FROM voice_speaker_profiles
WHERE owner_user_id=$1 AND status <> 'archived'
  AND (expires_at IS NULL OR expires_at > NOW())
ORDER BY CASE status WHEN 'provisional' THEN 0 ELSE 1 END, last_seen_at DESC, id`
	rows, err := r.db.QueryContext(ctx, query, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("list speaker profiles: %w", err)
	}
	profiles := make([]service.SpeakerProfile, 0)
	for rows.Next() {
		var profile service.SpeakerProfile
		if err := rows.Scan(
			&profile.ID, &profile.Status, &profile.DisplayName,
			&profile.RelationshipCategory, &profile.RelationshipLabel,
			&profile.PersonProfileID,
			&profile.EmbeddingModel, &profile.EmbeddingDimensions, &profile.SampleCount,
			&profile.FirstSeenAt, &profile.LastSeenAt, &profile.ExpiresAt,
		); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan speaker profile: %w", err)
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate speaker profiles: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close speaker profile rows: %w", err)
	}
	for index := range profiles {
		profiles[index].Samples, err = r.ListSpeakerSamples(ctx, profiles[index].ID, ownerUserID)
		if err != nil {
			return nil, err
		}
	}
	return profiles, nil
}

func (r *speakerProfileRepository) UpdateSpeakerProfile(
	ctx context.Context, input service.UpdateSpeakerProfileInput,
) (service.SpeakerProfile, error) {
	const query = `
WITH updated_profile AS (
    UPDATE voice_speaker_profiles
    SET status='confirmed', display_name=$3, relationship_category=$4,
        relationship_label=$5, expires_at=NULL, updated_at=NOW()
    WHERE id=$1 AND owner_user_id=$2 AND status <> 'archived'
      AND (expires_at IS NULL OR expires_at > NOW())
    RETURNING id, owner_user_id, status, display_name, relationship_category,
              relationship_label, embedding_model, embedding_dimensions,
              sample_count, first_seen_at, last_seen_at, expires_at
), updated_voice_recordings AS (
    UPDATE voice_recordings r
    SET transcript = jsonb_set(
            r.transcript,
            '{segments}',
            COALESCE((
                SELECT jsonb_agg(
                    CASE WHEN segment.value->>'speaker_profile_id'=p.id
                        THEN segment.value || jsonb_build_object(
                            'speaker_name', p.display_name,
                            'speaker_relationship', COALESCE(NULLIF(p.relationship_label, ''), p.relationship_category),
                            'speaker_identity_status', 'confirmed',
                            'speaker_role', 'other'
                        )
                        ELSE segment.value
                    END ORDER BY segment.ordinality
                )
                FROM jsonb_array_elements(COALESCE(r.transcript->'segments', '[]'::jsonb))
                     WITH ORDINALITY AS segment(value, ordinality)
            ), '[]'::jsonb),
            true
        ),
        updated_at=NOW()
    FROM updated_profile p
    WHERE r.owner_user_id=p.owner_user_id
      AND r.transcript IS NOT NULL
      AND EXISTS (
          SELECT 1 FROM jsonb_array_elements(COALESCE(r.transcript->'segments', '[]'::jsonb)) segment
          WHERE segment->>'speaker_profile_id'=p.id
      )
    RETURNING r.id
), updated_voice_episodes AS (
    UPDATE voice_episodes e
    SET segments = COALESCE((
            SELECT jsonb_agg(
                CASE WHEN segment.value->>'speaker_profile_id'=p.id
                    THEN segment.value || jsonb_build_object(
                        'speaker_name', p.display_name,
                        'speaker_relationship', COALESCE(NULLIF(p.relationship_label, ''), p.relationship_category),
                        'speaker_identity_status', 'confirmed',
                        'speaker_role', 'other'
                    )
                    ELSE segment.value
                END ORDER BY segment.ordinality
            )
            FROM jsonb_array_elements(COALESCE(e.segments, '[]'::jsonb))
                 WITH ORDINALITY AS segment(value, ordinality)
        ), '[]'::jsonb),
        graph_revision=e.graph_revision+1,
        status='queued', last_error='', updated_at=NOW()
    FROM voice_episode_batches b, updated_profile p
    WHERE e.batch_id=b.id
      AND b.owner_user_id=p.owner_user_id
      AND EXISTS (
          SELECT 1 FROM jsonb_array_elements(COALESCE(e.segments, '[]'::jsonb)) segment
          WHERE segment->>'speaker_profile_id'=p.id
      )
    RETURNING e.id
), requeued_voice_jobs AS (
    UPDATE voice_jobs j
    SET status='queued', attempts=0, run_at=NOW(), locked_at=NULL,
        last_error='', updated_at=NOW()
    FROM updated_voice_episodes e
    WHERE j.kind='memograph' AND j.episode_id=e.id
    RETURNING j.episode_id
), updated_voice_status AS (
    UPDATE voice_recordings r
    SET status='memograph_pending', last_error='', updated_at=NOW()
    WHERE EXISTS (
        SELECT 1 FROM voice_episode_recordings er
        JOIN updated_voice_episodes e ON e.id=er.episode_id
        WHERE er.recording_id=r.id
    )
    RETURNING r.id
), updated_video_recordings AS (
    UPDATE video_recordings r
    SET transcript = jsonb_set(
            r.transcript,
            '{segments}',
            COALESCE((
                SELECT jsonb_agg(
                    CASE WHEN segment.value->>'speaker_profile_id'=p.id
                        THEN segment.value || jsonb_build_object(
                            'speaker_name', p.display_name,
                            'speaker_relationship', COALESCE(NULLIF(p.relationship_label, ''), p.relationship_category),
                            'speaker_identity_status', 'confirmed',
                            'speaker_role', 'other'
                        )
                        ELSE segment.value
                    END ORDER BY segment.ordinality
                )
                FROM jsonb_array_elements(COALESCE(r.transcript->'segments', '[]'::jsonb))
                     WITH ORDINALITY AS segment(value, ordinality)
            ), '[]'::jsonb),
            true
        ),
        status='memograph_pending', last_error='', updated_at=NOW()
    FROM updated_profile p
    WHERE r.owner_user_id=p.owner_user_id
      AND r.transcript IS NOT NULL
      AND EXISTS (
          SELECT 1 FROM jsonb_array_elements(COALESCE(r.transcript->'segments', '[]'::jsonb)) segment
          WHERE segment->>'speaker_profile_id'=p.id
      )
    RETURNING r.id
), updated_video_episodes AS (
    UPDATE video_episodes e
    SET graph_revision=e.graph_revision+1,
        status='queued', last_error='', updated_at=NOW()
    WHERE e.recording_id IN (SELECT id FROM updated_video_recordings)
    RETURNING e.id
), requeued_video_jobs AS (
    UPDATE video_jobs j
    SET status='queued', attempts=0, run_at=NOW(), locked_at=NULL,
        last_error='', updated_at=NOW()
    FROM updated_video_episodes e
    WHERE j.kind='memograph' AND j.episode_id=e.id
      AND j.source IN ('speech', 'legacy')
    RETURNING j.episode_id
)
SELECT id, status, display_name, relationship_category, relationship_label,
	       COALESCE(person_profile_id,''), embedding_model, embedding_dimensions, sample_count, first_seen_at,
       last_seen_at, expires_at
FROM updated_profile`
	var profile service.SpeakerProfile
	err := r.db.QueryRowContext(ctx, query,
		input.ID, input.OwnerUserID, input.DisplayName,
		input.RelationshipCategory, input.RelationshipLabel,
	).Scan(
		&profile.ID, &profile.Status, &profile.DisplayName,
		&profile.RelationshipCategory, &profile.RelationshipLabel,
		&profile.PersonProfileID,
		&profile.EmbeddingModel, &profile.EmbeddingDimensions, &profile.SampleCount,
		&profile.FirstSeenAt, &profile.LastSeenAt, &profile.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.SpeakerProfile{}, service.ErrNotFound
	}
	if err != nil {
		return service.SpeakerProfile{}, fmt.Errorf("update speaker profile: %w", err)
	}
	profile.Samples, err = r.ListSpeakerSamples(ctx, profile.ID, input.OwnerUserID)
	if err != nil {
		return service.SpeakerProfile{}, err
	}
	return profile, nil
}

func (r *speakerProfileRepository) DeleteSpeakerProfile(
	ctx context.Context, id, ownerUserID string,
) ([]string, error) {
	const query = `
WITH target AS MATERIALIZED (
    SELECT id FROM voice_speaker_profiles
    WHERE id=$1 AND owner_user_id=$2 AND status <> 'archived'
), paths AS MATERIALIZED (
    SELECT s.file_path FROM voice_speaker_samples s JOIN target t ON t.id=s.profile_id
), deleted AS (
    DELETE FROM voice_speaker_profiles p USING target t
    WHERE p.id=t.id AND (SELECT COUNT(*) FROM paths) >= 0
    RETURNING p.id
)
SELECT file_path, false FROM paths CROSS JOIN deleted
UNION ALL
SELECT '', true FROM deleted WHERE NOT EXISTS (SELECT 1 FROM paths)`
	rows, err := r.db.QueryContext(ctx, query, id, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("delete speaker profile: %w", err)
	}
	defer rows.Close()
	paths := make([]string, 0)
	found := false
	for rows.Next() {
		var path string
		var sentinel bool
		if err := rows.Scan(&path, &sentinel); err != nil {
			return nil, fmt.Errorf("scan archived speaker sample: %w", err)
		}
		found = true
		if !sentinel && path != "" {
			paths = append(paths, path)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived speaker samples: %w", err)
	}
	if !found {
		return nil, service.ErrNotFound
	}
	return paths, nil
}

func (r *speakerProfileRepository) PurgeExpiredSpeakerProfiles(
	ctx context.Context, ownerUserID string,
) ([]string, error) {
	const query = `
WITH targets AS MATERIALIZED (
    SELECT id FROM voice_speaker_profiles
    WHERE owner_user_id=$1 AND status='provisional' AND expires_at <= NOW()
), paths AS MATERIALIZED (
    SELECT s.file_path FROM voice_speaker_samples s JOIN targets t ON t.id=s.profile_id
), deleted AS (
    DELETE FROM voice_speaker_profiles p USING targets t
    WHERE p.id=t.id AND (SELECT COUNT(*) FROM paths) >= 0
    RETURNING p.id
)
SELECT file_path FROM paths CROSS JOIN (SELECT COUNT(*) FROM deleted) completed
UNION ALL
SELECT '' FROM deleted WHERE NOT EXISTS (SELECT 1 FROM paths)`
	rows, err := r.db.QueryContext(ctx, query, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("purge expired speaker profiles: %w", err)
	}
	defer rows.Close()
	paths := make([]string, 0)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("scan expired speaker sample: %w", err)
		}
		if path != "" {
			paths = append(paths, path)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired speaker samples: %w", err)
	}
	return paths, nil
}

func (r *speakerProfileRepository) ListSpeakerSamples(
	ctx context.Context, profileID, ownerUserID string,
) ([]service.SpeakerSample, error) {
	const query = `
SELECT id, profile_id, file_name, file_path, media_type, size_bytes, duration_seconds, created_at
FROM voice_speaker_samples
WHERE profile_id=$1 AND owner_user_id=$2
ORDER BY created_at, id`
	rows, err := r.db.QueryContext(ctx, query, profileID, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("list speaker samples: %w", err)
	}
	defer rows.Close()
	result := make([]service.SpeakerSample, 0)
	for rows.Next() {
		var sample service.SpeakerSample
		if err := rows.Scan(
			&sample.ID, &sample.ProfileID, &sample.FileName, &sample.FilePath,
			&sample.MediaType, &sample.SizeBytes, &sample.DurationSeconds, &sample.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan speaker sample: %w", err)
		}
		result = append(result, sample)
	}
	return result, rows.Err()
}

func (r *speakerProfileRepository) GetSpeakerSample(
	ctx context.Context, id, profileID, ownerUserID string,
) (service.SpeakerSample, error) {
	const query = `
SELECT s.id, s.profile_id, s.file_name, s.file_path, s.media_type,
       s.size_bytes, s.duration_seconds, s.created_at
FROM voice_speaker_samples s
JOIN voice_speaker_profiles p ON p.id=s.profile_id
WHERE s.id=$1 AND s.profile_id=$2 AND s.owner_user_id=$3 AND p.status <> 'archived'
  AND (p.expires_at IS NULL OR p.expires_at > NOW())`
	var sample service.SpeakerSample
	err := r.db.QueryRowContext(ctx, query, id, profileID, ownerUserID).Scan(
		&sample.ID, &sample.ProfileID, &sample.FileName, &sample.FilePath,
		&sample.MediaType, &sample.SizeBytes, &sample.DurationSeconds, &sample.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return service.SpeakerSample{}, service.ErrNotFound
	}
	if err != nil {
		return service.SpeakerSample{}, fmt.Errorf("get speaker sample: %w", err)
	}
	return sample, nil
}

func postgresInterval(duration time.Duration) string {
	return fmt.Sprintf("%f seconds", duration.Seconds())
}

var _ service.SpeakerProfileRepository = (*speakerProfileRepository)(nil)
