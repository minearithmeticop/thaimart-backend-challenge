package domain

import "errors"

// Sentinel errors are the vocabulary inner layers use to describe
// business-rule failures. Transport adapters (HTTP, gRPC) translate them
// into protocol-specific status codes, keeping that mapping in one place.
var (
	// ErrInvalidInput reports a validation failure such as a missing
	// field or a malformed email.
	ErrInvalidInput = errors.New("invalid input")

	// ErrInvalidUserID reports a malformed user identifier.
	ErrInvalidUserID = errors.New("invalid user id")

	// ErrUserNotFound reports that no user matches the given id or email.
	ErrUserNotFound = errors.New("user not found")

	// ErrEmailAlreadyExists reports a uniqueness violation on email.
	ErrEmailAlreadyExists = errors.New("email already exists")

	// ErrInvalidCredentials reports a failed login. It is deliberately
	// vague so attackers cannot enumerate registered accounts.
	ErrInvalidCredentials = errors.New("invalid email or password")

	// ErrUnauthorized reports a missing, malformed or invalid token.
	ErrUnauthorized = errors.New("unauthorized")
)
