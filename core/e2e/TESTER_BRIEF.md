# Tester Brief — Core API (Phase 2 — Auth & User Profile)

> **Phase:** 2 — Core API Auth Endpoints  
> **Spec:** `GOBOX_SPEC.md §5.2`, `features/core_auth.feature`  
> **Module:** `github.com/aligh5331/gobox/core`  
> **E2E suite location:** `core/e2e/`

---

## Services to spin up

The Core API is a stateless gateway that proxies auth operations to the Auth
service via gRPC. Both services must be running.

The Core API compose (`core/docker-compose.yml`) also defines `fileupload`,
`fileupload-postgres`, and `minio` services — these are **not needed** for
Phase 2 tests. Start only the `core` service from that file.

### Startup order (dependencies first)

| Step | Service | Directory | Compose file | HEALTHCHECK | Notes |
|------|---------|-----------|-------------|-------------|-------|
| 1 | auth-postgres | `auth/` | `auth/docker-compose.yml` | yes (`pg_isready -U gobox`) | Creates the `gobox` network |
| 2 | auth | `auth/` | `auth/docker-compose.yml` | yes (`wget -qO- http://localhost:8080/health`) | Depends on postgres |
| 3 | core | `core/` | `core/docker-compose.yml` | yes (`wget -qO- http://localhost:8080/health`) | Start **only** the `core` service |

### Startup commands

```bash
# Step 1: Create network + start auth Postgres and auth service
docker compose -f /home/ali/gobox/auth/docker-compose.yml up -d

# Wait for auth healthcheck
until curl -sf http://localhost:8082/health > /dev/null 2>&1; do
  echo "Waiting for auth..."; sleep 2;
done

# Step 2: Start only the core service (skip fileupload/minio deps)
docker compose -f /home/ali/gobox/core/docker-compose.yml up -d core

# Wait for core healthcheck
until curl -sf http://localhost:3000/health > /dev/null 2>&1; do
  echo "Waiting for core..."; sleep 2;
done
```

**Important:** The auth compose file creates the `gobox` Docker bridge network.
The core compose declares it as `external: true`, so auth compose **must** be
started first.

---

## Port map

Only host-mapped ports are listed. Internal container-only ports are omitted.

| Service | Host address | Container port | Purpose |
|---------|-------------|----------------|---------|
| auth-postgres | `localhost:5432` | 5432 | Auth database (for direct session expiry if needed) |
| auth (gRPC) | `localhost:8081` | 8081 | AuthService RPCs (not called directly by E2E) |
| auth (HTTP) | `localhost:8082` | 8080 | Health + JWKS endpoint |
| **core (REST)** | **`localhost:3000`** | **8080** | **Primary test target — all scenarios** |

The E2E suite hits exactly two host ports:
- **`localhost:3000`** — Core API REST (all scenarios)
- **`localhost:8082`** — Auth HTTP JWKS (needed for expired-JWT crafting in Scenario 11)

---

## Scenarios

### Data flow / state variables

The test suite should maintain the following Go variables across scenarios:

| Variable | Source | Used by |
|----------|--------|---------|
| `userID` | Scenario 2 (Register) | Scenarios 4, 9, 10, 12-16 |
| `accessToken` | Scenario 4 (Login) | Scenarios 6-8, 10-16 |
| `refreshToken` | Scenario 4 (Login) | Scenario 6 |
| `sessionID` | Scenario 4 (Login) | Scenarios 7, 8 |
| `craftedExpiredToken` | Scenario 11 (crafted JWT) | Scenario 11 |

---

### Scenario 1 — Core API health endpoint
Depends on: none

Steps:
  1. `GET http://localhost:3000/health`
  2. Assert: HTTP 200, response body is `{"status":"ok"}`
Auth: none

---

### Scenario 2 — Register a new user successfully
Depends on: none

Steps:
  1. `POST http://localhost:3000/api/v1/auth/register` — JSON body:
     ```json
     {
       "email": "ali@example.com",
       "password": "correctPass1!",
       "name": "Ali"
     }
     ```
  2. Assert: HTTP 201. Response JSON has a `"user"` object with:
     - `"email"` = `"ali@example.com"`
     - `"name"` = `"Ali"`
     - `"id"` is a non-empty string (valid UUID format)
     - `"created_at"` is a non-empty string
     - `"updated_at"` is a non-empty string
  3. Assert: Response JSON has a `"tokens"` object with:
     - `"access_token"` is a non-empty string (3 dot-separated base64url segments)
     - `"refresh_token"` is a non-empty string
     - `"expires_in"` is a positive integer
  4. Assert: Response JSON has a `"session"` object with:
     - `"id"` is a valid UUID string
     - `"user_id"` is a valid UUID string matching `"user.id"`
  5. Save:
     - `userID` = `response.user.id`
     - `accessToken` = `response.tokens.access_token`
     - `refreshToken` = `response.tokens.refresh_token`
     - `sessionID` = `response.session.id`
