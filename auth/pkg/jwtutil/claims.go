// Package jwtutil provides JWT signing, verification, and key management.
package jwtutil

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AccessTokenClaims are the custom claims embedded in every access token JWT.
type AccessTokenClaims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
	Name  string `json:"name"`
	SID   string `json:"sid"`
}

// NewAccessTokenClaims constructs a populated AccessTokenClaims with a random
// jti and the given identity and session identifiers.
func NewAccessTokenClaims(userID, email, name, sessionID string, now time.Time) AccessTokenClaims {
	return AccessTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			ID:        uuid.New().String(),
		},
		Email: email,
		Name:  name,
		SID:   sessionID,
	}
}
