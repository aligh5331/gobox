package jwtutil

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"

	"github.com/golang-jwt/jwt/v5"
)

// JWKSEntry is a single key in the JWKS response.
type JWKSEntry struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Use string `json:"use"`
}

// JWKSResponse is the top-level JWKS document.
type JWKSResponse struct {
	Keys []JWKSEntry `json:"keys"`
}

// KeyManager holds one or more RSA key entries for signing and verification.
// It is immutable after construction — restart the service to change keys.
type KeyManager struct {
	activePrivateKey *rsa.PrivateKey
	activeKid        string
	previousPublicKey *rsa.PublicKey
	previousKid      string
}

// NewKeyManager loads RSA keys from PEM files.
// privateKeyPath is required (active signing key).
// previousKeyPath is optional (old key for verification only).
func NewKeyManager(privateKeyPath, previousKeyPath string) (*KeyManager, error) {
	km := &KeyManager{}

	privKey, err := loadPrivateKey(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("jwtutil: load private key: %w", err)
	}
	km.activePrivateKey = privKey
	km.activeKid = kidFromPublicKey(&privKey.PublicKey)

	if previousKeyPath != "" {
		prevKey, err := loadPrivateKey(previousKeyPath)
		if err != nil {
			return nil, fmt.Errorf("jwtutil: load previous key: %w", err)
		}
		km.previousPublicKey = &prevKey.PublicKey
		km.previousKid = kidFromPublicKey(km.previousPublicKey)
	}

	return km, nil
}

// ActiveKey returns the primary signing key and its kid.
func (km *KeyManager) ActiveKey() (*rsa.PrivateKey, string) {
	return km.activePrivateKey, km.activeKid
}

// JWKS returns the serializable JWKS structure including all loaded keys.
func (km *KeyManager) JWKS() *JWKSResponse {
	entries := make([]JWKSEntry, 0, 2)

	entries = append(entries, jwkEntry(&km.activePrivateKey.PublicKey, km.activeKid))

	if km.previousPublicKey != nil {
		entries = append(entries, jwkEntry(km.previousPublicKey, km.previousKid))
	}

	return &JWKSResponse{Keys: entries}
}

// Sign creates a signed JWT string from the given claims.
func (km *KeyManager) Sign(claims jwt.Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = km.activeKid

	signed, err := token.SignedString(km.activePrivateKey)
	if err != nil {
		return "", fmt.Errorf("jwtutil: sign token: %w", err)
	}
	return signed, nil
}

// KeyFunc returns a jwt.Keyfunc suitable for jwt.Parse.
// It looks up the key by the token's kid header.
func (km *KeyManager) KeyFunc() jwt.Keyfunc {
	return func(token *jwt.Token) (interface{}, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("jwtutil: missing kid in token header")
		}

		if kid == km.activeKid {
			return &km.activePrivateKey.PublicKey, nil
		}
		if km.previousPublicKey != nil && kid == km.previousKid {
			return km.previousPublicKey, nil
		}

		return nil, fmt.Errorf("jwtutil: unknown kid: %s", kid)
	}
}

// Verify parses and validates a JWT string, returning the raw token.
func (km *KeyManager) Verify(tokenString string) (*jwt.Token, error) {
	token, err := jwt.Parse(tokenString, km.KeyFunc())
	if err != nil {
		return nil, fmt.Errorf("jwtutil: verify token: %w", err)
	}
	return token, nil
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}

	// Try PKCS#8 first
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("key is not RSA")
		}
		return rsaKey, nil
	}

	// Fall back to PKCS#1
	rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key (PKCS#1): %w", err)
	}
	return rsaKey, nil
}

// kidFromPublicKey derives a deterministic key identifier.
// kid = base64url( SHA256(DER of public key)[0:16] )
func kidFromPublicKey(pub *rsa.PublicKey) string {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		panic("jwtutil: marshal public key: " + err.Error())
	}
	hash := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(hash[:16])
}

func jwkEntry(pub *rsa.PublicKey, kid string) JWKSEntry {
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	return JWKSEntry{
		Kty: "RSA",
		Kid: kid,
		Alg: "RS256",
		N:   n,
		E:   e,
		Use: "sig",
	}
}


