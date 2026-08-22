package security

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

func TestBcryptHasher(t *testing.T) {
	h := NewBcryptHasher(bcrypt.MinCost) // cheap on purpose: unit tests stay fast

	hash, err := h.Hash("P@ssw0rd1")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if hash == "" || hash == "P@ssw0rd1" {
		t.Fatalf("must never store plaintext, got %q", hash)
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Fatalf("not a bcrypt hash: %q", hash)
	}

	if err := h.Compare(hash, "P@ssw0rd1"); err != nil {
		t.Fatalf("correct password rejected: %v", err)
	}
	if err := h.Compare(hash, "wrong-password"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("wrong password: got %v, want ErrInvalidCredentials", err)
	}
	if err := h.Compare("not-a-hash", "P@ssw0rd1"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("malformed hash: got %v, want ErrInvalidCredentials", err)
	}

	other, err := h.Hash("P@ssw0rd1")
	if err != nil {
		t.Fatal(err)
	}
	if other == hash {
		t.Fatal("bcrypt must salt: two hashes of one password must differ")
	}
}
