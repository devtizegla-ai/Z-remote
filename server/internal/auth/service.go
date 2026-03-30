package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"remoteaccess/server/internal/models"
	"remoteaccess/server/internal/storage"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailExists        = errors.New("email already registered")
)

type Service struct {
	db     *sql.DB
	tokens *TokenManager
}

type LoginResult struct {
	User         models.User `json:"user"`
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
}

func NewService(store *storage.Store, tokens *TokenManager) *Service {
	return &Service{db: store.DB, tokens: tokens}
}

func (s *Service) Register(ctx context.Context, name, email, password string) (models.User, error) {
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	if name == "" || email == "" || len(password) < 6 {
		return models.User{}, fmt.Errorf("name, email and password are required; password min length is 6")
	}

	existing, err := s.findByEmail(ctx, email)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return models.User{}, err
	}
	if existing.ID != "" {
		return models.User{}, ErrEmailExists
	}

	hash, err := HashPassword(password)
	if err != nil {
		return models.User{}, err
	}

	now := storage.NowUTC()
	user := models.User{
		ID:           storage.NewID("usr"),
		Name:         name,
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO users (id, name, email, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		user.ID, user.Name, user.Email, user.PasswordHash, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return models.User{}, ErrEmailExists
		}
		return models.User{}, err
	}

	return user, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	user, err := s.findByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, err
	}
	if !ComparePassword(user.PasswordHash, password) {
		return LoginResult{}, ErrInvalidCredentials
	}

	accessToken, err := s.tokens.GenerateAccessToken(user.ID, user.Email)
	if err != nil {
		return LoginResult{}, err
	}
	refreshToken, err := s.tokens.GenerateRefreshToken(user.ID, user.Email)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *Service) Me(ctx context.Context, userID string) (models.User, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, email, password_hash, created_at, updated_at FROM users WHERE id = ?`,
		userID,
	)
	var user models.User
	err := row.Scan(&user.ID, &user.Name, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (s *Service) findByEmail(ctx context.Context, email string) (models.User, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, email, password_hash, created_at, updated_at FROM users WHERE email = ?`,
		email,
	)
	var user models.User
	err := row.Scan(&user.ID, &user.Name, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}
