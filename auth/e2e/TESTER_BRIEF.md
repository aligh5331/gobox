# Tester Brief — Auth Service

> **Phase:** 1 — Auth Service  
> **Spec:** `GOBOX_SPEC.md §5.1`, `features/auth.feature`  
> **Module:** `github.com/aligh5331/gobox/auth`  
> **E2E suite location:** `e2e/`

---

## Services to spin up

| # | Service | Directory | Compose file | HEALTHCHECK |
|---|---------|-----------|-------------|-------------|
| 1 | postgres | `auth/` | `auth/docker-compose.yml` | yes (`pg_isready -U gobox`) |
| 2 | auth | `auth/` | `auth/docker-compose.yml` | yes (`wget -qO- http://localhost:8080/health`) |

### Startup order

1. `docker compose -f auth/docker-compose.yml up -d postgres` — wait for healthy
2. `docker compose -f auth/docker-compose.yml up -d auth` — wait for healthy

The auth service depends on postgres (`depends_on: postgres condition: service_healthy`), but Docker Compose v2 does not wait for transitive dependencies. The tester should explicitly wait for postgres before starting auth.

---

## Port map

| Service | Host address | Container port | Purpose |
|---------|-------------|----------------|---------|
| auth (gRPC) | `localhost:8081` | 8081 | AuthService RPCs |
| auth (HTTP) | `localhost:8084` | 8080 | Health + JWKS |
| postgres | `localhost:5432` | 5432 | Database |

> Note: **8084** is the host-mapped port for auth HTTP. The spec says auth's HTTP port is 8080, but the compose maps host:8084 → container:8080 to avoid conflict with other services on the host. The healthcheck inside the container uses `http://localhost:8080/health` (correct).

---

## Scenarios

All gRPC calls target `localhost:8081` using the `AuthService` client from `github.com/aligh5331/gobox-proto/gen/auth/v1`.  
All HTTP calls target `localhost:8082` on the host.

Save values are carried between scenarios as Go variables in the test suite. The test suite must manage a `userID`, `accessToken`, `refreshToken`, and `sessionID` as the state carried between scenarios.

### Scenario 1 — Health endpoint
Depends on: none

Steps:
  1. `GET http://localhost:8082/health` — empty request
  2. Assert: HTTP 200, response body is `{"status":"ok"}`
  3. Save: none
Auth: none

### Scenario 2 — JWKS endpoint
Depends on: none

Steps:
  1. `GET http://localhost:8082/.well-known/jwks.json` — empty request
  2. Assert: HTTP 200, response body is a JSON object with a `"keys"` array containing at least one key object that has fields: `kty`, `kid`, `alg`, `n`, `e`, `use`
  3. Save: `jwks` — the full response body (needed for token verification in downstream scenarios)
Auth: none

### Scenario 3 — Register a new user successfully
Depends on: none

Steps:
  1. `GRPC localhost:8081 AuthService.Register` — request: `{email: "alice@example.com", name: "Alice", password: "ValidPass1!"}`
  2. Assert: gRPC status OK. Response has:
     - `user.email` = `"alice@example.com"`, `user.name` = `"Alice"`
     - `tokens.access_token` is a non-empty JWT string (3 dot-separated base64url segments)
     - `tokens.refresh_token` is a 43-character base64url string
     - `tokens.expires_in` = `900`
     - `session.id` is a valid UUID v4
     - `session.user_id` matches `user.id`
     - `session.expires_at` is approximately now + 30 days
  3. Save:
     - `userID_1` = `response.user.id`
     - `accessToken_1` = `response.tokens.access_token`
     - `refreshToken_1` = `response.tokens.refresh_token`
     - `sessionID_1` = `response.session.id`
Auth: none (register is unauthenticated)

### Scenario 4 — Register with duplicate email fails
Depends on: Scenario 3 (alice@example.com exists)

Steps:
  1. `GRPC localhost:8081 AuthService.Register` — request: `{email: "alice@example.com", name: "Bob", password: "AnotherPass1!"}`
  2. Assert: gRPC status code = `AlreadyExists`. Error message contains `EMAIL_ALREADY_EXISTS`
Auth: none

### Scenario 5 — Register with weak password fails
Depends on: none

