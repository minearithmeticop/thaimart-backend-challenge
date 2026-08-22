// Package user implements the user management use cases: create, fetch,
// list, partial update and delete, plus the input rules that guard them.
package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

// Pagination defaults and limits for List.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Update carries optional fields for a partial update; nil leaves the
// field untouched.
type Update struct {
	Name  *string
	Email *string
}

// Service is the user use case. It depends only on ports, so tests can
// run it against in-memory fakes.
type Service struct {
	repo   app.UserRepository
	hasher app.PasswordHasher
	log    *slog.Logger
}

func NewService(repo app.UserRepository, hasher app.PasswordHasher, log *slog.Logger) *Service {
	return &Service{repo: repo, hasher: hasher, log: log}
}

// Create validates input, hashes the password and stores a new user.
func (s *Service) Create(ctx context.Context, name, email, password string) (domain.User, error) {
	name = strings.TrimSpace(name)
	email = NormalizeEmail(email)

	if err := validateNewUser(name, email, password); err != nil {
		return domain.User{}, err
	}

	// Fail fast on a taken email before spending CPU on bcrypt.
	if err := s.ensureEmailAvailable(ctx, email, ""); err != nil {
		return domain.User{}, err
	}

	hashed, err := s.hasher.Hash(password)
	if err != nil {
		return domain.User{}, err
	}

	created, err := s.repo.Create(ctx, domain.User{Name: name, Email: email, Password: hashed})
	if err != nil {
		return domain.User{}, err
	}
	s.log.Info("user created", "user_id", created.ID)
	return created, nil
}

// Get returns the user with the given id.
func (s *Service) Get(ctx context.Context, id string) (domain.User, error) {
	if !isValidUserID(id) {
		return domain.User{}, domain.ErrInvalidUserID
	}
	return s.repo.GetByID(ctx, id)
}

// List returns one page of users plus the total count.
func (s *Service) List(ctx context.Context, limit, offset int64) ([]domain.User, int64, error) {
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.List(ctx, limit, offset)
}

// Update applies a partial update to a user's name and/or email.
func (s *Service) Update(ctx context.Context, id string, in Update) (domain.User, error) {
	if !isValidUserID(id) {
		return domain.User{}, domain.ErrInvalidUserID
	}
	if in.Name == nil && in.Email == nil {
		return domain.User{}, fmt.Errorf("%w: at least one of name or email must be provided", domain.ErrInvalidInput)
	}

	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.User{}, err
	}

	next, err := buildUpdate(current, in.Name, in.Email)
	if err != nil {
		return domain.User{}, err
	}
	if err := s.ensureEmailAvailable(ctx, next.Email, current.Email); err != nil {
		return domain.User{}, err
	}

	updated, err := s.repo.Update(ctx, next)
	if err != nil {
		return domain.User{}, err
	}
	s.log.Info("user updated", "user_id", id)
	return updated, nil
}

// Delete removes the user with the given id.
func (s *Service) Delete(ctx context.Context, id string) error {
	if !isValidUserID(id) {
		return domain.ErrInvalidUserID
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.log.Info("user deleted", "user_id", id)
	return nil
}

// buildUpdate validates the requested fields and returns the next version
// of the user. Nil pointers leave fields untouched.
func buildUpdate(current domain.User, name, email *string) (domain.User, error) {
	next := current.ApplyUpdate(trimPtr(name), normalizePtr(email))

	if name != nil {
		if next.Name == "" {
			return domain.User{}, fmt.Errorf("%w: name cannot be empty", domain.ErrInvalidInput)
		}
		if len(next.Name) > maxNameLength {
			return domain.User{}, fmt.Errorf("%w: name must be at most %d characters", domain.ErrInvalidInput, maxNameLength)
		}
	}
	if email != nil {
		if err := validateEmail(next.Email); err != nil {
			return domain.User{}, err
		}
	}
	return next, nil
}

// ensureEmailAvailable reports whether want is usable as an email. It
// short-circuits when the email is unchanged; because emails are unique,
// a hit on a changed email can never be the current user, so no id
// comparison is needed. The unique index remains the final guard for the
// race window.
func (s *Service) ensureEmailAvailable(ctx context.Context, want, current string) error {
	if want == current {
		return nil
	}
	if _, err := s.repo.GetByEmail(ctx, want); err == nil {
		return fmt.Errorf("%w: %s", domain.ErrEmailAlreadyExists, want)
	} else if !errors.Is(err, domain.ErrUserNotFound) {
		return err
	}
	return nil
}

func trimPtr(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	return &trimmed
}

func normalizePtr(v *string) *string {
	if v == nil {
		return nil
	}
	normalized := NormalizeEmail(*v)
	return &normalized
}
