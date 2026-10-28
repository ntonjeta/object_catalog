package domain

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrUnauthorized = errors.New("unauthorized")
	ErrInvalid      = errors.New("invalid input")
	// ErrRemoteActor marks an invalid remote actor document, as distinct from
	// malformed local input, so join can classify validation failures as 404.
	ErrRemoteActor = errors.New("invalid remote actor")
)