Steps:
  1. `GRPC localhost:8081 AuthService.Register` — request: `{email: "bob@example.com", name: "Bob", password: "short"}`
  2. Assert: gRPC status code = `InvalidArgument`. Error message contains `WEAK_PASSWORD`
Auth: none

### Scenario 6 — Login with valid credentials succeeds
Depends on: Scenario 3 (alice@example.com exists)

Steps:
  1. `GRPC localhost:8081 AuthService.Login` — request: `{email: "alice@example.com", password: "ValidPass1!", user_agent: "go-e2e-test", ip: "127.0.0.1"}`
  2. Assert: gRPC status OK. Response has:
     - `user.email` = `"alice@example.com"`
     - `tokens.access_token` is a non-empty JWT
     - `tokens.refresh_token` is a 43-character base64url string
     - `tokens.expires_in` = `900`
     - `session.id` is a valid UUID v4 (different from sessionID_1 if alice logged in before)
     - `session.expires_at` is approximately now + 30 days
  3. Save:
     - `userID` = `response.user.id` (should match userID_1)
     - `accessToken` = `response.tokens.access_token`
     - `refreshToken` = `response.tokens.refresh_token`
     - `sessionID` = `response.session.id`
Auth: none

### Scenario 7 — Login with wrong password fails
Depends on: Scenario 3

Steps:
  1. `GRPC localhost:8081 AuthService.Login` — request: `{email: "alice@example.com", password: "WrongPass1!", user_agent: "", ip: ""}`
  2. Assert: gRPC status code = `Unauthenticated`. Error message contains `INVALID_CREDENTIALS`
Auth: none

### Scenario 8 — Login with unknown email fails
Depends on: none

Steps:
  1. `GRPC localhost:8081 AuthService.Login` — request: `{email: "unknown@example.com", password: "AnyPass1!", user_agent: "", ip: ""}`
  2. Assert: gRPC status code = `Unauthenticated`. Error message contains `INVALID_CREDENTIALS`
Auth: none

### Scenario 9 — Refresh a valid token successfully rotates credentials
Depends on: Scenario 6 (has a valid refreshToken)

Steps:
  1. `GRPC localhost:8081 AuthService.RefreshToken` — request: `{refresh_token: "<refreshToken from Scenario 6>"}`
  2. Assert: gRPC status OK. Response has:
     - `tokens.access_token` is a non-empty JWT (different from the old accessToken)
     - `tokens.refresh_token` is a 43-character base64url string (different from the old refreshToken)
     - `tokens.expires_in` = `900`
  3. Save:
     - `newAccessToken` = `response.tokens.access_token`
     - `newRefreshToken` = `response.tokens.refresh_token`
Auth: none

### Scenario 10 — Refresh with a consumed (already-rotated) token fails
Depends on: Scenario 9 (the original refreshToken from Scenario 6 was rotated in Scenario 9)

Steps:
  1. `GRPC localhost:8081 AuthService.RefreshToken` — request: `{refresh_token: "<original refreshToken from Scenario 6>"}`
  2. Assert: gRPC status code = `Unauthenticated`. Error message contains `TOKEN_THEFT_DETECTED`
Auth: none

### Scenario 11 — Refresh with a revoked session fails
Depends on: Scenario 9 (has a valid newRefreshToken — use it, then revoke the session)

Note: This scenario requires programmatically revoking the session first (e.g., calling Logout with the sessionID from Scenario 6). Or the tester can create a fresh login, then call Logout, then attempt refresh.

Steps:
  1. `GRPC localhost:8081 AuthService.Login` — request: `{email: "alice@example.com", password: "ValidPass1!", user_agent: "", ip: ""}` — save the `revocableSessionID` and `revocableRefreshToken`
  2. `GRPC localhost:8081 AuthService.Logout` — request: `{session_id: "<revocableSessionID>"}`
  3. `GRPC localhost:8081 AuthService.RefreshToken` — request: `{refresh_token: "<revocableRefreshToken>"}`
  4. Assert (step 3): gRPC status code = `Unauthenticated`. Error message contains `SESSION_REVOKED`
Auth: none

### Scenario 12 — Login, then GetUser returns the user
Depends on: Scenario 6 (has a valid userID and accessToken)

