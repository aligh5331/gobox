// Package usecase implements all short link use cases (application layer).
package usecase

import "errors"

// Business-rule sentinel errors returned by use cases.
// These are mapped to gRPC status codes in the interface layer.
var (
	ErrLinkNotFound     = errors.New("LINK_NOT_FOUND")
	ErrPermissionDenied = errors.New("PERMISSION_DENIED")
	ErrMissingFileID    = errors.New("MISSING_FILE_ID")
	ErrSlugCollision    = errors.New("SLUG_COLLISION_EXHAUSTED")
	ErrLinkExpired      = errors.New("LINK_EXPIRED")
)
