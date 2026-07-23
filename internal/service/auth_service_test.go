package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type authUserRepository struct {
	createdUser StoredUser
	createOK    bool
	createErr   error
	foundUser   StoredUser
	findOK      bool
	findErr     error
	gotEmail    string
	gotHash     string
}

func (r *authUserRepository) Create(_ context.Context, email, passwordHash string) (StoredUser, bool, error) {
	r.gotEmail = email
	r.gotHash = passwordHash
	return r.createdUser, r.createOK, r.createErr
}

func (r *authUserRepository) FindByEmail(_ context.Context, email string) (StoredUser, bool, error) {
	r.gotEmail = email
	return r.foundUser, r.findOK, r.findErr
}

func TestAuthServiceSignupHashesPasswordAndIssuesJWT(t *testing.T) {
	const (
		secret   = "01234567890123456789012345678901"
		password = "correct horse battery staple"
	)
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	repository := &authUserRepository{
		createdUser: StoredUser{
			ID: "user-123", Email: "user@example.com", CreatedAt: now, UpdatedAt: now,
		},
		createOK: true,
	}
	auth := newAuthService(repository, testJWTConfig())
	auth.now = func() time.Time { return now }

	result, err := auth.Signup(context.Background(), " User@Example.COM ", password)
	if err != nil {
		t.Fatalf("Signup() error = %v", err)
	}
	if repository.gotEmail != "user@example.com" {
		t.Fatalf("stored email = %q, want normalized email", repository.gotEmail)
	}
	if repository.gotHash == password || bcrypt.CompareHashAndPassword([]byte(repository.gotHash), []byte(password)) != nil {
		t.Fatal("stored password is not a valid bcrypt hash")
	}
	if result.User.ID != "user-123" || result.TokenType != "Bearer" || result.AccessToken == "" {
		t.Fatalf("Signup() = %+v, want user and bearer token", result)
	}

	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(result.AccessToken, claims, func(*jwt.Token) (any, error) {
		return []byte(secret), nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(func() time.Time { return now }),
	)
	if err != nil || !token.Valid {
		t.Fatalf("issued token is invalid: %v", err)
	}
	if claims.Subject != "user-123" || claims.Issuer != "test-issuer" {
		t.Fatalf("claims = %+v, want expected subject and issuer", claims)
	}
}

func TestAuthServiceLoginRejectsInvalidPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("right-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	repository := &authUserRepository{
		foundUser: StoredUser{ID: "user-123", Email: "user@example.com", PasswordHash: string(hash)},
		findOK:    true,
	}
	auth := newAuthService(repository, testJWTConfig())

	_, err = auth.Login(context.Background(), "user@example.com", "wrong-password")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Login() error = %v, want ErrUnauthorized", err)
	}
}

func TestAuthServiceValidatesCredentials(t *testing.T) {
	auth := newAuthService(&authUserRepository{}, testJWTConfig())
	testCases := []struct {
		email    string
		password string
		field    string
	}{
		{email: "not-an-email", password: "password123", field: "email"},
		{email: "user@example.com", password: "short", field: "password"},
		{email: "user@example.com", password: strings.Repeat("x", 73), field: "password"},
	}

	for _, testCase := range testCases {
		_, err := auth.Signup(context.Background(), testCase.email, testCase.password)
		var validationErr *ValidationError
		if !errors.As(err, &validationErr) || validationErr.Field != testCase.field {
			t.Fatalf("Signup(%q) error = %v, want %s validation error", testCase.email, err, testCase.field)
		}
	}
}
