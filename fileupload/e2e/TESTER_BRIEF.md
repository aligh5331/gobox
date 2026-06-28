# Tester Brief — FileUpload E2E Suite

## Services to spin up

Spin up in this exact order (network dependency). Both compose files share the
Docker network `gobox` — auth compose creates it, core compose attaches to it.

### 1. Auth compose — `/home/ali/gobox/auth/docker-compose.yml`

| Service  | HEALTHCHECK |
|----------|-------------|
| postgres | yes         |
| auth     | yes         |

### 2. Core compose — `/home/ali/gobox/core/docker-compose.yml`

| Service            | HEALTHCHECK |
|--------------------|-------------|
| minio              | yes         |
| fileupload-postgres| yes         |
| fileupload         | **no**      |
| core               | yes         |

`fileupload` has no HEALTHCHECK block — its `depends_on` uses
`condition: service_healthy` for Postgres and MinIO, so it will start once
those are green. The tester must poll the Core API `/health` endpoint as the
readiness signal for the entire stack.

---

## Port map

Only host-mapped ports are listed. Internal container-only ports are omitted.

| Service             | Host address         | Container port | Purpose                  |
|---------------------|----------------------|----------------|--------------------------|
| auth (HTTP)         | `localhost:8082`     | 8080           | JWKS + health            |
| auth (gRPC)         | `localhost:8081`     | 8081           | gRPC (not directly used) |
| auth-postgres       | `localhost:5432`     | 5432           | Auth database            |
| core (REST)         | **`localhost:3000`** | 8080           | **Primary test target**  |
| fileupload (gRPC)   | `localhost:9090`     | 9090           | Internal (not directly used) |
| fileupload-postgres | `localhost:5433`     | 5432           | FileUpload database      |
| minio (S3 API)      | **`localhost:9000`** | 9000           | **Presigned upload/download URLs** |
| minio (console)     | `localhost:9001`     | 9001           | Admin UI (not used)      |

The E2E suite hits exactly two host ports:
- **`localhost:3000`** — Core API (all REST requests)
- **`localhost:9000`** — MinIO S3 API (presigned PUT/GET from upload/download URLs)

---

## Scenarios

### Scenario 1 — Register two users and login both
**Depends on:** none

Steps:
  1. `POST http://localhost:3000/api/v1/auth/register` — body `{"email":"user1@test.dev","name":"User One","password":"TestPass123!"}`
  2. Assert: status 201, response contains `tokens.accessToken` (string, non-empty)
  3. Save: `token_user1` = `tokens.accessToken`
  4. `POST http://localhost:3000/api/v1/auth/register` — body `{"email":"user2@test.dev","name":"User Two","password":"TestPass123!"}`
  5. Assert: status 201, response contains `tokens.accessToken`
  6. Save: `token_user2` = `tokens.accessToken`

Auth: none (public endpoints)
Note: Use unique email per run or clean up after. Email uniqueness is enforced.

---

### Scenario 2 — InitiateUpload: happy path
**Depends on:** 1

Steps:
  1. `POST http://localhost:3000/api/v1/files` — body `{"name":"report.pdf","size":1048576,"mime_type":"application/pdf"}`
  2. Assert: status 202, response contains `fileId` (UUID-like string) and `uploadUrl` (string starting with `http`)
  3. Save: `file_id`, `upload_url`

Auth: `Bearer <token_user1>`

---

### Scenario 3 — InitiateUpload: reject empty filename
**Depends on:** 1

Steps:
  1. `POST http://localhost:3000/api/v1/files` — body `{"name":"","size":1048576,"mime_type":"application/pdf"}`
  2. Assert: status 400, error envelope `{"error":{"code":"INVALID_ARGUMENT",...}}` with message indicating filename is required

Auth: `Bearer <token_user1>`

---

### Scenario 4 — InitiateUpload: reject zero-size file
**Depends on:** 1

Steps:
  1. `POST http://localhost:3000/api/v1/files` — body `{"name":"empty.txt","size":0,"mime_type":"text/plain"}`
  2. Assert: status 400, error envelope `{"error":{"code":"INVALID_ARGUMENT",...}}` with message indicating size must be positive

Auth: `Bearer <token_user1>`

---

### Scenario 5 — Upload bytes to the presigned URL
**Depends on:** 2

Steps:
  1. `PUT <upload_url>` — body: 1 KiB of binary data (e.g. 1024 bytes of `0x41`), header `Content-Type: application/pdf`
  2. Assert: status 200

Auth: none (presigned URL carries its own auth)

---

### Scenario 6 — ConfirmUpload: happy path
**Depends on:** 5

Steps:
  1. `POST http://localhost:3000/api/v1/files/<file_id>/confirm` — no body
  2. Assert: status 200, response body contains `"status":"ready"`
  3. Save: (none needed)

Auth: `Bearer <token_user1>`

---

### Scenario 7 — ConfirmUpload: idempotent on already-ready file
**Depends on:** 6

