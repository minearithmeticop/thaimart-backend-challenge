package security

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

const testSecret = "test-secret-that-is-long-enough-0123456789"

func TestJWTRoundTrip(t *testing.T) {
	m := NewJWTManager(testSecret, time.Hour)

	token, expiresAt, err := m.Generate("user-1", "a@example.com")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if token == "" {
		t.Fatal("expected a token")
	}
	if until := time.Until(expiresAt); until <= 0 || until > time.Hour {
		t.Fatalf("expiry not within (0, 1h]: %s", until)
	}

	claims, err := m.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != "user-1" || claims.Email != "a@example.com" {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}

func TestJWTExpired(t *testing.T) {
	m := NewJWTManager(testSecret, -time.Minute)

	token, _, err := m.Generate("user-1", "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Verify(token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired token: got %v, want ErrUnauthorized", err)
	}
}

func TestJWTWrongSecret(t *testing.T) {
	signer := NewJWTManager(testSecret, time.Hour)
	verifier := NewJWTManager("another-secret-also-long-enough-abcdef", time.Hour)

	token, _, err := signer.Generate("user-1", "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("token signed by another key: got %v, want ErrUnauthorized", err)
	}
}

// An attacker who swaps the header alg to "none" (no signature at all)
// must be rejected — the alg-confusion attack.
func TestJWTRejectsAlgNone(t *testing.T) {
	forged := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	signed, err := forged.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}

	m := NewJWTManager(testSecret, time.Hour)
	if _, err := m.Verify(signed); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("unsigned token accepted: got %v, want ErrUnauthorized", err)
	}
}

func TestJWTGarbage(t *testing.T) {
	m := NewJWTManager(testSecret, time.Hour)
	if _, err := m.Verify("not-a-token"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("garbage input: got %v, want ErrUnauthorized", err)
	}
}

func TestJWTRequiresSub(t *testing.T) {
	// Signed with the correct key but missing the sub claim.
	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"email": "a@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	signed, err := forged.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}

	m := NewJWTManager(testSecret, time.Hour)
	if _, err := m.Verify(signed); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("token without sub accepted: got %v, want ErrUnauthorized", err)
	}
}
