package user

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

// emailRe is a pragmatic RFC-5322 subset: good enough to reject typos
// while accepting every address a real user would type.
var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// objectIDHexRe matches the 24-character hex form of a MongoDB ObjectID,
// so callers get an immediate error instead of a database round trip.
var objectIDHexRe = regexp.MustCompile(`^[0-9a-fA-F]{24}$`)

const (
	minPasswordLength = 8
	maxNameLength     = 100
	maxEmailLength    = 254
)

// NormalizeEmail trims and lowercases an email so one canonical form is
// stored and looked up everywhere.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validateNewUser(name, email, password string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: name is required", domain.ErrInvalidInput)
	}
	if len(name) > maxNameLength {
		return fmt.Errorf("%w: name must be at most %d characters", domain.ErrInvalidInput, maxNameLength)
	}
	if err := validateEmail(email); err != nil {
		return err
	}
	if len(password) < minPasswordLength {
		return fmt.Errorf("%w: password must be at least %d characters", domain.ErrInvalidInput, minPasswordLength)
	}
	return nil
}

func validateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("%w: email is required", domain.ErrInvalidInput)
	}
	if len(email) > maxEmailLength {
		return fmt.Errorf("%w: email must be at most %d characters", domain.ErrInvalidInput, maxEmailLength)
	}
	if !emailRe.MatchString(email) {
		return fmt.Errorf("%w: email format is invalid", domain.ErrInvalidInput)
	}
	return nil
}

func isValidUserID(id string) bool {
	return objectIDHexRe.MatchString(id)
}
