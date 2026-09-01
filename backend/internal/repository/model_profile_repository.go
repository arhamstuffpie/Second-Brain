package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/arham/ai-second-brain/internal/service"
)

type ModelProfileRepository interface {
	service.ModelProfileRepository
}

type modelProfileRepository struct {
	*base
}

func newModelProfileRepository(base *base) *modelProfileRepository {
	return &modelProfileRepository{base: base}
}

func (r *modelProfileRepository) Get(
	ctx context.Context,
	ownerUserID, task string,
) (service.StoredModelProfile, bool, error) {
	const query = `
SELECT id, owner_user_id, task, provider, base_url, model,
       api_key_ciphertext, created_at, updated_at
FROM model_profiles
WHERE owner_user_id = $1 AND task = $2`
	profile, err := scanStoredModelProfile(r.db.QueryRowContext(ctx, query, ownerUserID, task))
	if errors.Is(err, sql.ErrNoRows) {
		return service.StoredModelProfile{}, false, nil
	}
	if err != nil {
		return service.StoredModelProfile{}, false, fmt.Errorf("get model profile: %w", err)
	}
	return profile, true, nil
}

func (r *modelProfileRepository) Upsert(
	ctx context.Context,
	profile service.StoredModelProfile,
) (service.StoredModelProfile, error) {
	const query = `
INSERT INTO model_profiles (
    owner_user_id, task, provider, base_url, model, api_key_ciphertext
) VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (owner_user_id, task) DO UPDATE SET
    provider = EXCLUDED.provider,
    base_url = EXCLUDED.base_url,
    model = EXCLUDED.model,
    api_key_ciphertext = EXCLUDED.api_key_ciphertext,
    updated_at = NOW()
RETURNING id, owner_user_id, task, provider, base_url, model,
          api_key_ciphertext, created_at, updated_at`
	result, err := scanStoredModelProfile(r.db.QueryRowContext(
		ctx, query, profile.OwnerUserID, profile.Task, profile.Provider,
		profile.BaseURL, profile.Model, profile.APIKeyCiphertext,
	))
	if err != nil {
		return service.StoredModelProfile{}, fmt.Errorf("upsert model profile: %w", err)
	}
	return result, nil
}

func (r *modelProfileRepository) Delete(ctx context.Context, ownerUserID, task string) error {
	const query = `DELETE FROM model_profiles WHERE owner_user_id = $1 AND task = $2`
	if _, err := r.db.ExecContext(ctx, query, ownerUserID, task); err != nil {
		return fmt.Errorf("delete model profile: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanStoredModelProfile(row rowScanner) (service.StoredModelProfile, error) {
	var profile service.StoredModelProfile
	err := row.Scan(
		&profile.ID, &profile.OwnerUserID, &profile.Task, &profile.Provider,
		&profile.BaseURL, &profile.Model, &profile.APIKeyCiphertext,
		&profile.CreatedAt, &profile.UpdatedAt,
	)
	return profile, err
}

var _ ModelProfileRepository = (*modelProfileRepository)(nil)
