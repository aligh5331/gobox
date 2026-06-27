package repository

import "errors"

var (
	// ErrNotFound is returned when a requested entity does not exist.
	ErrNotFound = errors.New("repository: not found")

	// ErrTokenReuse is returned when a refresh token rotation fails because
	// the old session has already been deleted (token theft detected).
	ErrTokenReuse = errors.New("repository: token reuse detected")
)