Auth: none (register is public)

---

### Scenario 3 — Register with an already-registered email
Depends on: Scenario 2 (`ali@example.com` exists)

Steps:
  1. `POST http://localhost:3000/api/v1/auth/register` — JSON body:
     ```json
     {
       "email": "ali@example.com",
       "password": "SecurePass123!",
       "name": "Ali Again"
     }
     ```
  2. Assert: HTTP 409. Response JSON has error envelope with:
     - `"error.code"` = `"CONFLICT"`
     - `"error.message"` is a non-empty string
Auth: none

---

### Scenario 4 — Login with valid credentials
Depends on: Scenario 2 (`ali@example.com` exists)

Steps:
  1. `POST http://localhost:3000/api/v1/auth/login` — JSON body:
     ```json
     {
       "email": "ali@example.com",
       "password": "correctPass1!"
     }
     ```
  2. Assert: HTTP 200. Response JSON has a `"user"` object with:
     - `"id"` matches `userID` from Scenario 2
     - `"email"` = `"ali@example.com"`
     - `"name"` = `"Ali"`
  3. Assert: Response JSON has a `"tokens"` object with:
     - `"access_token"` is a non-empty JWT string
     - `"refresh_token"` is a non-empty string
     - `"expires_in"` is a positive integer
  4. Assert: Response JSON has a `"session"` object with:
     - `"id"` is a valid UUID string
     - `"user_id"` matches `userID` from Scenario 2
  5. Save:
     - `accessToken` = `response.tokens.access_token`
     - `refreshToken` = `response.tokens.refresh_token`
     - `sessionID` = `response.session.id`
Auth: none (login is public)

---

### Scenario 5 — Login with wrong password
Depends on: Scenario 2

Steps:
  1. `POST http://localhost:3000/api/v1/auth/login` — JSON body:
     ```json
     {
       "email": "ali@example.com",
       "password": "wrongPassword99!"
     }
     ```
  2. Assert: HTTP 401. Response JSON has error envelope with:
     - `"error.code"` = `"UNAUTHORIZED"`
     - `"error.message"` is a non-empty string
Auth: none

---

### Scenario 6 — Refresh tokens with a valid refresh token
Depends on: Scenario 4 (has a valid `refreshToken`)

Steps:
  1. `POST http://localhost:3000/api/v1/auth/refresh` — JSON body:
     ```json
     {
       "refresh_token": "<refreshToken from Scenario 4>"
     }
     ```
  2. Assert: HTTP 200. Response JSON has a `"tokens"` object with:
     - `"access_token"` is a non-empty JWT string (different from the old `accessToken`)
     - `"refresh_token"` is a non-empty string (different from the old `refreshToken`)
     - `"expires_in"` is a positive integer
  3. Save:
     - `newAccessToken` = `response.tokens.access_token`
     - `newRefreshToken` = `response.tokens.refresh_token`
Auth: none (refresh accepts the opaque refresh token in body, not a JWT header)

---

### Scenario 7 — Refresh with an expired/stale refresh token
Depends on: Scenario 4 (has a valid `sessionID`)

Because refresh tokens live 30 days, the test cannot wait for natural expiry.
The test must either:
- **Option A (DB manipulation):** Connect to auth-postgres (`localhost:5432`),
  update the session's `expires_at` to the past, then attempt refresh with
  the session's refresh token.
- **Option B (simplified):** Use a non-existent or malformed refresh token and
  assert 401. The error code will be `UNAUTHORIZED` in either case (the REST
  layer does not distinguish expired from invalid).

Option B is recommended for simplicity:

Steps:
  1. `POST http://localhost:3000/api/v1/auth/refresh` — JSON body:
     ```json
     {
       "refresh_token": "expired-refresh-token-xyz"
     }
     ```
  2. Assert: HTTP 401. Response JSON has error envelope with:
     - `"error.code"` = `"UNAUTHORIZED"`
     - `"error.message"` is a non-empty string
Auth: none

---

### Scenario 8 — Logout with a valid session
Depends on: Scenario 4 (has a valid `accessToken` and `sessionID`)

