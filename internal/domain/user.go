// Package domain holds the core entities and business rules. It imports
// no frameworks or drivers: nothing here knows that MongoDB, gin or JWT
// exist.
package domain

import "time"

// User is the core user entity. Password always holds a bcrypt hash,
// never plaintext.
type User struct {
	ID        string
	Name      string
	Email     string
	Password  string
	CreatedAt time.Time
}

// ApplyUpdate returns a copy of the user with name and/or email replaced.
// Nil pointers keep the old value, which gives the API its PATCH-like
// partial update semantics without mutating the receiver.
func (u User) ApplyUpdate(name, email *string) User {
	if name != nil {
		u.Name = *name
	}
	if email != nil {
		u.Email = *email
	}
	return u
}
