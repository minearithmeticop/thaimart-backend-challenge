// Package app holds the use cases and the ports (interfaces) the
// application core depends on. Adapters in internal/platform implement
// these ports, which is what lets unit tests swap MongoDB for an
// in-memory fake without touching business logic.
package app

import (
	"context"
	"time"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

// UserRepository is the persistence port for users.
type UserRepository interface {
	// Create inserts a new user and returns it with ID and CreatedAt set.
	// It returns domain.ErrEmailAlreadyExists when the email is taken.
	Create(ctx context.Context, u domain.User) (domain.User, error)

	// GetByID returns the user with the given id, domain.ErrUserNotFound
	// when nothing matches and domain.ErrInvalidUserID when the id is
	// malformed.
	GetByID(ctx context.Context, id string) (domain.User, error)

	// GetByEmail returns the user with the given exact email address.
	GetByEmail(ctx context.Context, email string) (domain.User, error)

	// List returns one page of users ordered by creation time (oldest
	// first) together with the total number of users.
	List(ctx context.Context, limit, offset int64) (users []domain.User, total int64, err error)

	// Update persists changes to an existing user (identified by u.ID).
	Update(ctx context.Context, u domain.User) (domain.User, error)

	// Delete removes the user with the given id.
	Delete(ctx context.Context, id string) error

	// Count returns the total number of users.
	Count(ctx context.Context) (int64, error)
}

// PasswordHasher hashes passwords and verifies plaintext candidates
// against a stored hash.
type PasswordHasher interface {
	Hash(plaintext string) (string, error)
	Compare(hash, plaintext string) error
}

// Claims is the verified identity extracted from an auth token.
type Claims struct {
	UserID    string
	Email     string
	ExpiresAt time.Time
}

// TokenManager mints and verifies auth tokens.
type TokenManager interface {
	Generate(userID, email string) (token string, expiresAt time.Time, err error)
	Verify(token string) (Claims, error)
}

// HealthChecker reports whether a backing store is reachable. Used by
// the health endpoint.
type HealthChecker interface {
	Ping(ctx context.Context) error
}