Steps:
  1. `DELETE http://localhost:3000/api/v1/auth/logout` — JSON body:
     ```json
     {
       "session_id": "<sessionID from Scenario 4>"
     }
     ```
    Header: `Authorization: Bearer <accessToken from Scenario 4>`
  2. Assert: HTTP 204. Response body is empty.
Auth: `Bearer <accessToken>`

---

### Scenario 9 — Logout without an Authorization header (missing token)
Depends on: none

Steps:
  1. `DELETE http://localhost:3000/api/v1/auth/logout` — JSON body:
     ```json
     {
       "session_id": "a47ac10b-58cc-4372-a567-0e02b2c3d479"
     }
     ```
    No `Authorization` header.
  2. Assert: HTTP 401. Response JSON has error envelope with:
     - `"error.code"` = `"UNAUTHORIZED"`
     - `"error.message"` is a non-empty string
Auth: none

---

### Scenario 10 — Get own profile with a valid token
Depends on: Scenario 4 (has a valid `accessToken`)

Steps:
  1. `GET http://localhost:3000/api/v1/me`
     Header: `Authorization: Bearer <accessToken from Scenario 4>`
  2. Assert: HTTP 200. Response JSON has:
     - `"id"` matches `userID` from Scenario 2
     - `"email"` = `"ali@example.com"`
     - `"name"` = `"Ali"`
     - `"created_at"` is a non-empty string
     - `"updated_at"` is a non-empty string
Auth: `Bearer <accessToken>`

---

### Scenario 11 — Get own profile with an expired token
Depends on: Scenario 2 (user exists), Scenario 4 (for JWKS context)

The Core API validates JWT tokens locally using RS256 and the JWKS fetched from
auth at startup. To craft an expired JWT, the test must:

1. Fetch the JWKS from `http://localhost:8082/.well-known/jwks.json`
2. Read the RSA private key from `auth/keys/private.pem` (mounted in the auth
   container, also available from the host at the repo path)
3. Use `github.com/golang-jwt/jwt/v5` with RS256 to sign a JWT with:
   - `sub`: `<userID from Scenario 2>`
   - `email`: `"ali@example.com"`
   - `name`: `"Ali"`
   - `iat`: `now - 1 hour`
   - `exp`: `now - 1 minute` (in the past)
   - `jti`: a fresh UUID
   - `sid`: a fresh UUID

Steps:
  1. Craft an expired JWT as described above. Save it as `craftedExpiredToken`.
  2. `GET http://localhost:3000/api/v1/me`
     Header: `Authorization: Bearer <craftedExpiredToken>`
  3. Assert: HTTP 401. Response JSON has error envelope with:
     - `"error.code"` = `"UNAUTHORIZED"`
     - `"error.message"` is a non-empty string
Auth: `Bearer <craftedExpiredToken>`

**Note:** If the private key is not accessible to the test process, this
scenario can be approximated by using a token signed with a different key.
The Core API will reject it as invalid-signature (also 401), but this does
not test the expiry check. Consider adding a targeted unit test for
expiry-vs-signature distinction in the JWT middleware package. The E2E
test at minimum validates that an invalid token results in 401.

---

### Scenario 12 — Update own profile with valid fields
Depends on: Scenario 4 (has a valid `accessToken`)

Steps:
  1. `PUT http://localhost:3000/api/v1/me` — JSON body:
     ```json
     {
       "name": "Ali Reza"
     }
     ```
    Header: `Authorization: Bearer <accessToken from Scenario 4>`
  2. Assert: HTTP 200. Response JSON has:
     - `"id"` matches `userID` from Scenario 2
     - `"email"` = `"ali@example.com"`
     - `"name"` = `"Ali Reza"`
     - `"updated_at"` is a non-empty string
Auth: `Bearer <accessToken>`

---

### Scenario 13 — Update own profile with empty name
Depends on: Scenario 4 (has a valid `accessToken`)

Steps:
  1. `PUT http://localhost:3000/api/v1/me` — JSON body:
     ```json
     {
       "name": ""
     }
     ```
    Header: `Authorization: Bearer <accessToken from Scenario 4>`
  2. Assert: HTTP 400. Response JSON has error envelope with:
     - `"error.code"` = `"BAD_REQUEST"`
     - `"error.message"` is a non-empty string
Auth: `Bearer <accessToken>`

---

### Scenario 14 — Update own profile with missing name field
Depends on: Scenario 4 (has a valid `accessToken`)

