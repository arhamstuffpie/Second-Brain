package service

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

type authService struct {
	repository UserRepository
	secret     []byte
	issuer     string
	tokenTTL   time.Duration
	now        func() time.Time
}

func newAuthService(repository UserRepository, cfg config.JWTConfig) *authService {
	return &authService{
		repository: repository,
		secret:     []byte(cfg.Secret),
		issuer:     cfg.Issuer,
		tokenTTL:   cfg.AccessTokenTTL,
		now:        time.Now,
	}
}

func (s *authService) Signup(ctx context.Context, email, password string) (AuthResult, error) {
	normalizedEmail, err := validateCredentials(email, password)
	if err != nil {
		return AuthResult{}, err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return AuthResult{}, fmt.Errorf("hash password: %w", err)
	}

	user, created, err := s.repository.Create(ctx, normalizedEmail, string(passwordHash))
	if err != nil {
		return AuthResult{}, &UnavailableError{Cause: err}
	}
	if !created {
		return AuthResult{}, fmt.Errorf("%w: an account with this email already exists", ErrConflict)
	}

	return s.authResult(user)
}

func (s *authService) Login(ctx context.Context, email, password string) (AuthResult, error) {
	normalizedEmail, err := validateCredentials(email, password)
	if err != nil {
		return AuthResult{}, err
	}

	user, found, err := s.repository.FindByEmail(ctx, normalizedEmail)
	if err != nil {
		return AuthResult{}, &UnavailableError{Cause: err}
	}
	if !found || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return AuthResult{}, ErrUnauthorized
	}

	return s.authResult(user)
}

func (s *authService) authResult(stored StoredUser) (AuthResult, error) {
	issuedAt := s.now().UTC()
	expiresAt := issuedAt.Add(s.tokenTTL)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   stored.ID,
		Issuer:    s.issuer,
		IssuedAt:  jwt.NewNumericDate(issuedAt),
		NotBefore: jwt.NewNumericDate(issuedAt),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	})
	signedToken, err := token.SignedString(s.secret)
	if err != nil {
		return AuthResult{}, fmt.Errorf("sign access token: %w", err)
	}

	return AuthResult{
		User: User{
			ID:        stored.ID,
			Email:     stored.Email,
			CreatedAt: stored.CreatedAt,
			UpdatedAt: stored.UpdatedAt,
		},
		AccessToken: signedToken,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
	}, nil
}

func validateCredentials(email, password string) (string, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(normalizedEmail)
	if err != nil || parsed.Address != normalizedEmail || len(normalizedEmail) > 254 {
		return "", &ValidationError{Field: "email", Message: "must be a valid email address"}
	}
	if len(password) < 8 {
		return "", &ValidationError{Field: "password", Message: "must be at least 8 characters"}
	}
	if len(password) > 72 {
		return "", &ValidationError{Field: "password", Message: "must not exceed 72 bytes"}
	}
	return normalizedEmail, nil
}

var _ AuthService = (*authService)(nil)