Steps:
  1. `POST http://localhost:3000/api/v1/files/<file_id>/confirm` — no body (same file_id)
  2. Assert: status 200, response body contains `"status":"ready"`
  3. (No HEAD request to S3 is made by the server — this is an implementation guarantee, not observable at the REST layer)

Auth: `Bearer <token_user1>`

---

### Scenario 8 — ConfirmUpload: reject nonexistent file
**Depends on:** 1

Steps:
  1. `POST http://localhost:3000/api/v1/files/00000000-0000-0000-0000-000000000000/confirm` — no body
  2. Assert: status 404, error envelope with code `NOT_FOUND`

Auth: `Bearer <token_user1>`

---

### Scenario 9 — GetFile: retrieve metadata for own file
**Depends on:** 6

Steps:
  1. `GET http://localhost:3000/api/v1/files/<file_id>`
  2. Assert: status 200, response `file.name` == `"report.pdf"`, `file.size` == `1048576`, `file.status` == `"ready"`, `file.userId` is present

Auth: `Bearer <token_user1>`

---

### Scenario 10 — GetFile: nonexistent file returns 404
**Depends on:** 1

Steps:
  1. `GET http://localhost:3000/api/v1/files/00000000-0000-0000-0000-000000000000`
  2. Assert: status 404, error code `NOT_FOUND`

Auth: `Bearer <token_user1>`

---

### Scenario 11 — GetFile: different user cannot see file
**Depends on:** 6, 1

Steps:
  1. `GET http://localhost:3000/api/v1/files/<file_id>` (from Scenario 2, owned by user1)
  2. Assert: status 404, error code `NOT_FOUND`
  3. The error message must not reveal who owns the file

Auth: `Bearer <token_user2>`

---

### Scenario 12 — ListFiles: returns paginated results
**Depends on:** 1

Steps:
  1. (Seed 25 file records via InitiateUpload calls or use a bulk-insert helper — or just verify the contract with the file already created in Scenario 2)
  2. `GET http://localhost:3000/api/v1/files?pageSize=10`
  3. Assert: status 200, response contains exactly 10 file records, `nextPageToken` is not empty

Auth: `Bearer <token_user1>`
Note: If only 1 file exists (from Scenario 2), adjust assertion to 1 record with empty nextPageToken. The pagination contract will be fully tested in a dedicated integration test. The key assertion here is that pagination fields exist.

---

### Scenario 13 — ListFiles: empty list for user with no files
**Depends on:** 1

Steps:
  1. `GET http://localhost:3000/api/v1/files?pageSize=10`
  2. Assert: status 200, response contains exactly 0 file records, `nextPageToken` is empty or absent

Auth: `Bearer <token_user2>`

---

### Scenario 14 — DeleteFile: soft-delete own file
**Depends on:** 6

Steps:
  1. `DELETE http://localhost:3000/api/v1/files/<file_id>`
  2. Assert: status 200 or 204
  3. `GET http://localhost:3000/api/v1/files/<file_id>` — confirm it is now 404
  4. Assert: GET returns 404

Auth: `Bearer <token_user1>`

---

### Scenario 15 — DeleteFile: different user cannot delete
**Depends on:** 2 (a fresh upload owned by user1), 1

Steps:
  1. Create a new file record (repeat Scenario 2 to get `file_id2` owned by user1)
  2. `DELETE http://localhost:3000/api/v1/files/<file_id2>`
  3. Assert: status 404, error code `NOT_FOUND`
  3. Error message must not reveal the owner's identity

Auth: `Bearer <token_user2>`

---

### Scenario 16 — GetDownloadURL: presigned GET URL for ready file
**Depends on:** 6 (or use the file from Scenario 2 which is now ready)

Steps:
  1. `GET http://localhost:3000/api/v1/files/<file_id>/download`
  2. Assert: status 200, response contains `url` (string starting with `http`)
  3. Save: `download_url`
  4. `HEAD <download_url>` — verify the presigned URL is valid
  5. Assert: HEAD returns status 200

Auth: `Bearer <token_user1>`

---

### Scenario 17 — GetDownloadURL: pending file returns FailedPrecondition
**Depends on:** 1

Steps:
  1. Initiate a NEW upload (repeat Scenario 2 steps, get `file_id_pending`) — do NOT upload bytes or confirm
  2. `GET http://localhost:3000/api/v1/files/<file_id_pending>/download`
  3. Assert: status 400 or 409, error with code indicating the file is not yet ready (e.g. `FAILED_PRECONDITION`)

Auth: `Bearer <token_user1>`

---

### Scenario 18 — GetDownloadURL: soft-deleted file returns 404
**Depends on:** 16, 14 (use the file whose download URL was just verified, then delete)

Steps:
  1. After Scenario 14 (file is soft-deleted), or create a temp file, confirm it, then delete it
  2. `GET http://localhost:3000/api/v1/files/<deleted_file_id>/download`
  3. Assert: status 404, error code `NOT_FOUND`

Auth: `Bearer <token_user1>`

---

## Auth boundary scenarios

### Scenario 19 — No token
**Depends on:** none