Steps:
  1. `PUT http://localhost:3000/api/v1/me` — JSON body:
     ```json
     {}
     ```
    Header: `Authorization: Bearer <accessToken from Scenario 4>`
  2. Assert: HTTP 400. Response JSON has error envelope with:
     - `"error.code"` = `"BAD_REQUEST"`
     - `"error.message"` is a non-empty string
Auth: `Bearer <accessToken>`

---

### Scenario 15 — Change password with correct old password
Depends on: Scenario 4 (has a valid `accessToken`)

Steps:
  1. `PUT http://localhost:3000/api/v1/me/password` — JSON body:
     ```json
     {
       "old_password": "correctPass1!",
       "new_password": "newSecurePass99!"
     }
     ```
    Header: `Authorization: Bearer <accessToken from Scenario 4>`
  2. Assert: HTTP 204. Response body is empty.
Auth: `Bearer <accessToken>`

**Note:** This changes the user's password. Any subsequent scenarios that
require login with `correctPass1!` will fail (use Scenarios 4-6 which ran
before this one, or re-login with `newSecurePass99!`).

---

### Scenario 16 — Change password with wrong old password
Depends on: Scenario 4 (has a valid `accessToken`)

Steps:
  1. `PUT http://localhost:3000/api/v1/me/password` — JSON body:
     ```json
     {
       "old_password": "wrongOldPass!",
       "new_password": "newSecurePass99!"
     }
     ```
    Header: `Authorization: Bearer <accessToken from Scenario 4>`
  2. Assert: HTTP 403. Response JSON has error envelope with:
     - `"error.code"` = `"FORBIDDEN"`
     - `"error.message"` is a non-empty string
Auth: `Bearer <accessToken>`

---

## Auth boundary scenarios

### Boundary 1 — No token on authenticated endpoint
Depends on: none

Steps:
  1. `GET http://localhost:3000/api/v1/me` — no `Authorization` header
  2. Assert: HTTP 401. Response JSON has error envelope with:
     - `"error.code"` = `"UNAUTHORIZED"`
     - `"error.message"` is a non-empty string
Auth: none

### Boundary 2 — Malformed token on authenticated endpoint
Depends on: none

Steps:
  1. `GET http://localhost:3000/api/v1/me` — header `Authorization: Bearer obviouslyinvalid`
  2. Assert: HTTP 401. Response JSON has error envelope with:
     - `"error.code"` = `"UNAUTHORIZED"`
     - `"error.message"` is a non-empty string
Auth: `Bearer obviouslyinvalid`

### Boundary 3 — Empty token on authenticated endpoint
Depends on: none

Steps:
  1. `GET http://localhost:3000/api/v1/me` — header `Authorization: Bearer ` (empty after `Bearer `)
  2. Assert: HTTP 401. Response JSON has error envelope with:
     - `"error.code"` = `"UNAUTHORIZED"`
     - `"error.message"` is a non-empty string
Auth: `Bearer ` (empty token)

---

## Known environment constraints

1. **Auth compose must start first.** The auth compose creates the `gobox`
   Docker network (`docker-compose.yml` declares it inline with `driver: bridge`).
   The core compose declares the network as `external: true`. If auth compose
   is not up first, the core service will fail to attach to the network.

2. **Only start `core` from core's compose.** `core/docker-compose.yml` also
   defines `fileupload`, `fileupload-postgres`, and `minio`. These are not
   needed for Phase 2 tests. Starting them may fail if the FileUpload service
   has not been built yet (Phase 3). Use:
   ```bash
   docker compose -f core/docker-compose.yml up -d core
   ```

3. **Port 3000 for core REST, not 8080.** The compose maps host:3000 →
   container:8080. The spec says core's HTTP port is 8080, but on the host
   the E2E suite must use `localhost:3000`. The healthcheck inside the
   container correctly uses `localhost:8080`.

4. **Port 8082 for auth HTTP, not 8080.** The auth compose maps host:8082 →
   container:8080. The JWKS endpoint on the host is at
   `http://localhost:8082/.well-known/jwks.json`.

5. **Port 5432 collision.** The auth Postgres binds to `localhost:5432`. If
   any other Postgres instance (including a local system Postgres) is running
   on port 5432, the container will fail to bind. Stop other Postgres
   processes or change the host port mapping.

6. **Access token TTL is 15 minutes.** Tokens from login expire after 15
   minutes. If the scenario chain takes longer, the test must re-login or
   refresh the token. The crafted expired token (Scenario 11) bypasses this
   by constructing a token with an explicit past `exp` claim using the auth
   private key from `auth/keys/private.pem`.

