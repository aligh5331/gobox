// Package slug generates URL-safe random slugs using crypto/rand and base62 encoding.
package slug

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

const (
	// SlugLength is the number of characters in a generated slug.
	SlugLength = 6

	// MaxRetries is the maximum number of slug generation attempts.
	MaxRetries = 5

	base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// Generator creates unique slugs.
type Generator struct{}

// NewGenerator creates a new slug Generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// Generate produces a random 6-character base62 slug from crypto/rand.
func (g *Generator) Generate() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("slug: rand read: %w", err)
	}
	n := binary.BigEndian.Uint64(buf[:])
	return base62Encode(n)[:SlugLength], nil
}

// base62Encode encodes a uint64 to a base62 string.
func base62Encode(n uint64) string {
	if n == 0 {
		return string(base62Alphabet[0])
	}

	var chars [11]byte
	i := len(chars)
	for n > 0 && i > 0 {
		i--
		chars[i] = base62Alphabet[n%62]
		n /= 62
	}

	return string(chars[i:])
}
