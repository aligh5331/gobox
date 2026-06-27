package usecase

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

const (
	// MaxPageSize is the maximum number of files per page.
	MaxPageSize = 200
	// DefaultPageSize is used when no page_size is provided.
	DefaultPageSize = 20
)

// cursor is an opaque pagination token containing the last file ID.
type cursor struct {
	LastID uuid.UUID `json:"last_id"`
}

// encodeCursor encodes a cursor into a base64 string.
func encodeCursor(lastID uuid.UUID) string {
	c := cursor{LastID: lastID}
	data, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}

// decodeCursor decodes a base64 cursor string.
// Returns uuid.Nil if the token is empty or invalid.
func decodeCursor(token string) (uuid.UUID, error) {
	if token == "" {
		return uuid.Nil, nil
	}

	data, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return uuid.Nil, fmt.Errorf("decode cursor: %w", err)
	}

	var c cursor
	if err := json.Unmarshal(data, &c); err != nil {
		return uuid.Nil, fmt.Errorf("unmarshal cursor: %w", err)
	}

	return c.LastID, nil
}

// clampPageSize ensures pageSize is within valid bounds.
func clampPageSize(pageSize int) int {
	if pageSize <= 0 {
		return DefaultPageSize
	}
	if pageSize > MaxPageSize {
		return MaxPageSize
	}
	return pageSize
}
