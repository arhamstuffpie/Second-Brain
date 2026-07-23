package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	dbsqlc "github.com/arham/ai-second-brain/internal/db/sqlc"
	"github.com/arham/ai-second-brain/internal/service"
)

type UserRepository interface {
	Create(ctx context.Context, email, passwordHash string) (service.StoredUser, bool, error)
	FindByEmail(ctx context.Context, email string) (service.StoredUser, bool, error)
}

type userRepository struct {
	*base
}

func newUserRepository(base *base) *userRepository {
	return &userRepository{base: base}
}

func (r *userRepository) Create(ctx context.Context, email, passwordHash string) (service.StoredUser, bool, error) {
	user, err := r.queries.CreateUser(ctx, dbsqlc.CreateUserParams{
		Email: email, PasswordHash: passwordHash,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return service.StoredUser{}, false, nil
	}
	if err != nil {
		return service.StoredUser{}, false, fmt.Errorf("create user query: %w", err)
	}
	return storedUser(user), true, nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (service.StoredUser, bool, error) {
	user, err := r.queries.GetUserByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return service.StoredUser{}, false, nil
	}
	if err != nil {
		return service.StoredUser{}, false, fmt.Errorf("get user by email query: %w", err)
	}
	return storedUser(user), true, nil
}

func storedUser(user dbsqlc.User) service.StoredUser {
	return service.StoredUser{
		ID:           user.ID,
		Email:        user.Email,
		PasswordHash: user.PasswordHash,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
	}
}

var _ UserRepository = (*userRepository)(nil)
