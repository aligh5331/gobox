// Package usecase implements all auth use cases (application layer).
package usecase

import "errors"

// Business-rule sentinel errors returned by use cases.
// These are mapped to gRPC status codes in the interface layer.
var (
	ErrEmailAlreadyExists     = errors.New("EMAIL_ALREADY_EXISTS")
	ErrWeakPassword           = errors.New("WEAK_PASSWORD")
	ErrInvalidCredentials     = errors.New("INVALID_CREDENTIALS")
	ErrUserNotFound           = errors.New("USER_NOT_FOUND")
	ErrSessionNotFound        = errors.New("SESSION_NOT_FOUND")
	ErrSessionExpired         = errors.New("SESSION_EXPIRED")
	ErrSessionRevoked         = errors.New("SESSION_REVOKED")
	ErrSessionAlreadyRevoked  = errors.New("SESSION_ALREADY_REVOKED")
	ErrTokenTheftDetected     = errors.New("TOKEN_THEFT_DETECTED")
	ErrInvalidName            = errors.New("INVALID_NAME")
	ErrInvalidPassword        = errors.New("INVALID_PASSWORD")
)
