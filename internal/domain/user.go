package domain

import (
	"fmt"
	"net/mail"
	"strings"
	"time"
)

const (
	minPasswordLength = 8
	// bcrypt only uses the first 72 bytes of a password.
	maxPasswordLength = 72
	maxEmailLength    = 254
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// TODO check common pattern to enforce security
func ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("%w: email is required", ErrInvalid)
	}
	if len(email) > maxEmailLength {
		return fmt.Errorf("%w: email is too long", ErrInvalid)
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return fmt.Errorf("%w: email is invalid", ErrInvalid)
	}
	at := strings.LastIndex(email, "@")
	domainPart := email[at+1:]
	if !strings.Contains(domainPart, ".") || strings.HasPrefix(domainPart, ".") || strings.HasSuffix(domainPart, ".") {
		return fmt.Errorf("%w: email is invalid", ErrInvalid)
	}
	return nil
}

// TODO check common pattern to enforce security
func ValidatePassword(password string) error {
	if len(password) < minPasswordLength {
		return fmt.Errorf("%w: password must be at least %d characters", ErrInvalid, minPasswordLength)
	}
	if len(password) > maxPasswordLength {
		return fmt.Errorf("%w: password must be at most %d bytes", ErrInvalid, maxPasswordLength)
	}
	return nil
}
