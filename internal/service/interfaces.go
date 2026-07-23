package service

import (
	"context"
	"time"
)

type HealthRepository interface {
	Ping(ctx context.Context) error
}

type HealthService interface {
	Check(ctx context.Context) (Health, error)
}

type UserRepository interface {
	Create(ctx context.Context, email, passwordHash string) (StoredUser, bool, error)
	FindByEmail(ctx context.Context, email string) (StoredUser, bool, error)
}

type AuthService interface {
	Signup(ctx context.Context, email, password string) (AuthResult, error)
	Login(ctx context.Context, email, password string) (AuthResult, error)
}

type Health struct {
	Status    string    `json:"status"`
	Database  string    `json:"database"`
	CheckedAt time.Time `json:"checked_at"`
}

type StoredUser struct {
	ID           string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AuthResult struct {
	User        User      `json:"user"`
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}
