-- +goose Up
CREATE TABLE person_profiles (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'provisional'
        CHECK (status IN ('provisional','confirmed','rejected','archived')),
    display_name TEXT NOT NULL DEFAULT '',
    relationship_category TEXT NOT NULL DEFAULT ''
        CHECK (relationship_category IN ('','family','friend','colleague','professional','acquaintance','other')),
    relationship_label TEXT NOT NULL DEFAULT '',
    consent_state TEXT NOT NULL DEFAULT 'pending'
        CHECK (consent_state IN ('pending','granted','revoked')),
    retention_state TEXT NOT NULL DEFAULT 'standard'
        CHECK (retention_state IN ('standard','restricted','deletion_requested')),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (owner_user_id, id),
    CHECK ((status = 'provisional' AND expires_at IS NOT NULL) OR status <> 'provisional')
);
CREATE INDEX person_profiles_owner_status_idx
    ON person_profiles(owner_user_id, status, last_seen_at DESC);

ALTER TABLE voice_speaker_profiles
    ADD COLUMN person_profile_id TEXT,
    ADD CONSTRAINT voice_speaker_profiles_owner_id_unique UNIQUE (owner_user_id, id),
    ADD CONSTRAINT voice_speaker_profiles_person_fk
        FOREIGN KEY (owner_user_id, person_profile_id)
        REFERENCES person_profiles(owner_user_id, id) ON DELETE SET NULL (person_profile_id);

CREATE TABLE face_profiles (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    person_profile_id TEXT,
    status TEXT NOT NULL DEFAULT 'provisional'
        CHECK (status IN ('provisional','confirmed','rejected','archived')),
    provider TEXT NOT NULL,
    detector_model TEXT NOT NULL,
    embedding_model TEXT NOT NULL,
    embedding_dimensions INTEGER NOT NULL CHECK (embedding_dimensions > 0),
    centroid DOUBLE PRECISION[],
    managed_provider_identity TEXT,
    sample_count INTEGER NOT NULL DEFAULT 1 CHECK (sample_count > 0),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (owner_user_id, id),
    UNIQUE (owner_user_id, person_profile_id, provider, embedding_model),
    FOREIGN KEY (owner_user_id, person_profile_id)
        REFERENCES person_profiles(owner_user_id, id) ON DELETE CASCADE,
    CHECK ((centroid IS NOT NULL) <> (managed_provider_identity IS NOT NULL)),
    CHECK (centroid IS NULL OR cardinality(centroid) = embedding_dimensions)
);
CREATE INDEX face_profiles_matching_idx
    ON face_profiles(owner_user_id, provider, embedding_model, embedding_dimensions)
    WHERE status NOT IN ('rejected','archived');

CREATE TABLE face_profile_samples (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    face_profile_id TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_path TEXT NOT NULL UNIQUE,
    media_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    embedding DOUBLE PRECISION[] NOT NULL,
    detection_score DOUBLE PRECISION NOT NULL CHECK (detection_score BETWEEN 0 AND 1),
    quality JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (owner_user_id, face_profile_id)
        REFERENCES face_profiles(owner_user_id, id) ON DELETE CASCADE
);
CREATE INDEX face_profile_samples_profile_idx
    ON face_profile_samples(owner_user_id, face_profile_id, created_at DESC);

ALTER TABLE video_recordings
    ADD CONSTRAINT video_recordings_owner_id_unique UNIQUE (owner_user_id, id);
ALTER TABLE media_assets
    ADD CONSTRAINT media_assets_owner_id_unique UNIQUE (owner_user_id, id);

