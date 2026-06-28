package repository

import "errors"

var (
	// ErrNotFound is returned when a requested entity does not exist.
	ErrNotFound = errors.New("repository: not found")

	// ErrDuplicateSlug is returned when a slug violates the unique constraint.
	ErrDuplicateSlug = errors.New("repository: duplicate slug")
)
