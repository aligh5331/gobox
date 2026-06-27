// Package jwtutil provides JWT parsing and validation using a cached JWKS key set.
package jwtutil

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog"
)

// JWKSCache fetches JWKS from the Auth service, caches RSA public keys
// in memory, and periodically refreshes them on a background goroutine.
type JWKSCache struct {
	keys     atomic.Value // holds *keysMap
	httpAddr string
	client   *http.Client
	log      zerolog.Logger
}

// keysMap is the internal type stored in atomic.Value.
type keysMap struct {
	byKid map[string]*rsa.PublicKey
}

// jwksResponse represents the JWKS endpoint response.
type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	KID string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// NewJWKSCache creates a JWKS cache, performs the initial fetch synchronously,
// and starts a background refresh loop. Returns an error if the initial fetch fails.
func NewJWKSCache(ctx context.Context, authHTTPAddr string, refreshInterval time.Duration, log zerolog.Logger) (*JWKSCache, error) {
	c := &JWKSCache{
		httpAddr: authHTTPAddr,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		log: log,
	}
	// Seed the cache with an empty map so reads never panic.
	c.keys.Store(&keysMap{byKid: map[string]*rsa.PublicKey{}})

	if err := c.fetch(ctx); err != nil {
		return nil, fmt.Errorf("jwtutil: initial jwks fetch: %w", err)
	}
	go c.refreshLoop(ctx, refreshInterval)
	return c, nil
}

// fetch retrieves the JWKS from the Auth HTTP endpoint and updates the cache.
func (c *JWKSCache) fetch(ctx context.Context) error {
	url := fmt.Sprintf("%s/auth/v1/.well-known/jwks.json", c.httpAddr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("jwks: create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("jwks: http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks: endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("jwks: read body: %w", err)
	}

	var jwks jwksResponse
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("jwks: unmarshal: %w", err)
	}

	if len(jwks.Keys) == 0 {
		return fmt.Errorf("jwks: response contains no keys")
	}

	parsed := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, key := range jwks.Keys {
		if key.Kty != "RSA" {
			continue
		}
		pk, err := rsaPublicKeyFromJWK(key)
		if err != nil {
			c.log.Warn().Str("kid", key.KID).Err(err).Msg("jwtutil: failed to parse JWKS key, skipping")
			continue
		}
		parsed[key.KID] = pk
	}

	if len(parsed) == 0 {
		return fmt.Errorf("jwks: no valid RSA keys found in response")
	}

	c.keys.Store(&keysMap{byKid: parsed})
	c.log.Info().Int("keys", len(parsed)).Msg("jwtutil: jwks cache updated")
	return nil
}

func rsaPublicKeyFromJWK(key jwkKey) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}, nil
}

// refreshLoop periodically re-fetches JWKS.
func (c *JWKSCache) refreshLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.fetch(ctx); err != nil {
				c.log.Warn().Err(err).Msg("jwtutil: jwks refresh failed, retaining previous keys")
			}
		}
	}
}

// ParseAndValidate parses a bearer token, verifies its RS256 signature against
// the cached JWKS key matching the token's kid header, and returns the validated Claims.
func (c *JWKSCache) ParseAndValidate(tokenStr string) (*Claims, error) {
	km := c.keys.Load().(*keysMap)

	// Parse the token header to extract kid without verification.
	token, _, err := new(jwt.Parser).ParseUnverified(tokenStr, &Claims{})
	if err != nil {
		return nil, fmt.Errorf("jwtutil: parse unverified: %w", err)
	}

	kid, ok := token.Header["kid"].(string)
	if !ok || kid == "" {
		return nil, fmt.Errorf("jwtutil: token missing kid header")
	}

	pk, ok := km.byKid[kid]
	if !ok {
		return nil, fmt.Errorf("jwtutil: unknown kid %q", kid)
	}

	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("jwtutil: unexpected signing method %v", t.Header["alg"])
		}
		return pk, nil
	})
	if err != nil {
		return nil, fmt.Errorf("jwtutil: %w", err)
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("jwtutil: token invalid")
	}
	return claims, nil
}


