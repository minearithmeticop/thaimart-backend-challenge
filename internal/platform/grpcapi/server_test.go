package grpcapi

// gRPC tests run over bufconn (an in-memory listener): no real ports, no
// MongoDB. The stack is real except for the repository.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"golang.org/x/crypto/bcrypt"

	userv1 "github.com/minearithmeticop/thaimart-backend-challenge/api/proto/user/v1"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app/user"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/platform/security"
)

const testTokenSecret = "grpc-test-secret-long-enough-0123456789"

type fakeRepo struct {
	users  []domain.User
	nextID int
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

func (f *fakeRepo) Count(_ context.Context) (int64, error) {
	return int64(len(f.users)), nil
}

// newTestClient spins the gRPC server on a bufconn listener and returns
// a connected client plus a token minted by the same manager the server
// verifies with.
func newTestClient(t *testing.T) (userv1.UserServiceClient, string) {
	t.Helper()

	repo := &fakeRepo{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hasher := security.NewBcryptHasher(bcrypt.MinCost)
	tokens := security.NewJWTManager(testTokenSecret, time.Hour)
	users := user.NewService(repo, hasher, log)

	lis := bufconn.Listen(256 * 1024)
	grpcSrv := NewServer(users, tokens, log)
	t.Cleanup(func() { grpcSrv.Stop() })
	go func() { _ = grpcSrv.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc client: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	token, _, err := tokens.Generate("000000000000000000000001", "caller@example.com")
	if err != nil {
		t.Fatal(err)
	}
	return userv1.NewUserServiceClient(conn), token
}

func authCtx(token string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

func TestUnauthenticatedCallsAreRejected(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		{"no metadata", func() error {
			_, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: "000000000000000000000001"})
			return err
		}},
		{"malformed header", func() error {
			_, err := client.GetUser(
				metadata.AppendToOutgoingContext(ctx, "authorization", "just-a-token"),
				&userv1.GetUserRequest{Id: "000000000000000000000001"})
			return err
		}},
		{"forged token", func() error {
			other := security.NewJWTManager("attacker-secret-also-long-enough-9", time.Hour)
			forged, _, err := other.Generate("000000000000000000000001", "x@example.com")
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.GetUser(authCtx(forged), &userv1.GetUserRequest{Id: "000000000000000000000001"})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("code = %v, want Unauthenticated (err: %v)", status.Code(err), err)
			}
		})
	}
}

func TestCreateAndGetUser(t *testing.T) {
	client, token := newTestClient(t)

	created, err := client.CreateUser(authCtx(token), &userv1.CreateUserRequest{
		Name: "Somchai", Email: "somchai@example.com", Password: "P@ssw0rd1",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.User.Id == "" || created.User.Name != "Somchai" || created.User.Email != "somchai@example.com" {
		t.Fatalf("created user mismatch: %+v", created.User)
	}
	if created.User.CreatedAt == nil || !created.User.CreatedAt.IsValid() {
		t.Fatal("expected a valid created_at timestamp")
	}

	got, err := client.GetUser(authCtx(token), &userv1.GetUserRequest{Id: created.User.Id})
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.User.Id != created.User.Id || got.User.Email != "somchai@example.com" {
		t.Fatalf("fetched user mismatch: %+v", got.User)
	}
}

func TestDomainErrorsMapToStatusCodes(t *testing.T) {
	client, token := newTestClient(t)

	_, err := client.CreateUser(authCtx(token), &userv1.CreateUserRequest{
		Name: "A", Email: "somchai@example.com", Password: "P@ssw0rd1",
	})
	if status.Code(err) != codes.OK {
		t.Fatalf("seed create: %v", err)
	}

	_, err = client.CreateUser(authCtx(token), &userv1.CreateUserRequest{
		Name: "B", Email: "somchai@example.com", Password: "P@ssw0rd1",
	})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("duplicate email: code = %v, want AlreadyExists", status.Code(err))
	}

	_, err = client.CreateUser(authCtx(token), &userv1.CreateUserRequest{
		Name: "C", Email: "not-an-email", Password: "P@ssw0rd1",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invalid input: code = %v, want InvalidArgument", status.Code(err))
	}

	_, err = client.GetUser(authCtx(token), &userv1.GetUserRequest{Id: "0000000000000000000000ff"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("unknown id: code = %v, want NotFound", status.Code(err))
	}

	_, err = client.GetUser(authCtx(token), &userv1.GetUserRequest{Id: "not-an-id"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("malformed id: code = %v, want InvalidArgument", status.Code(err))
	}
}
