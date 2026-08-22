package security

import (
	"golang.org/x/crypto/bcrypt"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

var _ app.PasswordHasher = (*BcryptHasher)(nil)

// BcryptHasher implements app.PasswordHasher on top of
// golang.org/x/crypto/bcrypt.
type BcryptHasher struct{ cost int }

// NewBcryptHasher returns a hasher with the given bcrypt cost. Cost 10 is
// a sensible production default; tests use bcrypt.MinCost to stay fast.
func NewBcryptHasher(cost int) *BcryptHasher {
	return &BcryptHasher{cost: cost}
}

func (h *BcryptHasher) Hash(plaintext string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plaintext), h.cost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Compare reports whether plaintext matches the stored hash. Every
// failure — wrong password or malformed hash — becomes the same domain
// error so callers cannot leak which part failed.
func (h *BcryptHasher) Compare(hash, plaintext string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)); err != nil {
		return domain.ErrInvalidCredentials
	}
	return nil
}
