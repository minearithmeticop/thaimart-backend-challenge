package report

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

// fakeRepo implements app.UserRepository in memory and records Count
// calls; failFirst makes the first N counts fail.
type fakeRepo struct {
	mu        sync.Mutex
	users     []domain.User
	nextID    int
	calls     int
	failFirst int
}

var _ app.UserRepository = (*fakeRepo)(nil)

func (f *fakeRepo) Count(_ context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= f.failFirst {
		return 0, errors.New("db down")
	}
	return int64(len(f.users)), nil
}

func (f *fakeRepo) countCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

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

// capturingHandler records log records for assertions.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *capturingHandler) WithGroup(string) slog.Handler { return h }

func (h *capturingHandler) Handle(_ context.Context, rec slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, rec)
	return nil
}

func (h *capturingHandler) messagesAt(level slog.Level) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, rec := range h.records {
		if rec.Level == level {
			out = append(out, rec.Message)
		}
	}
	return out
}

func newReporter(repo *fakeRepo, interval time.Duration) (*UserCountReporter, *capturingHandler) {
	h := &capturingHandler{}
	return NewUserCountReporter(repo, interval, slog.New(h)), h
}

func TestRunReportsImmediatelyThenPerInterval(t *testing.T) {
	repo := &fakeRepo{}
	repo.Create(context.Background(), domain.User{Name: "A", Email: "a@example.com"})
	reporter, logs := newReporter(repo, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		reporter.Run(ctx)
		close(done)
	}()

	// The immediate report plus at least one tick.
	deadline := time.After(2 * time.Second)
	for repo.countCalls() < 2 {
		select {
		case <-deadline:
			t.Fatalf("only %d count calls after 2s, want >= 2", repo.countCalls())
		case <-time.After(2 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}

	infos := logs.messagesAt(slog.LevelInfo)
	if len(infos) < 2 || infos[0] != "user count report" {
		t.Fatalf("expected an immediate report then ticks, got %v", infos)
	}
	foundStop := false
	for _, m := range infos {
		if m == "user count reporter stopped" {
			foundStop = true
		}
	}
	if !foundStop {
		t.Fatal("expected a stop log after cancellation")
	}
}

func TestReportFailureIsLoggedNotFatal(t *testing.T) {
	repo := &fakeRepo{failFirst: 1}
	reporter, logs := newReporter(repo, time.Hour)

	reporter.reportOnce(context.Background()) // fails
	reporter.reportOnce(context.Background()) // succeeds

	warns := logs.messagesAt(slog.LevelWarn)
	if len(warns) != 1 || warns[0] != "user count report failed" {
		t.Fatalf("expected exactly one failure warning, got %v", warns)
	}
	infos := logs.messagesAt(slog.LevelInfo)
	if len(infos) != 1 || infos[0] != "user count report" {
		t.Fatalf("expected the second report to succeed, got %v", infos)
	}
}
