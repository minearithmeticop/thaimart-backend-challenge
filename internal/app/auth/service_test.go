package auth

// Login behaviour is verified against in-memory fakes. The headline rule:
// an unknown email and a wrong password must produce the same error.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app/user"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

// ---- fakes ------------------------------------------------------------------

type fakeRepo struct {
	users         []domain.User
	nextID        int
	getByEmailErr error // injected failure
}

var _ app.UserRepository = (*fakeRepo)(nil)

func (f *fakeRepo) Create(_ context.Context, u domain.User) (domain.User, error) {
	for _, existing := range f.users {
		if existing.Email == u.Email {
			return domain.User{}, fmt.Errorf("%w: %s", domain.ErrEmailAlreadyExists, u.Email)
		}
	}
	f.nextID++
	u.ID = fmt.Sprintf("%024x", f.nextID)
	u.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.users = append(f.users, u)
	return u, nil
}

func (f *fakeRepo) GetByID(_ context.Context, id string) (domain.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

func (f *fakeRepo) GetByEmail(_ context.Context, email string) (domain.User, error) {
	if f.getByEmailErr != nil {
		return domain.User{}, f.getByEmailErr
	}
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

func (f *fakeRepo) List(_ context.Context, limit, offset int64) ([]domain.User, int64, error) {
	if offset >= int64(len(f.users)) {
		return []domain.User{}, int64(len(f.users)), nil
	}
	end := offset + limit
	if end > int64(len(f.users)) {
		end = int64(len(f.users))
	}
	return f.users[offset:end], int64(len(f.users)), nil
}

func (f *fakeRepo) Update(_ context.Context, u domain.User) (domain.User, error) {
	for i, existing := range f.users {
		if existing.ID == u.ID {
			f.users[i] = u
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

func (f *fakeRepo) Delete(_ context.Context, id string) error {
	for i, existing := range f.users {
		if existing.ID == id {
			f.users = append(f.users[:i], f.users[i+1:]...)
			return nil
		}
	}
	return domain.ErrUserNotFound
}

func (f *fakeRepo) Count(_ context.Context) (int64, error) {
	return int64(len(f.users)), nil
}

type fakeHasher struct{}

func (fakeHasher) Hash(p string) (string, error) { return "hashed:" + p, nil }

func (fakeHasher) Compare(hash, p string) error {
	if hash != "hashed:"+p {
		return domain.ErrInvalidCredentials
	}
	return nil
}

type fakeTokens struct{ err error }

func (t fakeTokens) Generate(userID, email string) (string, time.Time, error) {
	if t.err != nil {
		return "", time.Time{}, t.err
	}
	return "tok-" + userID, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), nil
}

func (fakeTokens) Verify(token string) (app.Claims, error) {
	if len(token) < 5 || token[:4] != "tok" {
		return app.Claims{}, domain.ErrUnauthorized
	}
	return app.Claims{UserID: token[4:], Email: "", ExpiresAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}, nil
}

// ---- helpers -----------------------------------------------------------------

func newTestService(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	repo := &fakeRepo{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	users := user.NewService(repo, fakeHasher{}, log)
	return NewService(users, repo, fakeHasher{}, fakeTokens{}, log), repo
}

// ---- tests ---------------------------------------------------------------------

func TestRegister(t *testing.T) {
	svc, repo := newTestService(t)

	got, err := svc.Register(context.Background(), "Somchai", "Somchai@Example.com", "P@ssw0rd1")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got.Email != "somchai@example.com" {
		t.Errorf("email not normalized through the user use case: %q", got.Email)
	}
	if n, _ := repo.Count(context.Background()); n != 1 {
		t.Errorf("stored %d users, want 1", n)
	}
}

func TestLoginSuccess(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	registered, _ := svc.Register(ctx, "Somchai", "somchai@example.com", "P@ssw0rd1")

	session, err := svc.Login(ctx, "somchai@example.com", "P@ssw0rd1")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if session.Token != "tok-"+registered.ID {
		t.Errorf("token = %q", session.Token)
	}
	if session.User.ID != registered.ID {
		t.Errorf("session user = %+v", session.User)
	}
	if session.ExpiresAt.IsZero() {
		t.Error("expected ExpiresAt to be set")
	}
}

func TestLoginNormalizesEmail(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	svc.Register(ctx, "Somchai", "MiXeD@Example.COM", "P@ssw0rd1")

	if _, err := svc.Login(ctx, "  mixed@example.com ", "P@ssw0rd1"); err != nil {
		t.Fatalf("login with differently cased email failed: %v", err)
	}
}

func TestLoginFailuresLookIdentical(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	svc.Register(ctx, "Somchai", "real@example.com", "P@ssw0rd1")

	unknown, err1 := svc.Login(ctx, "ghost@example.com", "P@ssw0rd1")
	wrong, err2 := svc.Login(ctx, "real@example.com", "WrongP@ss1")

	if !errors.Is(err1, domain.ErrInvalidCredentials) || !errors.Is(err2, domain.ErrInvalidCredentials) {
		t.Fatalf("both must fail with ErrInvalidCredentials, got %v and %v", err1, err2)
	}
	if unknown.Token != "" || wrong.Token != "" {
		t.Error("failed logins must not return a token")
	}
	if err1.Error() != err2.Error() {
		t.Errorf("error messages differ, leaking which part failed: %q vs %q", err1, err2)
	}
}

// Only a genuine ErrUserNotFound translates into ErrInvalidCredentials;
// infrastructure failures must surface unchanged so operators can tell a
// broken database from a wrong password.
func TestLoginPropagatesInfrastructureErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("email lookup fails", func(t *testing.T) {
		svc, repo := newTestService(t)
		svc.Register(ctx, "A", "a@example.com", "P@ssw0rd1")
		repo.getByEmailErr = errors.New("db down")

		_, err := svc.Login(ctx, "a@example.com", "P@ssw0rd1")
		if !errors.Is(err, repo.getByEmailErr) {
			t.Fatalf("got %v, want the injected error", err)
		}
		if errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatal("infrastructure failure must not be masked as ErrInvalidCredentials")
		}
	})

	t.Run("token signing fails", func(t *testing.T) {
		signErr := errors.New("signing failed")
		repo := &fakeRepo{}
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		users := user.NewService(repo, fakeHasher{}, log)
		svc := NewService(users, repo, fakeHasher{}, fakeTokens{err: signErr}, log)
		svc.Register(ctx, "A", "a@example.com", "P@ssw0rd1")

		_, err := svc.Login(ctx, "a@example.com", "P@ssw0rd1")
		if !errors.Is(err, signErr) {
			t.Fatalf("got %v, want the injected error", err)
		}
	})
}