Steps:
  1. `GRPC localhost:8081 AuthService.GetUser` — request: `{user_id: "<userID from Scenario 6>"}`
  2. Assert: gRPC status OK. Response has:
     - `id` = `<userID>`
     - `email` = `"alice@example.com"`
     - `name` = `"Alice"`
     - `created_at` and `updated_at` are valid timestamps
Auth: none (GetUser is an internal gRPC — the response contains whatever the caller's user_id maps to)

### Scenario 13 — GetUser with unknown user_id fails
Depends on: none

Steps:
  1. `GRPC localhost:8081 AuthService.GetUser` — request: `{user_id: "00000000-0000-0000-0000-000000000000"}`
  2. Assert: gRPC status code = `NotFound`. Error message contains `USER_NOT_FOUND`
Auth: none

### Scenario 14 — UpdateProfile changes the user's display name
Depends on: Scenario 6 (has a valid userID)

Steps:
  1. `GRPC localhost:8081 AuthService.UpdateProfile` — request: `{user_id: "<userID>", name: "Alice Updated"}`
  2. Assert: gRPC status OK. Response has `user.name` = `"Alice Updated"` and `user.email` = `"alice@example.com"`
  3. Save: `updatedName` = `"Alice Updated"`
Auth: none

### Scenario 15 — UpdateProfile with empty name fails
Depends on: Scenario 6

Steps:
  1. `GRPC localhost:8081 AuthService.UpdateProfile` — request: `{user_id: "<userID>", name: ""}`
  2. Assert: gRPC status code = `InvalidArgument`. Error message contains `INVALID_NAME`
Auth: none

### Scenario 16 — ChangePassword with correct old password succeeds
Depends on: Scenario 6 (has a valid userID)

Steps:
  1. `GRPC localhost:8081 AuthService.ChangePassword` — request: `{user_id: "<userID>", old_password: "ValidPass1!", new_password: "NewPass2!"}`
  2. Assert: gRPC status OK. Response is empty (`google.protobuf.Empty`)
Auth: none

### Scenario 17 — Login with new password after ChangePassword
Depends on: Scenario 16 (password was changed to "NewPass2!")

Steps:
  1. `GRPC localhost:8081 AuthService.Login` — request: `{email: "alice@example.com", password: "NewPass2!", user_agent: "", ip: ""}`
  2. Assert: gRPC status OK. Response has:
     - `tokens.access_token` is a valid JWT
     - `tokens.refresh_token` is a 43-character string
  3. Save:
     - `userID` = `response.user.id`
     - `accessToken` = `response.tokens.access_token`
     - `refreshToken` = `response.tokens.refresh_token`
     - `sessionID` = `response.session.id`
Auth: none

### Scenario 18 — Login with old password fails after ChangePassword
Depends on: Scenario 16

Steps:
  1. `GRPC localhost:8081 AuthService.Login` — request: `{email: "alice@example.com", password: "ValidPass1!", user_agent: "", ip: ""}`
  2. Assert: gRPC status code = `Unauthenticated`. Error message contains `INVALID_CREDENTIALS`
Auth: none

### Scenario 19 — ChangePassword with wrong old password fails
Depends on: Scenario 17 (has a valid userID, password is "NewPass2!")

Steps:
  1. `GRPC localhost:8081 AuthService.ChangePassword` — request: `{user_id: "<userID>", old_password: "WrongPass1!", new_password: "AnotherNew3!"}`
  2. Assert: gRPC status code = `InvalidArgument`. Error message contains `INVALID_PASSWORD`
Auth: none

### Scenario 20 — ChangePassword with weak new password fails
Depends on: Scenario 17

Steps:
  1. `GRPC localhost:8081 AuthService.ChangePassword` — request: `{user_id: "<userID>", old_password: "NewPass2!", new_password: "short"}`
  2. Assert: gRPC status code = `InvalidArgument`. Error message contains `WEAK_PASSWORD`
Auth: none

### Scenario 21 — Logout revokes the active session
Depends on: Scenario 17 (has a valid sessionID)

Steps:
  1. `GRPC localhost:8081 AuthService.Logout` — request: `{session_id: "<sessionID from Scenario 17>"}`
  2. Assert: gRPC status OK. Response is empty
Auth: none

### Scenario 22 — ValidateSession returns valid=false for revoked session
Depends on: Scenario 21 (session from Scenario 17 was revoked)

Steps:
  1. `GRPC localhost:8081 AuthService.ValidateSession` — request: `{session_id: "<sessionID from Scenario 17>"}`
  2. Assert: gRPC status OK. Response has `valid` = `false`
Auth: none

### Scenario 23 — Logout with an already-revoked session fails
Depends on: Scenario 21

Steps:
  1. `GRPC localhost:8081 AuthService.Logout` — request: `{session_id: "<sessionID from Scenario 17>"}`
  2. Assert: gRPC status code = `FailedPrecondition`. Error message contains `SESSION_ALREADY_REVOKED`
Auth: none

### Scenario 24 — Login second user, then LogoutAll revokes all sessions
Depends on: Scenario 3

Steps:
  1. `GRPC localhost:8081 AuthService.Register` — request: `{email: "carol@example.com", name: "Carol", password: "CarolPass1!"}`
  2. Save: `carolUserID` = `response.user.id`
  3. `GRPC localhost:8081 AuthService.Login` — request: `{email: "carol@example.com", password: "CarolPass1!", user_agent: "", ip: ""}` — save `carolSessionID_A`
  4. `GRPC localhost:8081 AuthService.Login` — request: `{email: "carol@example.com", password: "CarolPass1!", user_agent: "", ip: ""}` — save `carolSessionID_B`
  5. `GRPC localhost:8081 AuthService.LogoutAll` — request: `{user_id: "<carolUserID>"}`
  6. Assert (step 5): gRPC status OK. Response is empty
  7. `GRPC localhost:8081 AuthService.ValidateSession` — request: `{session_id: "<carolSessionID_A>"}` — assert `valid` = `false`
  8. `GRPC localhost:8081 AuthService.ValidateSession` — request: `{session_id: "<carolSessionID_B>"}` — assert `valid` = `false`
  9. Assert (steps 7-8): both sessions are invalid
Auth: none

### Scenario 25 — LogoutAll with no active sessions succeeds as no-op
Depends on: none

Steps:
  1. `GRPC localhost:8081 AuthService.Register` — request: `{email: "dave@example.com", name: "Dave", password: "DavePass1!"}` — save `daveUserID`
  2. `GRPC localhost:8081 AuthService.LogoutAll` — request: `{user_id: "<daveUserID>"}`
  3. Assert (step 2): gRPC status OK. Response is empty
Auth: none

---

## Auth boundary scenarios

The Auth service does not expose JWT-protected HTTP endpoints. Its HTTP surface (/health, /.well-known/jwks.json) is intentionally public. The gRPC endpoints are internal service-to-service calls that do not require Bearer tokens; authorization is delegated to Core API's JWT middleware.

The closest applicable boundary tests involve invalid/expired session validation:

### Boundary 1 — ValidateSession with non-existent session_id
Depends on: none

Steps:
  1. `GRPC localhost:8081 AuthService.ValidateSession` — request: `{session_id: "00000000-0000-0000-0000-000000000000"}`
  2. Assert: gRPC status OK. Response has `valid` = `false`
Auth: none

### Boundary 2 — ValidateSession with malformed session_id
Depends on: none

Steps:
  1. `GRPC localhost:8081 AuthService.ValidateSession` — request: `{session_id: "not-a-uuid-at-all"}`
  2. Assert: gRPC status OK. Response has `valid` = `false`
Auth: none

### Boundary 3 — GetUser with malformed user_id
Depends on: none

Steps:
  1. `GRPC localhost:8081 AuthService.GetUser` — request: `{user_id: "not-a-uuid"}`
  2. Assert: gRPC status code = `InvalidArgument`. Error message contains `invalid user_id`
Auth: none

### Boundary 4 — Register with missing fields
Depends on: none

Steps:
  1. `GRPC localhost:8081 AuthService.Register` — request: `{email: "", name: "", password: ""}`
  2. Assert: gRPC status code is either `InvalidArgument` or the service accepts it and returns a weak-password error. The tester should verify that:
     - If the server validates empty email → error code `InvalidArgument`
     - If the server allows empty email → the use case returns `WEAK_PASSWORD`
Auth: none

---

## Known environment constraints

1. **Port 8084 for auth HTTP, not 8080.** The docker-compose maps host:8084 → container:8080. The spec says auth's HTTP port is 8080, but on the host you must use `localhost:8084`. The healthcheck inside the container correctly uses `localhost:8080`.

2. **Key files required at startup.** The compose mounts `./certs:/app/keys:ro` and expects `JWT_PRIVATE_KEY_PATH=/app/keys/private.pem`. If `./certs/private.pem` does not exist, the auth service will fail startup with a fatal error. The directory contains:
   - `certs/private.pem` (required — RSA 2048-bit private key)
   - `certs/public.pem` (optional — used for `JWT_PREVIOUS_PRIVATE_KEY_PATH`)

3. **Postgres DB name mismatch risk.** The docker-compose uses `POSTGRES_DB=gobox` and the DATABASE_URL points to `postgres://gobox:gobox@postgres:5432/gobox`. The `.env.example` uses `gobox_auth`. If you use a standalone `.env` file with `gobox_auth` as the DB name but the compose creates a `gobox` database, the service will fail to connect. Always rely on the compose file's DATABASE_URL.

4. **gRPC reflection is enabled.** The auth server registers `reflection.Register(grpcSrv)`, so `grpcurl` can be used for manual testing: `grpcurl -plaintext localhost:8081 list`.

5. **Port 5432 collision.** If any other Postgres instance (or the fileupload docker-compose postgres on port 5432) is running on the host, the auth postgres will fail to bind. Stop any other Postgres container or change the host port mapping before running the E2E suite.

6. **JWT token verification.** The access token returned by Register/Login is an RS256-signed JWT. The E2E suite can verify it locally using the public key from the JWKS endpoint. The JWT payload contains: `sub` (user UUID), `email`, `name`, `iat`, `exp` (= iat+900), `jti` (UUID), `sid` (session UUID). The `kid` in the JWT header must match one of the keys in the JWKS response.

7. **Refresh token rotation idempotency.** Once a refresh token is used (Scenario 9), it is permanently deleted from the database. Reusing it (Scenario 10) must return `TOKEN_THEFT_DETECTED`. The test suite must ensure it does not accidentally reuse a consumed token in a later scenario.

8. **No Docker Compose network isolation.** The auth docker-compose creates a `gobox` bridge network. If other services' compose files also declare a `gobox` network with `external: true`, they will conflict unless the network is created first with `docker network create gobox`. The auth compose declares it inline (not external), so it will create its own. This is fine for isolated testing.

---

## Compile check command

```bash
cd /home/ali/gobox/auth && go build ./...
```

This compiles the auth service binary and all packages, including any E2E test files under `e2e/`. All imports (including `gobox-proto`) must resolve.

---

## Run command

```bash
cd /home/ali/gobox/auth && docker compose up -d && go test ./e2e/ -v -count=1 -timeout 180s
```

Breakdown:
- `docker compose up -d` — starts postgres + auth in the background
- The test suite should include a startup health check loop (poll `/health` and gRPC `ValidateSession`/`GetPublicKey` up to 30s) before running scenarios
- `-count=1` disables test caching
- `-timeout 180s` gives the suite 3 minutes to complete (generous for 25+ sequential gRPC calls)

---

## Teardown order

| Order | Service | Compose file | Command |
|-------|---------|-------------|---------|
| 1 | auth | `auth/docker-compose.yml` | `docker compose -f auth/docker-compose.yml stop auth` |
| 2 | postgres | `auth/docker-compose.yml` | `docker compose -f auth/docker-compose.yml down` |

Full teardown:

```bash
cd /home/ali/gobox/auth && docker compose down -v
```

The `-v` flag removes the `pgdata` volume, ensuring a clean DB state for the next run. Omit `-v` if you want to preserve session data between test runs.

---

## Validation checklist

- [x] Every use case from GOBOX_SPEC.md §5.1 has a matching scenario: Register (3-5), Login (6-8), RefreshToken (9-11), Logout (21, 23), LogoutAll (24-25), GetUser (12-13), UpdateProfile (14-15), ChangePassword (16, 19-20)
- [x] Every port in scenario URLs appears in the port map: 8081 (gRPC), 8082 (HTTP)
- [x] Every "Depends on" reference points to a real scenario number
- [x] Known environment constraints section is not empty (8 items documented)

Tester Brief complete. Ready for Tester.
