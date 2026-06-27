package usecase

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

const (
	// BcryptCost is the bcrypt hashing cost used for passwords and refresh tokens.
	BcryptCost = 12

	refreshTokenBytes = 32
)

// validatePassword checks that the password meets minimum strength requirements:
// at least 8 characters, containing uppercase, lowercase, and a digit.
func validatePassword(password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}

	var hasUpper, hasLower, hasDigit bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		}
	}

	if !hasUpper || !hasLower || !hasDigit {
		return ErrWeakPassword
	}
	return nil
}

// validateEmail performs a basic check that the email is non-empty and contains an @.
func validateEmail(email string) error {
	if email == "" || !strings.Contains(email, "@") {
		return ErrInvalidCredentials
	}
	return nil
}

// generateRefreshToken creates a cryptographically random opaque token.
// Returns a 43-character base64url-encoded string.
func generateRefreshToken() (string, error) {
	b := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken computes a bcrypt hash of the given token string.
func hashToken(token string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(token), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash token: %w", err)
	}
	return string(hash), nil
}
