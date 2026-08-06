-- +goose Up
CREATE TABLE model_profiles (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    task TEXT NOT NULL CHECK (task IN ('transcription')),
    provider TEXT NOT NULL,
    base_url TEXT NOT NULL,
    model TEXT NOT NULL,
    api_key_ciphertext TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT model_profiles_owner_task_unique UNIQUE (owner_user_id, task)
);

CREATE INDEX model_profiles_owner_idx ON model_profiles(owner_user_id);

-- +goose Down
DROP TABLE model_profiles;
