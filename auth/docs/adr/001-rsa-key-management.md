# ADR-001: RSA Key Management

## Decision
Adopt restart-based RSA key rotation with deterministic `kid` thumbprints. Load one active signing key from a configurable PEM file, and optionally a second verification-only key for the overlap window. Serve both in the JWKS response.

## Context
- Auth service signs JWTs using RS256 (RSA 2048-bit minimum) per GOBOX_SPEC.md §5.1.
- All other services verify tokens locally via a cached JWKS response, so the public key must be available at a well-known URL.
- Key rotation is required for operational security. Rotated keys must remain available for the full refresh-token TTL (30 days) so tokens signed under the old key can still be verified.
- The private key is a production secret — must never leave memory or be logged.

## Options considered

### 1. Single key file, file-watch hot-reload
Watch `JWT_PRIVATE_KEY_PATH` with `fsnotify` and reload on `IN_CLOSE_WRITE`.
- **Pro:** Zero-downtime key rotation. No restart required.
- **Con:** Adds a filesystem dependency and goroutine. Race windows during rotation. Overcomplicates v1.
- **Verdict:** Defer to post-v1 hardening.

### 2. Two env vars: primary + previous key, restart-based rotation
Load `JWT_PRIVATE_KEY_PATH` (active signing key) and optionally `JWT_PREVIOUS_PRIVATE_KEY_PATH` (old key for verification only). Both are loaded at startup from PEM files. Restart to change either.
- **Pro:** Simple, no file-watch, no race conditions. All state is in memory at startup. Easy to understand.
- **Con:** Requires a restart (container rollout) for rotation. Acceptable for v1 — auth service is stateless beyond its DB.
- **Verdict:** **Chosen approach.**

### 3. Key directory with multiple numbered keys
Point `JWT_KEY_DIR` at a directory of `kid.pem` files. Load all of them. Newest is active.
- **Pro:** No env-var changes needed during rotation. Just drop a new file.
- **Con:** Directory enumeration and ordering are fragile. Rename-based activation is implicit and confusing.
- **Verdict:** Rejected — too implicit; env vars are more visible and auditable.

## Chosen approach

### Key loading (startup)

1. Read `JWT_PRIVATE_KEY_PATH` env var (required). Parse the PEM file.
2. Try PKCS#8 (`BEGIN PRIVATE KEY`) first, then fall back to PKCS#1 (`BEGIN RSA PRIVATE KEY`).
3. If `JWT_PREVIOUS_PRIVATE_KEY_PATH` env var is set (optional), load that PEM file as well.
4. Derive the `kid` (Key ID) deterministically:
   - Marshal the public key to DER.
   - SHA-256 hash the DER bytes.
   - Take the first 16 bytes → base64url-encode → `kid`.
   - This ensures the same key always produces the same `kid`, and distinct keys produce distinct kids with negligible collision probability.
5. Store both keys in an in-memory key registry:
   ```go
   type KeyEntry struct {
       PrivateKey *rsa.PrivateKey
       PublicKey  *rsa.PublicKey
       Kid        string   // base64url(sha256(public_key_der)[0:16])
       IsActive   bool     // true only for the primary signing key
   }
   ```

### Signing behavior

- **All new tokens are signed with the active key** (`IsActive == true`).
- **Token verification** tries each loaded key by matching the `kid` header.
- If a token's `kid` doesn't match any loaded key → verification fails (token from a retired key beyond the overlap window).

### JWKS endpoint structure

`GET /auth/v1/.well-known/jwks.json` returns:

```json
{
  "keys": [
    {
      "kty": "RSA",
      "kid": "abc123def456...",
      "alg": "RS256",
      "n":   "base64url-encoded-modulus",
      "e":   "AQAB",
      "use": "sig"
    }
  ]
}
```

- All loaded keys (both active and previous) are included in the `keys` array.
- Each key's `n` is the base64url-encoded modulus (big-endian unsigned integer without leading zeros).
- `e` is `AQAB` (65537) — the standard public exponent for OpenSSL-generated RSA 2048-bit keys.
- `use` is always `sig` (signature, not encryption).

### Rotation procedure (operator manual)

1. Generate a new 2048-bit RSA key:
   ```bash
   openssl genrsa -out /secrets/private.pem 2048
   ```
2. Rename the old key:
   ```bash
   mv /secrets/private.pem /secrets/private-old.pem
   ```
3. Place the new key at the original path:
   ```bash
   cp /path/to/new/private.pem /secrets/private.pem
   ```
4. Update the deployment manifest:
   - `JWT_PRIVATE_KEY_PATH=/secrets/private.pem`
   - `JWT_PREVIOUS_PRIVATE_KEY_PATH=/secrets/private-old.pem`
5. Roll out auth service (restart).
6. After the overlap window has passed (≥30 days, matching refresh token TTL):
   - Remove `JWT_PREVIOUS_PRIVATE_KEY_PATH` from the manifest.
   - Delete the old key file.
   - Roll out auth service again.

### Data structures (interface sketch)

```go
// KeyManager holds one or more RSA key entries.
// Thread-safe after initialization (immutable at runtime — restart to change).
type KeyManager interface {
    // ActiveKey returns the primary signing key and its kid.
    ActiveKey() (*rsa.PrivateKey, string)

    // JWKS returns the serializable JWKS structure for the endpoint handler.
    JWKS() *JWKSResponse

    // Verify validates a JWT's signature against any loaded key by matching kid.
    Verify(token *jwt.Token) (*rsa.PublicKey, error)
}
```

## Constraints and risks

- **Restart-gated rotation:** Between the old key's retirement and the next restart, tokens signed with the old key will fail verification. This is acceptable because the overlap window (30 days) is generous, and deployments are controlled.
- **No filesystem watch:** If the key file is modified between restarts, the service won't notice. This is by design — the key is loaded once at startup.
- **Memory risk:** Each key is an `*rsa.PrivateKey` (~2 KB for 2048-bit RSA). With at most 2 keys in memory, the footprint is negligible.
- **PEM parsing failure at startup** is a terminal error — the service must not start without a valid signing key.

## References

- GOBOX_SPEC.md §5.1 — "Auth service", "JWT signing"
- RFC 7517 (JWK) — key structure format
- RFC 7518 §6.3 — RSA key parameters
  
JWKS path is /.well-known/jwks.json (RFC 8414 standard). The /auth/v1/ prefix
is NOT used — Auth is an internal service called directly by Core, not via a
public reverse proxy. Core fetches from AUTH_HTTP_ADDR + "/.well-known/jwks.json".
