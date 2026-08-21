package mongostore

// These tests run against the local MongoDB from deploy/compose.yaml and
// skip themselves when it is not reachable, so `go test ./...` still
// passes on a machine without Docker running.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

// newTestRepo connects to MongoDB, skips the test when it is unreachable,
// and gives every test its own throwaway database.
func newTestRepo(t *testing.T) *UserRepository {
	t.Helper()

	uri := envOr("MONGO_URI", "mongodb://localhost:27017")
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connecting to mongo: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		client.Disconnect(context.Background())
		t.Skipf("local mongodb not reachable at %s: %v", uri, err)
	}

	db := client.Database(fmt.Sprintf("thaimart_test_%d", time.Now().UnixNano()))
	ctx, cancelIdx := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelIdx()
	repo, err := NewUserRepository(ctx, db)
	if err != nil {
		t.Fatalf("creating repository: %v", err)
	}

	t.Cleanup(func() {
		db.Drop(context.Background())
		client.Disconnect(context.Background())
	})
	return repo
}

func mustCreate(t *testing.T, repo *UserRepository, name, email string) domain.User {
	t.Helper()
	u, err := repo.Create(context.Background(), domain.User{Name: name, Email: email, Password: "hashed"})
	if err != nil {
		t.Fatalf("creating %s: %v", email, err)
	}
	return u
}

func TestCreateAndGetByID(t *testing.T) {
	repo := newTestRepo(t)
	created := mustCreate(t, repo, "Somchai", "somchai@example.com")

	if created.ID == "" {
		t.Fatal("expected generated ID")
	}
	if created.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set")
	}

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Somchai" || got.Email != "somchai@example.com" || got.Password != "hashed" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestGetByIDErrors(t *testing.T) {
	repo := newTestRepo(t)

	if _, err := repo.GetByID(context.Background(), "not-an-objectid"); !errors.Is(err, domain.ErrInvalidUserID) {
		t.Fatalf("malformed id: got %v, want ErrInvalidUserID", err)
	}
	if _, err := repo.GetByID(context.Background(), bson.NewObjectID().Hex()); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("missing id: got %v, want ErrUserNotFound", err)
	}
}

func TestGetByEmail(t *testing.T) {
	repo := newTestRepo(t)
	created := mustCreate(t, repo, "Somsri", "somsri@example.com")

	got, err := repo.GetByEmail(context.Background(), "somsri@example.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("email lookup returned id %q, want %q", got.ID, created.ID)
	}
	if _, err := repo.GetByEmail(context.Background(), "ghost@example.com"); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("missing email: got %v, want ErrUserNotFound", err)
	}
}

func TestCreateDuplicateEmail(t *testing.T) {
	repo := newTestRepo(t)
	mustCreate(t, repo, "First", "dup@example.com")

	_, err := repo.Create(context.Background(), domain.User{Name: "Second", Email: "dup@example.com", Password: "hashed"})
	if !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("duplicate email: got %v, want ErrEmailAlreadyExists", err)
	}
}

func TestUpdate(t *testing.T) {
	repo := newTestRepo(t)
	created := mustCreate(t, repo, "Old Name", "old@example.com")

	created.Name = "New Name"
	created.Email = "new@example.com"
	updated, err := repo.Update(context.Background(), created)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "New Name" || updated.Email != "new@example.com" {
		t.Fatalf("returned user mismatch: %+v", updated)
	}

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Name != "New Name" || got.Email != "new@example.com" {
		t.Fatalf("persisted user mismatch: %+v", got)
	}
	if got.Password != "hashed" || !got.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("password or created_at must survive an update: %+v", got)
	}
}

func TestUpdateEmailTaken(t *testing.T) {
	repo := newTestRepo(t)
	a := mustCreate(t, repo, "A", "a@example.com")
	mustCreate(t, repo, "B", "b@example.com")

	a.Email = "b@example.com"
	_, err := repo.Update(context.Background(), a)
	if !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("taken email: got %v, want ErrEmailAlreadyExists", err)
	}
}

func TestUpdateNotFound(t *testing.T) {
	repo := newTestRepo(t)

	ghost := domain.User{ID: bson.NewObjectID().Hex(), Name: "Ghost", Email: "ghost@example.com"}
	_, err := repo.Update(context.Background(), ghost)
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("missing user: got %v, want ErrUserNotFound", err)
	}
}

func TestDeleteLifecycle(t *testing.T) {
	repo := newTestRepo(t)
	created := mustCreate(t, repo, "Doomed", "doomed@example.com")

	if err := repo.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(context.Background(), created.ID); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("user should be gone, got %v", err)
	}
	if err := repo.Delete(context.Background(), created.ID); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("second delete: got %v, want ErrUserNotFound", err)
	}
}

func TestListAndCount(t *testing.T) {
	repo := newTestRepo(t)
	emails := []string{"u1@example.com", "u2@example.com", "u3@example.com", "u4@example.com", "u5@example.com"}
	for i, e := range emails {
		mustCreate(t, repo, fmt.Sprintf("User %d", i+1), e)
	}

	if total, err := repo.Count(context.Background()); err != nil || total != int64(len(emails)) {
		t.Fatalf("Count = (%d, %v), want (%d, nil)", total, err, len(emails))
	}

	users, total, err := repo.List(context.Background(), 2, 0)
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if total != 5 || len(users) != 2 {
		t.Fatalf("page 1 = (%d users, total %d), want (2 users, total 5)", len(users), total)
	}
	if users[0].Email != "u1@example.com" || users[1].Email != "u2@example.com" {
		t.Fatalf("oldest-first order broken: %+v", users)
	}

	users, _, err = repo.List(context.Background(), 2, 4)
	if err != nil {
		t.Fatalf("List last page: %v", err)
	}
	if len(users) != 1 || users[0].Email != "u5@example.com" {
		t.Fatalf("last page mismatch: %+v", users)
	}

	users, total, err = repo.List(context.Background(), 20, 50)
	if err != nil {
		t.Fatalf("List beyond end: %v", err)
	}
	if len(users) != 0 || total != 5 {
		t.Fatalf("beyond end = (%d users, total %d), want (0 users, total 5)", len(users), total)
	}
}

// TestConcurrentCreateSameEmail proves the unique index is the source of
// truth for email uniqueness: parallel creates of the same email end with
// exactly one document stored.
func TestConcurrentCreateSameEmail(t *testing.T) {
	repo := newTestRepo(t)
	const n = 10

	start := make(chan struct{})
	results := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := repo.Create(context.Background(), domain.User{
				Name:     fmt.Sprintf("Racer %d", i),
				Email:    "race@example.com",
				Password: "hashed",
			})
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrEmailAlreadyExists):
			// expected loser
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 success out of %d, got %d", n, successes)
	}

	total, err := repo.Count(context.Background())
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected exactly 1 stored document, got %d", total)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