CREATE TABLE person_tracks (
    id TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recording_id TEXT NOT NULL,
    start_time DOUBLE PRECISION NOT NULL CHECK (start_time >= 0),
    end_time DOUBLE PRECISION NOT NULL CHECK (end_time >= start_time),
    face_track_provider_ref TEXT NOT NULL DEFAULT '',
    temporary_visual_label TEXT NOT NULL,
    resolved_person_profile_id TEXT,
    tracking_confidence DOUBLE PRECISION NOT NULL CHECK (tracking_confidence BETWEEN 0 AND 1),
    evidence_frame_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    processing_version INTEGER NOT NULL CHECK (processing_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (owner_user_id, recording_id, id, processing_version),
    FOREIGN KEY (owner_user_id, recording_id)
        REFERENCES video_recordings(owner_user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (owner_user_id, resolved_person_profile_id)
        REFERENCES person_profiles(owner_user_id, id) ON DELETE SET NULL (resolved_person_profile_id)
);
CREATE INDEX person_tracks_recording_time_idx
    ON person_tracks(owner_user_id, recording_id, start_time, end_time);

CREATE TABLE object_tracks (
    id TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recording_id TEXT NOT NULL,
    canonical_label TEXT NOT NULL,
    attributes JSONB NOT NULL DEFAULT '[]'::jsonb,
    start_time DOUBLE PRECISION NOT NULL CHECK (start_time >= 0),
    end_time DOUBLE PRECISION NOT NULL CHECK (end_time >= start_time),
    tracking_confidence DOUBLE PRECISION NOT NULL CHECK (tracking_confidence BETWEEN 0 AND 1),
    evidence_frame_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    processing_version INTEGER NOT NULL CHECK (processing_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (owner_user_id, recording_id, id, processing_version),
    FOREIGN KEY (owner_user_id, recording_id)
        REFERENCES video_recordings(owner_user_id, id) ON DELETE CASCADE
);
CREATE INDEX object_tracks_recording_time_idx
    ON object_tracks(owner_user_id, recording_id, start_time, end_time);

CREATE TABLE identity_link_evidence (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recording_id TEXT NOT NULL,
    face_profile_id TEXT,
    person_track_id TEXT NOT NULL,
    voice_speaker_profile_id TEXT NOT NULL,
    diarized_segment_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    active_speaker_score DOUBLE PRECISION CHECK (active_speaker_score BETWEEN 0 AND 1),
    visible_mouth_coverage DOUBLE PRECISION CHECK (visible_mouth_coverage BETWEEN 0 AND 1),
    face_match_score DOUBLE PRECISION CHECK (face_match_score BETWEEN -1 AND 1),
    face_runner_up_score DOUBLE PRECISION CHECK (face_runner_up_score BETWEEN -1 AND 1),
    voice_match_score DOUBLE PRECISION CHECK (voice_match_score BETWEEN -1 AND 1),
    voice_runner_up_score DOUBLE PRECISION CHECK (voice_runner_up_score BETWEEN -1 AND 1),
    temporal_coverage DOUBLE PRECISION NOT NULL CHECK (temporal_coverage BETWEEN 0 AND 1),
    overlapping_speaker_conflict BOOLEAN NOT NULL DEFAULT FALSE,
    decision TEXT NOT NULL DEFAULT 'suggested'
        CHECK (decision IN ('suggested','accepted','rejected','ambiguous')),
    face_provider TEXT NOT NULL,
    face_model TEXT NOT NULL,
    active_speaker_provider TEXT NOT NULL,
    active_speaker_model TEXT NOT NULL,
    processing_version INTEGER NOT NULL CHECK (processing_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (owner_user_id, recording_id, person_track_id, voice_speaker_profile_id, processing_version),
    FOREIGN KEY (owner_user_id, recording_id)
        REFERENCES video_recordings(owner_user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (owner_user_id, recording_id, person_track_id, processing_version)
        REFERENCES person_tracks(owner_user_id, recording_id, id, processing_version) ON DELETE CASCADE,
    FOREIGN KEY (owner_user_id, face_profile_id)
        REFERENCES face_profiles(owner_user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (owner_user_id, voice_speaker_profile_id)
        REFERENCES voice_speaker_profiles(owner_user_id, id) ON DELETE CASCADE
);
CREATE INDEX identity_link_review_idx
    ON identity_link_evidence(owner_user_id, decision, created_at DESC);

CREATE TABLE action_events (
    id TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recording_id TEXT NOT NULL,
    actor_person_track_id TEXT NOT NULL,
    resolved_person_profile_id TEXT,
    secondary_actor_track_id TEXT,
    action_type TEXT NOT NULL CHECK (action_type IN (
        'REACH_FOR','TOUCH','GRASP','PICK_UP','HOLD','CARRY','PUT_DOWN',
        'DROP','HAND_OVER','RECEIVE','UNKNOWN_INTERACTION'
    )),
    object_track_id TEXT NOT NULL,
    start_time DOUBLE PRECISION NOT NULL CHECK (start_time >= 0),
    end_time DOUBLE PRECISION NOT NULL CHECK (end_time >= start_time),
    before_state TEXT NOT NULL CHECK (before_state IN (
        'resting_on_surface','approached','contact_only','controlled_by_actor',
        'moving_with_actor','released_on_surface','falling','occluded','unknown'
    )),
    after_state TEXT NOT NULL CHECK (after_state IN (
        'resting_on_surface','approached','contact_only','controlled_by_actor',
        'moving_with_actor','released_on_surface','falling','occluded','unknown'
    )),
    overall_confidence DOUBLE PRECISION NOT NULL CHECK (overall_confidence BETWEEN 0 AND 1),
    actor_tracking_confidence DOUBLE PRECISION NOT NULL CHECK (actor_tracking_confidence BETWEEN 0 AND 1),
    object_tracking_confidence DOUBLE PRECISION NOT NULL CHECK (object_tracking_confidence BETWEEN 0 AND 1),
    transition_confidence DOUBLE PRECISION NOT NULL CHECK (transition_confidence BETWEEN 0 AND 1),
    identity_confidence DOUBLE PRECISION CHECK (identity_confidence BETWEEN 0 AND 1),
    temporal_coverage DOUBLE PRECISION NOT NULL CHECK (temporal_coverage BETWEEN 0 AND 1),
    ambiguity_reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    evidence_frame_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    observation_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_media_asset_id TEXT NOT NULL,
    temporal_provider TEXT NOT NULL,
    temporal_model TEXT NOT NULL,
    processing_version INTEGER NOT NULL CHECK (processing_version > 0),
    review_status TEXT NOT NULL DEFAULT 'unreviewed'
        CHECK (review_status IN ('unreviewed','confirmed','corrected','rejected','ambiguous')),
    corrected_action_type TEXT CHECK (corrected_action_type IS NULL OR corrected_action_type IN (
        'REACH_FOR','TOUCH','GRASP','PICK_UP','HOLD','CARRY','PUT_DOWN',
        'DROP','HAND_OVER','RECEIVE','UNKNOWN_INTERACTION'
    )),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (owner_user_id, recording_id, id, processing_version),
    FOREIGN KEY (owner_user_id, recording_id)
        REFERENCES video_recordings(owner_user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (owner_user_id, recording_id, actor_person_track_id, processing_version)
        REFERENCES person_tracks(owner_user_id, recording_id, id, processing_version) ON DELETE CASCADE,
    FOREIGN KEY (owner_user_id, recording_id, secondary_actor_track_id, processing_version)
        REFERENCES person_tracks(owner_user_id, recording_id, id, processing_version) ON DELETE CASCADE,
    FOREIGN KEY (owner_user_id, recording_id, object_track_id, processing_version)
        REFERENCES object_tracks(owner_user_id, recording_id, id, processing_version) ON DELETE CASCADE,
    FOREIGN KEY (owner_user_id, source_media_asset_id)
        REFERENCES media_assets(owner_user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (owner_user_id, resolved_person_profile_id)
        REFERENCES person_profiles(owner_user_id, id) ON DELETE SET NULL (resolved_person_profile_id)
);
CREATE INDEX action_events_recording_time_idx
    ON action_events(owner_user_id, recording_id, start_time, end_time);
CREATE INDEX action_events_review_idx
    ON action_events(owner_user_id, review_status, created_at DESC);

CREATE TABLE temporal_jobs (
    id BIGSERIAL PRIMARY KEY,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recording_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','processing','completed','dead')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL CHECK (max_attempts > 0),
    run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    warning TEXT NOT NULL DEFAULT '',
    processing_version INTEGER NOT NULL CHECK (processing_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (recording_id, processing_version),
    FOREIGN KEY (owner_user_id, recording_id)
        REFERENCES video_recordings(owner_user_id, id) ON DELETE CASCADE
);
CREATE INDEX temporal_jobs_claim_idx ON temporal_jobs(status, run_at, id);

-- +goose Down
DROP TABLE temporal_jobs;
DROP TABLE action_events;
DROP TABLE identity_link_evidence;
DROP TABLE object_tracks;
DROP TABLE person_tracks;
ALTER TABLE media_assets DROP CONSTRAINT media_assets_owner_id_unique;
ALTER TABLE video_recordings DROP CONSTRAINT video_recordings_owner_id_unique;
DROP TABLE face_profile_samples;
DROP TABLE face_profiles;
ALTER TABLE voice_speaker_profiles
    DROP CONSTRAINT voice_speaker_profiles_person_fk,
    DROP CONSTRAINT voice_speaker_profiles_owner_id_unique,
    DROP COLUMN person_profile_id;
DROP TABLE person_profiles;
