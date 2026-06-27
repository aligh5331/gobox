# ADR-003: JWT Claims

## Decision
Adopt a fixed seven-claim JWT for access tokens and an opaque bearer token (non-JWT) for refresh tokens. Access tokens live 15 minutes; refresh tokens live 30 days and are bcrypt-hashed in the database.

## Context
- Auth service issues RS256-signed JWTs that other services verify locally.
- The JWT must carry enough identity information to authorize requests (user ID, email, name) without requiring a gRPC call back to Auth on every request.
- Token revocation is handled via short access token TTL, not an online blacklist — so `jti` and `sid` are included as future hooks for revocation or session pinning.

## Options considered

### 1. Minimal JWT (sub + exp only)
Strictly spec-compliant JWT with only `sub` and `exp`.
- **Pro:** Smallest token size (~200 bytes). Least surface area.
- **Con:** Downstream services would need to call Auth's `GetUser` gRPC to get email/name for every request, defeating the point of local verification.
- **Verdict:** Rejected — violates the "no auth round-trip" requirement from GOBOX_SPEC.md §1.

### 2. Full identity JWT (chosen)
Include `sub`, `email`, `name`, `iat`, `exp`, `jti`, `sid` as specified in GOBOX_SPEC.md §3 "JWT design".
- **Pro:** All downstream services have user identity without any RPC call. Token is self-contained.
- **Con:** Larger token (~400 bytes). Email/name changes during the 15-minute window are not reflected until token refresh.
- **Verdict:** **Chosen approach** — matches the spec exactly.

### 3. Opaque refresh token + JWT access token (chosen)
Refresh token is an opaque random string (not a JWT), stored bcrypt-hashed in the DB.
- **Pro:** No signing overhead. No replay risk (rotation detection via DELETE). Not inspectable by clients. Can be revoked server-side.
- **Con:** Requires DB lookup on refresh. More complex than a long-lived JWT refresh token.
- **Verdict:** **Chosen approach** — matches the spec ("opaque random string, stored in Auth DB").

## Chosen approach

### Access token claims

| Claim | Type | Value | Required |
|-------|------|-------|----------|
| `sub` | `string` | User UUID (`user.id`) | Yes |
| `email` | `string` | User's email address | Yes |
| `name` | `string` | User's display name | Yes |
| `iat` | `int` | Unix timestamp of issuance | Yes |
| `exp` | `int` | `iat + 900` (15 minutes) | Yes |
| `jti` | `string` | UUID v4 — unique per token | Yes |
| `sid` | `string` | Session UUID — links to Session row | Yes |

Serialized JWT payload:

```json
{
  "sub":   "c9a1c6a0-3f1a-4b5e-8d7f-9e0b1c2d3e4f",
  "email": "user@example.com",
  "name":  "Ali G.",
  "iat":   1761456000,
  "exp":   1761456900,
  "jti":   "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "sid":   "f0e1d2c3-b4a5-6789-0fed-cba987654321"
}
```

**Why `email` and `name` in the JWT?** Core API and other services need to display user info in responses (e.g., `GET /api/v1/me`). Including these claims avoids a gRPC round-trip. The 15-minute staleness is acceptable — user profile changes are propagated on next token refresh.

**Why `jti`?** Provides a unique token identifier for future revocation blacklist (gRPC endpoint `ValidateSession` or a Redis-based JTI blacklist for sensitive operations). Not used in v1 but reserved in the claim set.

**Why `sid`?** Links the JWT to a specific session row. Used by Logout (which takes `session_id` from the JWT's `sid`). Also allows future rich session management.

### Refresh token format

- **Generation:** 32 cryptographically random bytes (`crypto/rand`), base64url-encoded without padding → 43-character ASCII string.
- **Storage:** bcrypt hash (cost 10) of the raw 43-character string. Stored in `sessions.refresh_token`.
- **Transmission:** The raw 43-character string is returned to the client. The server never stores or logs the raw value.
- **Validation:** On refresh request, the server loads candidate sessions for the user, computes bcrypt on the provided token, and compares with each stored hash.
- **TTL:** 30 days, matching `sessions.expires_at`.

**Why bcrypt instead of SHA-256?** If the `sessions` table is ever leaked, bcrypt's cost factor makes reverse-engineering refresh tokens computationally expensive. SHA-256 would allow instant rainbow-table lookup.

**Why 32 bytes?** 32 bytes = 256 bits of entropy. Sufficient to make brute-force infeasible even without bcrypt (though we use bcrypt as defense-in-depth).

**Why not a JWT for refresh?** Refresh tokens must be revocable server-side (LogoutAll). A JWT cannot be revoked without a blacklist. An opaque token looked up in the DB is revocable by setting `revoked = true`.

### Token issuance flow

```
Login:
  1. Verify password (bcrypt)
  2. Create Session record (UUID, user_id, refresh_token_hash, expires_at = NOW+30d)
  3. Generate JWT:
     sub = user.id
     email = user.email
     name = user.name
     iat = NOW
     exp = NOW + 900s
     jti = uuid.New()
     sid = session.id
  4. Sign JWT with active RSA private key (RS256)
  5. Return { access_token, refresh_token (raw), expires_in: 900 }

Refresh:
  1. Receive opaque refresh token
  2. Look up session by bcrypt(token) match
  3. Verify session is not revoked and not expired
  4. Begin transaction: DELETE old session → INSERT new session
  5. Generate new JWT with new sid, new jti
  6. Return { access_token, refresh_token (raw, new), expires_in: 900 }
```

### Token validation (downstream services)

Downstream services (Core, FileUpload, Shortener, ThumbGen):

1. Extract `Authorization: Bearer <token>` from the gRPC metadata or HTTP header.
2. Parse JWT without verification to extract `kid` header.
3. Fetch JWKS from Auth (`GET /auth/v1/.well-known/jwks.json`), cache in memory, refresh every 5 minutes.
4. Select public key matching the token's `kid`.
5. Verify RS256 signature.
6. Check `exp` ≥ now (reject expired — return 401/Unauthenticated).
7. Use `sub` as the requesting user's identity.

Downstream services do **not** call Auth's `ValidateSession` gRPC for normal request processing. The JWT is the sole trust boundary.

## Constraints and risks

- **Token size:** ~400 bytes with 7 claims and RS256 signature. Acceptable for HTTP headers and gRPC metadata.
- **Email/name staleness:** If a user changes their email, the JWT is stale for up to 15 minutes. Mitigation: force re-login after email change (revoke all sessions). Acceptable for v1.
- **bcrypt cost on refresh:** If a user has many active sessions (>100), the refresh path must bcrypt-compare each stored hash. At cost 10, 100 compares ≈ 10 seconds — unacceptable. Mitigation: either limit active sessions per user (configurable cap, e.g., 20), or introduce a `refresh_token_hash_prefix` column with the first 8 bytes of the raw hash as a fast pre-filter. For v1 cap sessions at 20 per user.
- **jti not checked in v1:** The `jti` claim is included but not validated against a blacklist. This is by design. If JTI blacklisting is added later, use Redis with the token's expiry as the key's TTL.

## References

- GOBOX_SPEC.md §3 — "JWT design", "Session management"
- GOBOX_SPEC.md §5.1 — "Use cases" table (AccessToken TTL, RefreshToken rotation)
- RFC 7519 — JWT claim names and types
- RFC 7518 §3.1 — RS256 algorithm
- OWASP — bcrypt for password/token storage
