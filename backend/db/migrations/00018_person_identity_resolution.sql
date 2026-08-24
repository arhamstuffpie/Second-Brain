-- +goose Up
CREATE TABLE person_merge_candidates (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_person_profile_id TEXT NOT NULL,
    target_person_profile_id TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'needs_more_evidence'
        CHECK (state IN ('needs_more_evidence','accepted','rejected')),
    evidence_count INTEGER NOT NULL DEFAULT 0 CHECK (evidence_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (source_person_profile_id <> target_person_profile_id),
    FOREIGN KEY (owner_user_id, source_person_profile_id)
        REFERENCES person_profiles(owner_user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (owner_user_id, target_person_profile_id)
        REFERENCES person_profiles(owner_user_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX person_merge_candidates_pair_unique
    ON person_merge_candidates(
        owner_user_id,
        LEAST(source_person_profile_id,target_person_profile_id),
        GREATEST(source_person_profile_id,target_person_profile_id)
    );
CREATE INDEX person_merge_candidates_review_idx
    ON person_merge_candidates(owner_user_id, state, evidence_count DESC, updated_at DESC);

CREATE TABLE person_merge_candidate_evidence (
    candidate_id TEXT NOT NULL REFERENCES person_merge_candidates(id) ON DELETE CASCADE,
    identity_link_evidence_id TEXT NOT NULL REFERENCES identity_link_evidence(id) ON DELETE CASCADE,
    recording_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (candidate_id, recording_id),
    UNIQUE (identity_link_evidence_id)
);

CREATE TABLE person_profile_aliases (
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    alias_person_profile_id TEXT NOT NULL,
    canonical_person_profile_id TEXT NOT NULL,
    merge_candidate_id TEXT REFERENCES person_merge_candidates(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (owner_user_id, alias_person_profile_id),
    CHECK (alias_person_profile_id <> canonical_person_profile_id),
    FOREIGN KEY (owner_user_id, alias_person_profile_id)
        REFERENCES person_profiles(owner_user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (owner_user_id, canonical_person_profile_id)
        REFERENCES person_profiles(owner_user_id, id) ON DELETE CASCADE
);
CREATE INDEX person_profile_aliases_canonical_idx
    ON person_profile_aliases(owner_user_id, canonical_person_profile_id);

-- +goose Down
DROP TABLE person_profile_aliases;
DROP TABLE person_merge_candidate_evidence;
DROP TABLE person_merge_candidates;
