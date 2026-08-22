package user

// These tests run the use case against in-memory fakes — no MongoDB, no
// network. This is the payoff of the ports: business rules are verified
// fast and deterministically.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

// ---- fakes -----------------------------------------------------------------

type fakeRepo struct {
	users         []domain.User
	nextID        int
	lastLimit     int64
	lastOffset    int64
	getByEmailErr error // injected failures
	createErr     error
	updateErr     error
}

var _ app.UserRepository = (*fakeRepo)(nil)

func (f *fakeRepo) Create(_ context.Context, u domain.User) (domain.User, error) {
	if f.createErr != nil {
		return domain.User{}, f.createErr
	}
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
	f.lastLimit = limit
	f.lastOffset = offset
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
	if f.updateErr != nil {
		return domain.User{}, f.updateErr
	}
	for i, existing := range f.users {
		if existing.ID == u.ID {
			for j, other := range f.users {
				if j != i && other.Email == u.Email {
					return domain.User{}, fmt.Errorf("%w: %s", domain.ErrEmailAlreadyExists, u.Email)
				}
			}
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

type fakeHasher struct{ err error }

func (h fakeHasher) Hash(p string) (string, error) {
	if h.err != nil {
		return "", h.err
	}
	return "hashed:" + p, nil
}

func (fakeHasher) Compare(hash, p string) error {
	if hash != "hashed:"+p {
		return domain.ErrInvalidCredentials
	}
	return nil
}

// ---- helpers ----------------------------------------------------------------

func newTestService(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	repo := &fakeRepo{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(repo, fakeHasher{}, log), repo
}

func ptr(s string) *string { return &s }

// ---- tests ------------------------------------------------------------------

func TestCreate(t *testing.T) {
	svc, repo := newTestService(t)

	got, err := svc.Create(context.Background(), "  Somchai  ", "Somchai@Example.com ", "P@ssw0rd1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got.Name != "Somchai" {
		t.Errorf("name not trimmed: %q", got.Name)
	}
	if got.Email != "somchai@example.com" {
		t.Errorf("email not normalized: %q", got.Email)
	}
	if got.Password != "hashed:P@ssw0rd1" {
		t.Errorf("password not hashed, stored %q", got.Password)
	}
	if got.ID != repo.users[0].ID {
		t.Errorf("stored and returned IDs differ")
	}
}

func TestCreateValidation(t *testing.T) {
	cases := []struct {
		desc     string
		name     string
		email    string
		password string
	}{
		{"empty name", "", "a@example.com", "P@ssw0rd1"},
		{"name too long", strings.Repeat("x", 101), "a@example.com", "P@ssw0rd1"},
		{"empty email", "A", "", "P@ssw0rd1"},
		{"malformed email", "A", "not-an-email", "P@ssw0rd1"},
		{"email too long", "A", strings.Repeat("a", 250) + "@x.com", "P@ssw0rd1"},
		{"short password", "A", "a@example.com", "short"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			svc, repo := newTestService(t)
			_, err := svc.Create(context.Background(), tc.name, tc.email, tc.password)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("got %v, want ErrInvalidInput", err)
			}
			if n, _ := repo.Count(context.Background()); n != 0 {
				t.Fatalf("invalid input must not reach the repository, stored %d users", n)
			}
		})
	}
}

func TestCreateDuplicateEmail(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	if _, err := svc.Create(ctx, "A", "dup@example.com", "P@ssw0rd1"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.Create(ctx, "B", "dup@example.com", "P@ssw0rd1")
	if !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("got %v, want ErrEmailAlreadyExists", err)
	}
}

func TestGet(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	created, _ := svc.Create(ctx, "A", "a@example.com", "P@ssw0rd1")

	if _, err := svc.Get(ctx, "not-an-objectid"); !errors.Is(err, domain.ErrInvalidUserID) {
		t.Errorf("malformed id: got %v, want ErrInvalidUserID", err)
	}
	if _, err := svc.Get(ctx, "0000000000000000000000ff"); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("missing id: got %v, want ErrUserNotFound", err)
	}
	if got, err := svc.Get(ctx, created.ID); err != nil || got.ID != created.ID {
		t.Errorf("Get(created) = (%v, %v)", got, err)
	}
}

func TestListSanitizesPagination(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		svc.Create(ctx, fmt.Sprintf("U%d", i), fmt.Sprintf("u%d@example.com", i), "P@ssw0rd1")
	}

	if _, _, err := svc.List(ctx, 0, 0); err != nil {
		t.Fatal(err)
	}
	if repo.lastLimit != DefaultPageSize {
		t.Errorf("zero limit not defaulted: %d", repo.lastLimit)
	}

	if _, _, err := svc.List(ctx, 500, -7); err != nil {
		t.Fatal(err)
	}
	if repo.lastLimit != MaxPageSize {
		t.Errorf("limit not capped: %d", repo.lastLimit)
	}
	if repo.lastOffset != 0 {
		t.Errorf("negative offset not clamped: %d", repo.lastOffset)
	}

	users, total, err := svc.List(ctx, 2, 0)
	if err != nil || total != 3 || len(users) != 2 {
		t.Errorf("List(2,0) = (%d users, total %d, %v)", len(users), total, err)
	}
}

