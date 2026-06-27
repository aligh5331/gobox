// Package usecase implements all file upload use cases (application layer).
package usecase

import "errors"

// Business-rule sentinel errors returned by use cases.
// These are mapped to gRPC status codes in the interface layer.
var (
	ErrFileNotFound       = errors.New("FILE_NOT_FOUND")
	ErrPermissionDenied   = errors.New("PERMISSION_DENIED")
	ErrInvalidName        = errors.New("INVALID_NAME")
	ErrInvalidSize        = errors.New("INVALID_SIZE")
	ErrInvalidMimeType    = errors.New("INVALID_MIME_TYPE")
	ErrInvalidPageToken   = errors.New("INVALID_PAGE_TOKEN")
	ErrFileNotPending     = errors.New("FILE_NOT_PENDING")
	ErrFileNotReady       = errors.New("FILE_NOT_READY")
	ErrObjectNotExists    = errors.New("OBJECT_NOT_EXISTS")
)