Steps:
  1. `GET http://localhost:3000/api/v1/files` — no `Authorization` header
  2. Assert: status 401, response body matches `{"error":{"code":"UNAUTHORIZED",...}}`

---

### Scenario 20 — Malformed token
**Depends on:** none

Steps:
  1. `GET http://localhost:3000/api/v1/files` — header `Authorization: Bearer obviouslyinvalid`
  2. Assert: status 401, response body matches `{"error":{"code":"UNAUTHORIZED",...}}`

---

### Scenario 21 — Empty token
**Depends on:** none

Steps:
  1. `GET http://localhost:3000/api/v1/files` — header `Authorization: Bearer ` (empty after `Bearer `)
  2. Assert: status 401, response body matches `{"error":{"code":"UNAUTHORIZED",...}}`

---

## Known environment constraints

1. **MinIO presigned URL hostname mismatch.** The FileUpload service uses
   `S3_ENDPOINT=minio:9000` (internal Docker hostname) to construct presigned
   URLs. The E2E test runs on the **host** where `minio:9000` does not resolve.
   **The tester MUST set `S3_PUBLIC_ENDPOINT=http://localhost:9000`** on the
   `fileupload` service in `core/docker-compose.yml` (or via a `docker-compose.override.yml`)
   **before** spinning up. Without this, presigned URLs will contain `minio:9000`
   and the PUT/HEAD requests in Scenarios 5 and 16 will fail with DNS errors.

2. **Auth compose creates the shared `gobox` network.** The core compose declares
   the network as `external: true`. Startup order: Auth compose **must** be
   started first so the network exists. Failure to do so produces Docker network
   errors.

3. **No HEALTHCHECK on `fileupload`.** The service starts silently once its
   dependencies (Postgres, MinIO) are healthy. Wait for Core API's `/health`
   (`GET http://localhost:3000/health`) to return `{"status":"ok"}` before
   running tests. Poll with a reasonable timeout (~60s, 2s intervals).

4. **Postgres port collision.** Auth compose maps Postgres to `localhost:5432`.
   The fileupload compose (standalone) also maps Postgres to `localhost:5432`.
   **Do not use `fileupload/docker-compose.yml` for E2E** — use
   `core/docker-compose.yml` which maps `fileupload-postgres` to
   `localhost:5433` (no conflict).

5. **Email uniqueness.** The Register endpoint enforces unique email. Use a
   unique email per test run (e.g. append timestamp: `tester-1719500000@test.dev`)
   or delete the user between runs. The recommended approach: generate unique
   emails at runtime.

6. **Auth service Postgres DB name is `gobox`, not `fileupload`.** The auth
   compose creates a database named `gobox` for the auth service. The
   fileupload compose creates a database named `fileupload` for the fileupload
   service. They are independent and must not be confused.

7. **Token TTL is 15 minutes.** Access tokens from Auth expire after 15 minutes.
   If the full scenario chain takes longer, re-login or refresh the token.
   For the expired-token auth boundary test, either wait ~15 min or craft an
   expired JWT manually (requires knowing the public key — not practical for
   automated E2E). Recommendation: defer expired-token testing to a unit test
   in the JWT middleware package; cover no-token and malformed-token only in E2E.

---

## Compile check command

```bash
cd /home/ali/gobox/fileupload && go test -tags e2e -run ^$ ./e2e/ -count=0
```

This compiles the `e2e` package without running any tests. Exit code 0 means
the suite is buildable.

---

## Run command

```bash
cd /home/ali/gobox/fileupload && go test -tags e2e -v -count=1 ./e2e/
```

Flags:
- `-tags e2e` — includes the build-constrained e2e test files
- `-v` — verbose output (log each scenario start/end)
- `-count=1` — disables test caching

---

## Teardown order

Reverse startup order (dependencies before dependents):

1. `docker compose -f /home/ali/gobox/core/docker-compose.yml down`
2. `docker compose -f /home/ali/gobox/auth/docker-compose.yml down`

If volumes should be cleaned too (to avoid email-uniqueness issues on re-runs):

```bash
docker compose -f /home/ali/gobox/core/docker-compose.yml down -v
docker compose -f /home/ali/gobox/auth/docker-compose.yml down -v
docker network rm gobox
```

---

## Validation checklist

- [x] Every use case from `features/fileupload.feature` has a matching scenario:
      InitiateUpload (Sc. 2-4), ConfirmUpload (Sc. 6-8), GetFile (Sc. 9-11),
      ListFiles (Sc. 12-13), DeleteFile (Sc. 14-15), GetDownloadURL (Sc. 16-18)
- [x] Every port in scenario URLs appears in the port map: `3000` (Core API),
      `9000` (MinIO)
- [x] Every "Depends on" reference points to a real scenario number
- [x] Known environment constraints section covers MinIO hostname mismatch,
      network ordering, HEALTHCHECK gap, port collision, email uniqueness,
      DB name discrepancy, and token TTL limitation

Tester Brief complete. Ready for Tester.