7. **Auth private key must exist.** The auth service mounts `./keys:/app/keys:ro`
   and expects `JWT_PRIVATE_KEY_PATH=/app/keys/private.pem`. This file is
   required for:
   - Auth service startup
   - Crafting the expired JWT in Scenario 11 (the E2E suite reads it from
     the host path `auth/keys/private.pem`)
   Generate it if missing: `openssl genrsa -out auth/keys/private.pem 2048`

8. **Email uniqueness is enforced.** The Register endpoint returns 409 if the
   email already exists (Scenario 3). The test suite uses a fixed email
   `ali@example.com`. If previous test runs or manual testing have created
   this email, the test must either:
   - Drop the DB volume before each run
   - Use a unique email with a timestamp suffix
   The recommended approach: use a fresh DB volume (`docker compose down -v`)
   between runs.

9. **Refresh token rotation.** After a refresh token is used (Scenario 6), it
   is rotated. Reusing the old refresh token will return 401. The test suite
   must ensure that Scenario 6 uses the token from Scenario 4, and any
   subsequent refresh attempts use the latest token.

10. **Expired access token test requires the private key.** Scenario 11
    constructs an RS256-signed JWT with a past `exp` claim. This requires
    `github.com/golang-jwt/jwt/v5` (already in `go.mod`) and access to
    `auth/keys/private.pem`. If the private key is not accessible, the
    expired-token scenario must be deferred to a unit test. The no-token
    (Boundary 1) and malformed-token (Boundary 2) scenarios are sufficient
    to validate the middleware rejects invalid tokens in E2E.

11. **No redis or additional services required.** For Phase 2, only Postgres
    (from auth) and the two Go services are needed.

---

## Compile check command

```bash
cd /home/ali/gobox/core && go test -run ^$ ./e2e/ -count=0
```

This compiles the `e2e` package without running any tests. Exit code 0 means
the suite is buildable. All imports (including `gobox-proto`) must resolve.

---

## Run command

```bash
# 1. Spin up services
docker compose -f /home/ali/gobox/auth/docker-compose.yml up -d
docker compose -f /home/ali/gobox/core/docker-compose.yml up -d core

# 2. Wait for readiness (poll both health endpoints)
echo "Waiting for services..."
for i in $(seq 1 30); do
  if curl -sf http://localhost:8082/health > /dev/null 2>&1 && \
     curl -sf http://localhost:3000/health > /dev/null 2>&1; then
    echo "Services ready."
    break
  fi
  sleep 2
done

# 3. Run the test suite
cd /home/ali/gobox/core && go test ./e2e/ -v -count=1 -timeout 120s
```

Flags:
- `-v` — verbose output (log each scenario start/end)
- `-count=1` — disables test caching
- `-timeout 120s` — generous limit for 16+ sequential HTTP requests

---

## Teardown order

Reverse startup order (dependents before dependencies):

| Order | Service | Compose file | Command |
|-------|---------|-------------|---------|
| 1 | core | `core/docker-compose.yml` | `docker compose -f core/docker-compose.yml stop core` |
| 2 | auth | `auth/docker-compose.yml` | `docker compose -f auth/docker-compose.yml stop auth` |
| 3 | auth-postgres | `auth/docker-compose.yml` | `docker compose -f auth/docker-compose.yml down` |

Full teardown (cleans volumes for a fresh state):

```bash
docker compose -f /home/ali/gobox/core/docker-compose.yml down
docker compose -f /home/ali/gobox/auth/docker-compose.yml down -v
docker network rm gobox 2>/dev/null || true
```

The `-v` flag on auth compose removes the `pgdata` volume, ensuring a clean
DB state for the next run. Omit `-v` if you want to preserve session data
between test runs.

---

## Validation checklist

- [x] Every use case from `features/core_auth.feature` has a matching scenario:
      Register (Sc. 2-3), Login (Sc. 4-5), RefreshToken (Sc. 6-7), Logout (Sc. 8-9),
      GetUser profile (Sc. 10-11), UpdateProfile (Sc. 12-14), ChangePassword (Sc. 15-16)
- [x] Every port in scenario URLs appears in the port map: `3000` (Core API REST),
      `8082` (Auth HTTP / JWKS)
- [x] Every "Depends on" reference points to a real scenario number
- [x] Known environment constraints section is not empty (11 items documented)

Tester Brief complete. Ready for Tester.
