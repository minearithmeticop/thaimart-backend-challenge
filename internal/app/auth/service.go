// Package auth implements registration and login.
package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app/user"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

// Session is the result of a successful login.
type Session struct {
	Token     string
	ExpiresAt time.Time
	User      domain.User
}

// Service is the authentication use case.
type Service struct {
	users  *user.Service
	repo   app.UserRepository
	hasher app.PasswordHasher
	tokens app.TokenManager
	log    *slog.Logger
}

func NewService(users *user.Service, repo app.UserRepository, hasher app.PasswordHasher, tokens app.TokenManager, log *slog.Logger) *Service {
	return &Service{users: users, repo: repo, hasher: hasher, tokens: tokens, log: log}
}

// Register creates a new account through the user use case.
func (s *Service) Register(ctx context.Context, name, email, password string) (domain.User, error) {
	return s.users.Create(ctx, name, email, password)
}

// Login verifies credentials and returns a signed session. An unknown
// email and a wrong password return the same error so that attackers
// cannot probe which emails are registered.
func (s *Service) Login(ctx context.Context, email, password string) (Session, error) {
	u, err := s.repo.GetByEmail(ctx, user.NormalizeEmail(email))
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return Session{}, domain.ErrInvalidCredentials
		}
		return Session{}, err
	}

	if err := s.hasher.Compare(u.Password, password); err != nil {
		return Session{}, domain.ErrInvalidCredentials
	}

	token, expiresAt, err := s.tokens.Generate(u.ID, u.Email)
	if err != nil {
		return Session{}, err
	}
	s.log.Info("user logged in", "user_id", u.ID)
	return Session{Token: token, ExpiresAt: expiresAt, User: u}, nil
}