func TestUpdate(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	created, _ := svc.Create(ctx, "Old", "old@example.com", "P@ssw0rd1")

	t.Run("name only", func(t *testing.T) {
		got, err := svc.Update(ctx, created.ID, Update{Name: ptr("New")})
		if err != nil || got.Name != "New" || got.Email != "old@example.com" {
			t.Errorf("Update(name) = (%+v, %v)", got, err)
		}
	})

	t.Run("email only", func(t *testing.T) {
		got, err := svc.Update(ctx, created.ID, Update{Email: ptr("New@Example.COM")})
		if err != nil || got.Email != "new@example.com" {
			t.Errorf("Update(email) = (%+v, %v)", got, err)
		}
	})

	t.Run("empty payload", func(t *testing.T) {
		_, err := svc.Update(ctx, created.ID, Update{})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("got %v, want ErrInvalidInput", err)
		}
	})

	t.Run("blank name", func(t *testing.T) {
		_, err := svc.Update(ctx, created.ID, Update{Name: ptr("   ")})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("got %v, want ErrInvalidInput", err)
		}
	})

	t.Run("email taken", func(t *testing.T) {
		svc.Create(ctx, "Other", "taken@example.com", "P@ssw0rd1")
		_, err := svc.Update(ctx, created.ID, Update{Email: ptr("taken@example.com")})
		if !errors.Is(err, domain.ErrEmailAlreadyExists) {
			t.Errorf("got %v, want ErrEmailAlreadyExists", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := svc.Update(ctx, "0000000000000000000000ff", Update{Name: ptr("X")})
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("got %v, want ErrUserNotFound", err)
		}
	})
}

func TestDelete(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	created, _ := svc.Create(ctx, "Doomed", "doomed@example.com", "P@ssw0rd1")

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if n, _ := repo.Count(ctx); n != 0 {
		t.Fatalf("user still stored: %d", n)
	}
	if err := svc.Delete(ctx, created.ID); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("second delete: got %v, want ErrUserNotFound", err)
	}
	if err := svc.Delete(ctx, "not-an-objectid"); !errors.Is(err, domain.ErrInvalidUserID) {
		t.Fatalf("malformed id: got %v, want ErrInvalidUserID", err)
	}
}

// Infrastructure failures must surface unchanged: never swallowed and
// never dressed up as a business error.
func TestCreatePropagatesInfrastructureErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("email lookup fails", func(t *testing.T) {
		svc, repo := newTestService(t)
		repo.getByEmailErr = errors.New("db down")

		_, err := svc.Create(ctx, "A", "a@example.com", "P@ssw0rd1")
		if !errors.Is(err, repo.getByEmailErr) {
			t.Fatalf("got %v, want the injected error", err)
		}
		if errors.Is(err, domain.ErrEmailAlreadyExists) {
			t.Fatal("infrastructure failure must not surface as ErrEmailAlreadyExists")
		}
	})

	t.Run("hashing fails", func(t *testing.T) {
		hasher := &fakeHasher{err: errors.New("hash failed")}
		svc := NewService(&fakeRepo{}, hasher, slog.New(slog.NewTextHandler(io.Discard, nil)))

		_, err := svc.Create(ctx, "A", "a@example.com", "P@ssw0rd1")
		if !errors.Is(err, hasher.err) {
			t.Fatalf("got %v, want the injected error", err)
		}
	})

	t.Run("insert fails", func(t *testing.T) {
		svc, repo := newTestService(t)
		repo.createErr = errors.New("insert failed")

		_, err := svc.Create(ctx, "A", "a@example.com", "P@ssw0rd1")
		if !errors.Is(err, repo.createErr) {
			t.Fatalf("got %v, want the injected error", err)
		}
	})
}

func TestUpdatePropagatesInfrastructureErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("write fails", func(t *testing.T) {
		svc, repo := newTestService(t)
		created, _ := svc.Create(ctx, "A", "a@example.com", "P@ssw0rd1")
		repo.updateErr = errors.New("write failed")

		_, err := svc.Update(ctx, created.ID, Update{Name: ptr("New")})
		if !errors.Is(err, repo.updateErr) {
			t.Fatalf("got %v, want the injected error", err)
		}
	})

	t.Run("name too long", func(t *testing.T) {
		svc, _ := newTestService(t)
		created, _ := svc.Create(ctx, "A", "a@example.com", "P@ssw0rd1")

		_, err := svc.Update(ctx, created.ID, Update{Name: ptr(strings.Repeat("x", 101))})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("got %v, want ErrInvalidInput", err)
		}
	})
}

func TestUpdateRejectsBadInput(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	created, _ := svc.Create(ctx, "A", "a@example.com", "P@ssw0rd1")

	if _, err := svc.Update(ctx, "not-an-objectid", Update{Name: ptr("X")}); !errors.Is(err, domain.ErrInvalidUserID) {
		t.Fatalf("malformed id: got %v, want ErrInvalidUserID", err)
	}
	if _, err := svc.Update(ctx, created.ID, Update{Email: ptr("not-an-email")}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("malformed email: got %v, want ErrInvalidInput", err)
	}
}
