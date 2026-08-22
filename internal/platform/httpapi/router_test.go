package httpapi

// Full-stack tests minus MongoDB: real gin router, real bcrypt hasher
// (MinCost) and real JWT manager — only the repository is a fake. Each
// test exercises the HTTP surface exactly the way a client would.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app/auth"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app/user"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/platform/security"
)

const testTokenSecret = "router-test-secret-long-enough-0123456789"

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

type fakeHealth struct{}

func (fakeHealth) Ping(context.Context) error { return nil }

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	repo := &fakeRepo{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hasher := security.NewBcryptHasher(bcrypt.MinCost)
	tokens := security.NewJWTManager(testTokenSecret, time.Hour)
	users := user.NewService(repo, hasher, log)
	authSvc := auth.NewService(users, repo, hasher, tokens, log)

	srv := httptest.NewServer(NewRouter(fakeHealth{}, tokens, authSvc, users, log))
	t.Cleanup(srv.Close)
	return srv
}

func doRequest(t *testing.T, method, url, token, body string) (*http.Response, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(raw)
}

// registerAndLogin creates an account and returns a valid bearer token.
func registerAndLogin(t *testing.T, base, email string) string {
	t.Helper()
	resp, body := doRequest(t, http.MethodPost, base+"/api/v1/auth/register", "",
		`{"name":"Tester","email":"`+email+`","password":"P@ssw0rd1"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: status %d body %s", resp.StatusCode, body)
	}
	resp, body = doRequest(t, http.MethodPost, base+"/api/v1/auth/login", "",
		`{"email":"`+email+`","password":"P@ssw0rd1"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d body %s", resp.StatusCode, body)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || out.Token == "" {
		t.Fatalf("login response has no token: %s", body)
	}
	return out.Token
}

// createUser creates a user through the protected endpoint.
func createUser(t *testing.T, base, token, name, email string) userResponse {
	t.Helper()
	resp, body := doRequest(t, http.MethodPost, base+"/api/v1/users", token,
		`{"name":"`+name+`","email":"`+email+`","password":"P@ssw0rd1"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user: status %d body %s", resp.StatusCode, body)
	}
	var u userResponse
	if err := json.Unmarshal([]byte(body), &u); err != nil {
		t.Fatal(err)
	}
	return u
}

// ---- health -------------------------------------------------------------------

func TestHealthz(t *testing.T) {
	srv := newTestServer(t)
	resp, body := doRequest(t, http.MethodGet, srv.URL+"/healthz", "", "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"ok"`) {
		t.Fatalf("healthz: status %d body %s", resp.StatusCode, body)
	}
}

// ---- auth: register and login ---------------------------------------------------

func TestRegisterReturnsUserWithoutSecrets(t *testing.T) {
	srv := newTestServer(t)

	resp, body := doRequest(t, http.MethodPost, srv.URL+"/api/v1/auth/register", "",
		`{"name":"Somchai","email":"somchai@example.com","password":"P@ssw0rd1"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if strings.Contains(body, "password") || strings.Contains(body, "hashed") {
		t.Fatalf("response leaks credentials: %s", body)
	}

	var u userResponse
	if err := json.Unmarshal([]byte(body), &u); err != nil {
		t.Fatal(err)
	}
	if u.ID == "" || u.Email != "somchai@example.com" || u.CreatedAt.IsZero() {
		t.Fatalf("incomplete user payload: %+v", u)
	}
	if loc := resp.Header.Get("Location"); loc != "/api/v1/users/"+u.ID {
		t.Fatalf("Location header = %q", loc)
	}
}

func TestRegisterDuplicateEmailIsConflict(t *testing.T) {
	srv := newTestServer(t)
	payload := `{"name":"Somchai","email":"dup@example.com","password":"P@ssw0rd1"}`

	if resp, body := doRequest(t, http.MethodPost, srv.URL+"/api/v1/auth/register", "", payload); resp.StatusCode != http.StatusCreated {
		t.Fatalf("first register: status %d body %s", resp.StatusCode, body)
	}
	resp, _ := doRequest(t, http.MethodPost, srv.URL+"/api/v1/auth/register", "",
		`{"name":"Other","email":"dup@example.com","password":"P@ssw0rd1"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate register: status %d, want 409", resp.StatusCode)
	}
}

func TestRegisterInvalidBodyIsBadRequest(t *testing.T) {
	srv := newTestServer(t)
	resp, _ := doRequest(t, http.MethodPost, srv.URL+"/api/v1/auth/register", "", `{not-json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestLoginReturnsBearerSession(t *testing.T) {
	srv := newTestServer(t)
	registerAndLogin(t, srv.URL, "session@example.com") // seeds the account

	resp, body := doRequest(t, http.MethodPost, srv.URL+"/api/v1/auth/login", "",
		`{"email":"session@example.com","password":"P@ssw0rd1"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}

	var out loginResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Token == "" || out.TokenType != "Bearer" || out.ExpiresAt.IsZero() || out.User.ID == "" {
		t.Fatalf("incomplete session payload: %+v", out)
	}
}

func TestLoginWrongPasswordIsUnauthorized(t *testing.T) {
	srv := newTestServer(t)
	registerAndLogin(t, srv.URL, "wrongpass@example.com")

	resp, _ := doRequest(t, http.MethodPost, srv.URL+"/api/v1/auth/login", "",
		`{"email":"wrongpass@example.com","password":"WrongP@ss1"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
}

// ---- auth middleware --------------------------------------------------------

func TestProtectedRoutesRejectMissingOrBadTokens(t *testing.T) {
	srv := newTestServer(t)
	base := srv.URL

	cases := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/users"},
		{http.MethodPost, "/api/v1/users"},
		{http.MethodGet, "/api/v1/users/000000000000000000000001"},
		{http.MethodPatch, "/api/v1/users/000000000000000000000001"},
		{http.MethodDelete, "/api/v1/users/000000000000000000000001"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp, _ := doRequest(t, tc.method, base+tc.path, "", "")
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("no token: status %d, want 401", resp.StatusCode)
			}
		})
	}

	resp, _ := doRequest(t, http.MethodGet, base+"/api/v1/users", "garbage.token.here", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("garbage token: status %d, want 401", resp.StatusCode)
	}

	other := security.NewJWTManager("attacker-secret-also-long-enough-123456", time.Hour)
	forged, _, err := other.Generate("000000000000000000000001", "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	resp, _ = doRequest(t, http.MethodGet, base+"/api/v1/users", forged, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("forged token: status %d, want 401", resp.StatusCode)
	}
}

// ---- users CRUD ----------------------------------------------------------------

func TestCreateUserEndpoint(t *testing.T) {
	srv := newTestServer(t)
	token := registerAndLogin(t, srv.URL, "creator@example.com")

	u := createUser(t, srv.URL, token, "Somsri", "somsri@example.com")
	if u.Name != "Somsri" || u.Email != "somsri@example.com" || u.ID == "" {
		t.Fatalf("created payload mismatch: %+v", u)
	}
}

func TestListUsersPagination(t *testing.T) {
	srv := newTestServer(t)
	base := srv.URL
	token := registerAndLogin(t, base, "lister@example.com")
	createUser(t, base, token, "Second", "second@example.com")

	resp, body := doRequest(t, http.MethodGet, base+"/api/v1/users?limit=1&offset=0", token, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	var out listUsersResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Total != 2 || len(out.Users) != 1 || out.Limit != 1 || out.Offset != 0 {
		t.Fatalf("list payload mismatch: %+v", out)
	}
}

func TestListUsersRejectsNonIntegerLimit(t *testing.T) {
	srv := newTestServer(t)
	token := registerAndLogin(t, srv.URL, "lister2@example.com")

	resp, _ := doRequest(t, http.MethodGet, srv.URL+"/api/v1/users?limit=abc", token, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestGetUser(t *testing.T) {
	srv := newTestServer(t)
	base := srv.URL
	token := registerAndLogin(t, base, "getter@example.com")
	target := createUser(t, base, token, "Target", "target@example.com")

	resp, body := doRequest(t, http.MethodGet, base+"/api/v1/users/"+target.ID, token, "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "target@example.com") {
		t.Fatalf("get by id: status %d body %s", resp.StatusCode, body)
	}

	resp, _ = doRequest(t, http.MethodGet, base+"/api/v1/users/not-an-id", token, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed id: status %d, want 400", resp.StatusCode)
	}

	resp, _ = doRequest(t, http.MethodGet, base+"/api/v1/users/0000000000000000000000ff", token, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id: status %d, want 404", resp.StatusCode)
	}
}

func TestUpdateUser(t *testing.T) {
	srv := newTestServer(t)
	base := srv.URL
	token := registerAndLogin(t, base, "updater@example.com")
	target := createUser(t, base, token, "Old Name", "old@example.com")

	resp, body := doRequest(t, http.MethodPatch, base+"/api/v1/users/"+target.ID, token,
		`{"name":"New Name"}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "New Name") {
		t.Fatalf("update name: status %d body %s", resp.StatusCode, body)
	}

	resp, _ = doRequest(t, http.MethodPatch, base+"/api/v1/users/"+target.ID, token,
		`{"email":"updater@example.com"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("taken email: status %d, want 409", resp.StatusCode)
	}
}

func TestDeleteUser(t *testing.T) {
	srv := newTestServer(t)
	base := srv.URL
	token := registerAndLogin(t, base, "deleter@example.com")
	target := createUser(t, base, token, "Doomed", "doomed@example.com")

	resp, _ := doRequest(t, http.MethodDelete, base+"/api/v1/users/"+target.ID, token, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status %d, want 204", resp.StatusCode)
	}
	resp, _ = doRequest(t, http.MethodGet, base+"/api/v1/users/"+target.ID, token, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete: status %d, want 404", resp.StatusCode)
	}
}
