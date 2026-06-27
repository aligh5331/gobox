package usecase

import "github.com/golang-jwt/jwt/v5"

// TokenSigner is the interface for signing JWT claims.
// The concrete implementation is in pkg/jwtutil.KeyManager.
type TokenSigner interface {
	Sign(claims jwt.Claims) (string, error)
}
