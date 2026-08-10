-- +goose Up
CREATE TABLE voice_speaker_profiles (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'provisional'
        CHECK (status IN ('provisional', 'confirmed', 'archived')),
    display_name TEXT NOT NULL DEFAULT '',
    relationship_category TEXT NOT NULL DEFAULT ''
        CHECK (relationship_category IN ('', 'family', 'friend', 'colleague', 'professional', 'acquaintance', 'other')),
    relationship_label TEXT NOT NULL DEFAULT '',
    embedding_model TEXT NOT NULL,
    embedding_dimensions INTEGER NOT NULL CHECK (embedding_dimensions > 0),
    centroid DOUBLE PRECISION[] NOT NULL,
    sample_count INTEGER NOT NULL DEFAULT 1 CHECK (sample_count > 0),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (cardinality(centroid) = embedding_dimensions),
    CHECK (
        (status = 'provisional' AND expires_at IS NOT NULL) OR
        (status <> 'provisional' AND expires_at IS NULL)
    )
);

CREATE INDEX voice_speaker_profiles_owner_status_idx
    ON voice_speaker_profiles(owner_user_id, status, last_seen_at DESC);
CREATE INDEX voice_speaker_profiles_matching_idx
    ON voice_speaker_profiles(owner_user_id, embedding_model, embedding_dimensions)
    WHERE status <> 'archived';

CREATE TABLE voice_speaker_samples (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES voice_speaker_profiles(id) ON DELETE CASCADE,
    source_kind TEXT NOT NULL CHECK (source_kind IN ('voice', 'video')),
    source_recording_id TEXT NOT NULL,
    provider_speaker TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_path TEXT NOT NULL UNIQUE,
    media_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    duration_seconds DOUBLE PRECISION NOT NULL CHECK (duration_seconds >= 2 AND duration_seconds <= 10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT voice_speaker_samples_source_unique
        UNIQUE (owner_user_id, source_kind, source_recording_id, provider_speaker)
);

CREATE INDEX voice_speaker_samples_profile_idx
    ON voice_speaker_samples(profile_id, created_at DESC);

CREATE TABLE voice_speaker_observations (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES voice_speaker_profiles(id) ON DELETE CASCADE,
    source_kind TEXT NOT NULL CHECK (source_kind IN ('voice', 'video')),
    source_recording_id TEXT NOT NULL,
    provider_speaker TEXT NOT NULL,
    segment_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    outcome TEXT NOT NULL CHECK (outcome IN ('created', 'matched')),
    similarity DOUBLE PRECISION,
    runner_up_similarity DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT voice_speaker_observations_source_unique
        UNIQUE (owner_user_id, source_kind, source_recording_id, provider_speaker)
);

CREATE INDEX voice_speaker_observations_profile_idx
    ON voice_speaker_observations(profile_id, created_at DESC);

-- +goose Down
DROP TABLE voice_speaker_observations;
DROP TABLE voice_speaker_samples;
DROP TABLE voice_speaker_profiles;
