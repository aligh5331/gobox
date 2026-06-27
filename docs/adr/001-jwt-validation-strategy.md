# ADR-001: JWT Validation Strategy for Core API

## Decision

Core API will load the Auth service's RSA public key via a JWKS endpoint at startup, cache it in memory, and refresh it every 5 minutes. The JWT middleware will inject `user_id` (from `sub`) and `session_id` (from `sid`) into the Echo context. Invalid tokens return `401` with the standardized error envelope.

## Context

Core API is a stateless REST gateway. Per GOBOX_SPEC.md §5.2, it must validate JWT tokens **locally** — no gRPC call to Auth on each request. The Auth service (§5.1) exposes a JWKS endpoint at `GET /auth/v1/.well-known/jwks.json` and a gRPC `GetPublicKey` RPC that returns `jwks_json`.

Every incoming REST request (except `/api/v1/auth/register`, `/api/v1/auth/login`, `/api/v1/auth/refresh`, and `/health`) must pass through JWT validation. The middleware needs to extract the authenticated user's identity for downstream use cases.

## Options considered

### 1. JWKS fetch at startup + periodic refresh (chosen)

Fetch `GET /auth/v1/.well-known/jwks.json` on Core API boot, parse the JWKS set, cache the RSA public key in-memory (`sync.Map` or `atomic.Value` keyed by `kid`), and re-fetch every 5 minutes on a background goroutine.

- **Pros:** Stateless, no per-request gRPC overhead, supports key rotation via `kid` lookup, simple to implement with `github.com/golang-jwt/jwt/v5`.
- **Cons:** Stale key window up to 5 minutes after rotation (acceptable given 15-minute token TTL and typical rotation lead time).

### 2. gRPC `GetPublicKey` call per request

Call Auth's `GetPublicKey` gRPC on every request, parse the PEM/JWKS response, and validate in the middleware.

- **Pros:** Always has the freshest key.
- **Cons:** Introduces latency, network dependency, and load on Auth for every request. Violates the spec's requirement for local validation.

### 3. Static PEM file mounted as a secret

Load `JWT_PUBLIC_KEY_PATH` PEM file from disk at startup only.

- **Pros:** Simplest implementation, no network dependency for key material after boot.
- **Cons:** Key rotation requires a pod restart or config reload. No graceful overlap window for multiple keys. Does not align with the JWKS design in the spec.

## Chosen approach

**Option 1 — JWKS fetch at startup + periodic refresh every 5 minutes.**

### Detailed design

1. **Startup sequence:**
   - Core reads `AUTH_HTTP_ADDR` from env (e.g. `http://auth:8080`).
   - On `main.go` init, a `JWKSCache` component is constructed.
   - `JWKSCache.Fetch()` is called synchronously before the HTTP server starts. If the first fetch fails, the application **terminates** (fail-fast) — no point running without a valid key.

2. **Refresh loop:**
   - A background goroutine calls `Fetch()` every 5 minutes (`time.Ticker`).
   - If a refresh fails, the previous key set is retained (stale-ok). A warning is logged.
   - Refresh errors do **not** trigger a shutdown.

3. **Cache structure:**
   - The JWKS response is parsed into `map[string]*rsa.PublicKey` keyed by `kid`.
   - An `atomic.Value` holds the current map, so reads are lock-free.
   - If the response contains no keys, the previous set is preserved.

4. **Middleware extraction:**
   - Echo middleware reads the `Authorization: Bearer <token>` header.
   - Token is parsed with `jwt.ParseWithClaims` using `jwt.MapClaims` or a custom `Claims` struct.
   - The `kid` from the JWT header is matched against the cached keys.
   - On match, the `rsa.PublicKey` is used to verify the RS256 signature.
   - On mismatch (unknown kid), validation fails immediately — this catches tokens signed by a rotated key before the cache refreshes.

5. **Context injection:**
   ```go
   // Injected into echo.Context
   c.Set("user_id", claims["sub"])      // string
   c.Set("session_id", claims["sid"])   // string
   ```
   - A helper function `GetUserID(c echo.Context) (string, bool)` is provided for use case handlers to extract these values cleanly.
   - The injected values are **strings** (UUIDs as returned by the JWT). Conversion to `uuid.UUID` is done by the consuming handler.

6. **Public endpoints (skip middleware):**
   - `POST /api/v1/auth/register`
   - `POST /api/v1/auth/login`
   - `POST /api/v1/auth/refresh`
   - `GET /health`
   - These are registered on a group that does **not** include the JWT middleware, or the middleware skips them based on path matching.

7. **Error response:**
   - When the token is missing, malformed, expired, or signature-invalid:
     ```json
     {
       "error": {
         "code": "UNAUTHORIZED",
         "message": "invalid or expired token"
       }
     }
     ```
   - Specific messages for different failure modes (expired, malformed, missing) but always code `UNAUTHORIZED` and status `401`.

### Env vars

| Var | Purpose | Example |
|-----|---------|---------|
| `AUTH_HTTP_ADDR` | Base URL for Auth HTTP server (JWKS endpoint) | `http://auth:8080` |
| `JWKS_REFRESH_INTERVAL` | Refresh interval (default `5m`) | `5m` |

### Package structure

```
core/
└── pkg/
    └── jwtutil/
        ├── jwks.go        ← JWKSCache: fetch, refresh, lookup
        ├── claims.go      ← custom Claims struct (sub, sid, email, name, jti)
        └── middleware.go  ← Echo middleware func
```

## Constraints and risks

- **Clock skew:** JWKS fetch success on first boot is a hard dependency. If Auth is not yet reachable, Core will crash-loop. Mitigation: use Docker Compose `depends_on` with health check on Auth's `/health`.
- **Key rotation gap:** If Auth rotates keys and Core fails to refresh for >5 minutes (e.g., network partition), tokens signed with the new key will be rejected. Mitigation: Auth should keep the old key in the JWKS for at least the overlap window (5 min + token TTL margin).
- **Memory safety:** The JWKS key map is small (<1 KB for 2 keys). No risk.
- **Concurrency:** `atomic.Value` ensures safe concurrent reads. The refresh goroutine is the only writer.

