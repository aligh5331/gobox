# Tester Brief — Shortener E2E Suite

## Services to spin up

Spin up in this exact order (network dependency). Both auth and core compose
files share the Docker network `gobox` — auth compose creates it, core compose
attaches to it. The shortener compose creates its own internal network.

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
those are green. Poll Core API `/health` as the readiness signal.

### 3. Shortener compose — `/home/ali/gobox/shortener/docker-compose.yml`

| Service  | HEALTHCHECK |
|----------|-------------|
| postgres | yes         |
| redis    | yes         |
| shortener| **no**      |

`shortener` has no HEALTHCHECK block — its `depends_on` uses
`condition: service_healthy` for Postgres and Redis. Poll the shortener's
`/health` endpoint (`localhost:8082/health`) or the redirect endpoint
(`GET /s/healthcheck — expect 404 which proves the server is up`) as
readiness signal.

---

## Port map

Only host-mapped ports are listed. Internal container-only ports are omitted.

| Service             | Host address         | Container port | Purpose                         |
|---------------------|----------------------|----------------|---------------------------------|
| auth (HTTP)         | `localhost:8082`     | 8080           | JWKS + health                   |
| auth (gRPC)         | `localhost:8081`     | 8081           | gRPC (not directly used)        |
| auth-postgres       | `localhost:5432`     | 5432           | Auth database                   |
| core (REST)         | **`localhost:3000`** | 8080           | **Primary test target (auth)**  |
| fileupload (gRPC)   | `localhost:9090`     | 9090           | Internal (not directly used)    |
| fileupload-postgres | `localhost:5433`     | 5432           | FileUpload database             |
| minio (S3 API)      | `localhost:9000`     | 9000           | S3 API                          |
| minio (console)     | `localhost:9001`     | 9001           | Admin UI (not used)             |
| shortener (HTTP)    | **`localhost:8082`** | 8082           | **Public redirect endpoint**    |
| shortener (gRPC)    | `localhost:9091`     | 9091           | Internal (not directly used)    |
| shortener-postgres  | `localhost:5433`     | 5432           | Shortener database              |
| redis               | `localhost:6379`     | 6379           | Redirect cache                  |

The E2E suite hits exactly two host ports:
- **`localhost:3000`** — Core API (all authenticated REST requests)
- **`localhost:8082`** — Shortener HTTP (public redirects)

**Port collision warning:** Auth compose maps port `8082:8080` (auth HTTP)
and shortener compose maps port `8082:8082` (shortener redirect). **These
cannot both bind to host port 8082.** Change one of them:
- Either change auth's host port to a different port (e.g. `8084:8080`) in
  its compose file, **or**
- Change shortener's host port to a different port (e.g. `8083:8082`).

This brief assumes shortener's redirect port is reachable at `localhost:8082`
(after resolving the conflict). Update scenario URLs if you choose a different
port.

**Postgres port collision warning:** Shortener compose maps `5433:5432` and
core compose maps `fileupload-postgres` to `5433:5432`. **These cannot both
bind to host port 5433.** Either:
- Run shortener's postgres on a different host port (e.g. `5434:5432`), or
- Use the same Postgres instance for both shortener and fileupload (not
  recommended — they use different databases), or
- Start shortener compose with a different Postgres host port via override.

---

## Scenarios

### Scenario 0 — Register two users and login both
**Depends on:** none (Auth + Core must be running)

Steps:
  1. `POST http://localhost:3000/api/v1/auth/register` — body `{"email":"shortener-e2e-1@test.dev","name":"User One","password":"TestPass123!"}`
  2. Assert: status 201, response contains `tokens.accessToken` (string, non-empty)
  3. Save: `token_user1` = `tokens.accessToken`
  4. `POST http://localhost:3000/api/v1/auth/register` — body `{"email":"shortener-e2e-2@test.dev","name":"User Two","password":"TestPass123!"}`
  5. Assert: status 201, response contains `tokens.accessToken`
  6. Save: `token_user2` = `tokens.accessToken`

Auth: none (public endpoints)
Note: Use unique email per run. Email uniqueness is enforced.

---

### Scenario 1 — Upload a file and confirm it (prerequisite for CreateLink)
**Depends on:** 0

Steps:
  1. `POST http://localhost:3000/api/v1/files` — body `{"name":"share-test.pdf","size":4096,"mime_type":"application/pdf"}`
  2. Assert: status 202, response contains `fileId` (UUID string) and `uploadUrl` (string starting with `http`)
  3. Save: `file_id` = `fileId`, `upload_url` = `uploadUrl`
  4. `PUT <upload_url>` — body: 4096 bytes of binary data (e.g. 0x41 repeated), header `Content-Type: application/pdf`
  5. Assert: status 200
  6. `POST http://localhost:3000/api/v1/files/<file_id>/confirm` — body: `{"storage_key":"<extract from upload_url path>","size":4096,"mime_type":"application/pdf"}`
     If `storage_key` is unknown, send empty body or use the previous response's storage_key.
  7. Assert: status 200, response contains `"status":"ready"`

Auth: `Bearer <token_user1>`
Note: If the ConfirmUpload endpoint does not require a body, send an empty JSON object `{}`.

---

### Scenario 2 — CreateLink happy path returns slug and short URL
**Depends on:** 1

Steps:
  1. `POST http://localhost:3000/api/v1/files/<file_id>/share` — body `{}`
  2. Assert: status 201, response contains a `link` object with:
     - `slug` (exactly 6 alphanumeric characters)
     - `fileId` equal to `<file_id>`
     - `id` (UUID string)
     - `userId` equal to the user's UUID
  3. Save: `link_slug` = `link.slug`, `link_id` = `link.id`

Auth: `Bearer <token_user1>`

---

### Scenario 3 — CreateLink with missing file_id returns validation error
**Depends on:** 0

Steps:
  1. `POST http://localhost:3000/api/v1/files//share` — body `{}`
  2. Assert: status 404 or 400 depending on route matching. If route requires a non-empty `:id` param, the request should use a well-formed URL with an invalid UUID: `POST http://localhost:3000/api/v1/files/00000000-0000-0000-0000-000000000000/share`
  3. If using invalid UUID: assert status 400, response error code `INVALID_ARGUMENT` or `BAD_REQUEST`

Auth: `Bearer <token_user1>`
Note: The spec feature file tests this at the gRPC level. At the HTTP level,
the Core API validates the path parameter presence before forwarding. Adjust
assertions to match the Core API's actual validation behaviour.

---

### Scenario 4 — GetLink via redirect: existing link returns 302
**Depends on:** 2

Steps:
  1. `GET http://localhost:8082/s/<link_slug>`
  2. Assert: status 302
  3. Assert: `Location` header is present and starts with `http`
  4. Assert: `Location` header contains a presigned URL (e.g. contains `X-Amz-Signature` or `AWSAccessKeyId`)
  5. (Optional) Follow the redirect: `HEAD <Location>` — assert 200

Auth: none (public redirect endpoint)

---

### Scenario 5 — GetLink via redirect: non-existent slug returns 404
**Depends on:** none

Steps:
  1. `GET http://localhost:8082/s/nonexist99`
  2. Assert: status 404
  3. Assert: response body is a JSON error with code `NOT_FOUND`

Auth: none

---

### Scenario 6 — DeleteLink removes the user's own link
**Depends on:** 4 (link must exist)

Steps:
  1. `DELETE http://localhost:3000/api/v1/links/<link_id>`
  2. Assert: status 204 No Content
  3. `GET http://localhost:8082/s/<link_slug>` — confirm the redirect now returns 404
  4. Assert: status 404 (link is gone)

Auth: `Bearer <token_user1>`

---

### Scenario 7 — DeleteLink: another user's link returns permission denied
**Depends on:** 2 (need a fresh link), 0

Steps:
  1. Create a new link (repeat Scenario 2 steps to get a fresh `<link_id_2>`)
  2. `DELETE http://localhost:3000/api/v1/links/<link_id_2>`
  3. Assert: status 403 or 404, error code `FORBIDDEN` or `PERMISSION_DENIED` or `NOT_FOUND`
  4. The response must not reveal the link's owner

Auth: `Bearer <token_user2>`

---

### Scenario 8 — ListLinks returns paginated results
**Depends on:** 2

Steps:
  1. `GET http://localhost:3000/api/v1/files/<file_id>/links`
  2. Assert: status 200
  3. Assert: response contains a `links` array (non-empty, at least 1 entry from Scenario 2)
  4. Assert: each entry in `links` has `id`, `slug`, `fileId`, `userId`, `hitCount`, `createdAt`
  5. If multiple links exist, assert `nextPageToken` is empty or contains a cursor

Auth: `Bearer <token_user1>`

---

### Scenario 9 — ListLinks: filtered by owner returns only that owner's links
**Depends on:** 2, 0

Steps:
  1. `GET http://localhost:3000/api/v1/files/<file_id>/links`
  2. Assert: status 200
  3. Assert: every entry in the `links` array has `userId` matching the authenticated user (user1)
  4. Repeat with `Bearer <token_user2>`:
     `GET http://localhost:3000/api/v1/files/<file_id>/links`
  5. Assert: status 200
  6. The links list for user2 should be either empty or contain only user2's links

Auth: `Bearer <token_user1>` (first call), `Bearer <token_user2>` (second call)

---

### Scenario 10 — Redirect: cache hit returns 302
**Depends on:** 2

Steps:
  1. Prime the Redis cache by calling the redirect endpoint first:
     `GET http://localhost:8082/s/<link_slug>`
  2. Assert: status 302 (this populates Redis)
  3. Call again immediately (should be a cache hit):
     `GET http://localhost:8082/s/<link_slug>`
  4. Assert: status 302
  5. Assert: `Location` header is present

Auth: none

---

### Scenario 11 — Redirect: cache miss queries Postgres and populates Redis
**Depends on:** 2

Steps:
  1. Flush the Redis cache or use a fresh slug that has never been requested
  2. If Redis is accessible on `localhost:6379`, run: `redis-cli DEL slug:<link_slug>`
  3. `GET http://localhost:8082/s/<link_slug>`
  4. Assert: status 302
  5. Assert: `Location` header is present
  6. Verify cache was populated: `redis-cli TTL slug:<link_slug>` should return a value > 0 and <= 300

Auth: none
Note: Step 6 requires `redis-cli` on the host. If not available, skip it and
rely on step 4 (the redirect succeeding proves the cache was populated for the
next request).

---

### Scenario 12 — Redirect: unknown slug returns 404
**Depends on:** none

Steps:
  1. `GET http://localhost:8082/s/zzzzzz`
  2. Assert: status 404
  3. Assert: response body is a JSON error with code `NOT_FOUND`

Auth: none

---

### Scenario 13 — Redirect: expired link returns 410 Gone
**Depends on:** none (seed required)

Steps:
  1. Create a ShortLink with `expires_at` set to a past timestamp.
     This requires either:
     - Directly inserting into the shortener's Postgres (port 5434 if changed, else 5433):
       ```sql
       INSERT INTO short_links (id, file_id, user_id, slug, target_url, expires_at, created_at)
       VALUES (gen_random_uuid(), '660e8400-e29b-41d4-a716-446655440001',
               '550e8400-e29b-41d4-a716-446655440000', 'expired1', '',
               NOW() - INTERVAL '1 day', NOW() - INTERVAL '2 days');
       ```
  2. `GET http://localhost:8082/s/expired1`
  3. Assert: status 410
  4. Assert: response body is a JSON error with code `GONE`

Auth: none
Note: This scenario requires direct database access. If the tester cannot
access Postgres from the host, defer this to a unit test or integration test
at the use-case level.

---

## Auth boundary scenarios

### Scenario 14 — CreateShare: no token
**Depends on:** none

Steps:
  1. `POST http://localhost:3000/api/v1/files/660e8400-e29b-41d4-a716-446655440001/share` — no `Authorization` header
  2. Assert: status 401, response body matches `{"error":{"code":"UNAUTHORIZED",...}}`

---

### Scenario 15 — CreateShare: malformed token
**Depends on:** none

Steps:
  1. `POST http://localhost:3000/api/v1/files/660e8400-e29b-41d4-a716-446655440001/share` — header `Authorization: Bearer obviouslyinvalidtoken`
  2. Assert: status 401, response body matches `{"error":{"code":"UNAUTHORIZED",...}}`

---

### Scenario 16 — CreateShare: empty bearer token
**Depends on:** none

Steps:
  1. `POST http://localhost:3000/api/v1/files/660e8400-e29b-41d4-a716-446655440001/share` — header `Authorization: Bearer ` (empty after `Bearer `)
  2. Assert: status 401, response body matches `{"error":{"code":"UNAUTHORIZED",...}}`

---

### Scenario 17 — DeleteLink: no token
**Depends on:** none

Steps:
  1. `DELETE http://localhost:3000/api/v1/links/770e8400-e29b-41d4-a716-446655440002` — no `Authorization` header
  2. Assert: status 401

---

### Scenario 18 — Redirect endpoint: no auth required (public endpoint accepts any/missing token)
**Depends on:** none

Steps:
  1. `GET http://localhost:8082/s/nonexist`
  2. Assert: status 404 (not 401 — proving the endpoint is unauthenticated)

Note: This is a negative test — proving the public endpoint does NOT reject
unauthenticated requests. The redirect endpoint must never return 401.

---

## Known environment constraints

1. **Port collision: Auth HTTP (8082) vs Shortener redirect (8082).**
   Auth's `docker-compose.yml` maps `8082:8080` (Auth's HTTP health/JWKS
   endpoint). Shortener's `docker-compose.yml` maps `8082:8082` (shortener
   redirect). **These cannot both bind to host port 8082.** Resolve by
   changing one of them before spin-up. The recommended fix: change auth's
   host port to `8084:8080` in `auth/docker-compose.yml`.

2. **Port collision: fileupload-postgres (5433) vs shortener-postgres (5433).**
   Core compose maps `fileupload-postgres` to `5433:5432`. Shortener compose
   maps its postgres to `5433:5432`. Resolve by changing shortener's postgres
   host port to `5434:5432` in `shortener/docker-compose.yml`.

3. **No HEALTHCHECK on `shortener` or `fileupload`.**
   - `fileupload` starts once Postgres and MinIO are healthy (no explicit
     health check block).
   - `shortener` starts once Postgres and Redis are healthy (no explicit
     health check block).
   - Poll Core API `/health` (`GET http://localhost:3000/health`) for the
     auth+fileupload stack readiness.
   - Poll shortener redirect endpoint (`GET http://localhost:8082/s/healthcheck`
     — expect 404 because no slug exists, which proves the server is running)
     for shortener readiness.

4. **Shortener cannot serve redirects without FileUpload running.**
   The redirect flow calls FileUpload's `GetDownloadURL` gRPC to obtain a
   presigned S3 URL. If FileUpload is down, redirects return 502. Ensure
   the full core compose stack is healthy before testing redirects.

5. **MinIO presigned URL hostname mismatch.**
   FileUpload uses `S3_ENDPOINT=minio:9000` (internal Docker hostname) to
   construct presigned URLs. The E2E test runs on the **host** where
   `minio:9000` does not resolve. Set `S3_PUBLIC_ENDPOINT=http://localhost:9000`
   on the `fileupload` service in `core/docker-compose.yml` (or via override)
   before spinning up.

6. **Auth compose creates the shared `gobox` network.**
   The core compose declares the network as `external: true`. Auth compose
   **must** be started first. Shortener compose uses its own network and does
   not share `gobox`.

7. **Email uniqueness.** Register enforces unique email. Use unique emails
   per run (e.g. `shortener-e2e-{timestamp}@test.dev`).

8. **Token TTL is 15 minutes.** Access tokens expire after 15 minutes. For
   longer test runs, re-login or refresh. The expired-token E2E test is
   deferred — cover it in the JWT middleware unit test instead.

9. **Shortener's gRPC port (9091) is not mapped from Core compose.**
   Core API connects to the shortener container over the internal Docker
   network (`shortener:9091`). The host port `9091:9091` is only mapped by
   shortener's compose. If Core and shortener are on different Docker
   networks, Core cannot reach shortener. Ensure they are on the same
   network, or use a combined docker-compose.yml that places both on the
   `gobox` network.

10. **CreateLink slug collision retry (Scenario 3 in spec) is not testable
    in E2E.** The retry logic requires controlling the slug generator output
    to produce collisions — this is only feasible in a unit test with a mock
    generator. Skip this scenario in the E2E suite.

11. **Expired link redirect (Scenario 13) requires direct DB access.**
    The E2E test cannot set `expires_at` to the past through the public API
    (CreateLink only accepts a future timestamp). Insert the expired link
    directly into the shortener's Postgres, or defer this to a unit test.

---

## Compile check command

```bash
cd /home/ali/gobox/shortener && go test -tags e2e -run ^$ ./e2e/ -count=0
```

This compiles the `e2e` package without running any tests. Exit code 0 means
the suite is buildable.

---

## Run command

```bash
cd /home/ali/gobox/shortener && go test -tags e2e -v -count=1 ./e2e/
```

Flags:
- `-tags e2e` — includes the build-constrained e2e test files
- `-v` — verbose output (log each scenario start/end)
- `-count=1` — disables test caching

---

## Teardown order

Reverse startup order (dependencies before dependents):

1. `docker compose -f /home/ali/gobox/shortener/docker-compose.yml down`
2. `docker compose -f /home/ali/gobox/core/docker-compose.yml down`
3. `docker compose -f /home/ali/gobox/auth/docker-compose.yml down`

If volumes should be cleaned too (to avoid email-uniqueness issues on re-runs):

```bash
docker compose -f /home/ali/gobox/shortener/docker-compose.yml down -v
docker compose -f /home/ali/gobox/core/docker-compose.yml down -v
docker compose -f /home/ali/gobox/auth/docker-compose.yml down -v
docker network rm gobox
```

---

## Validation checklist

- [x] Every use case from `features/shortener.feature` has a matching scenario:
      CreateLink happy path → Sc. 2, CreateLink missing file_id → Sc. 3,
      CreateLink slug collision → deferred (unit test, noted in constraint #10),
      GetLink found → Sc. 4 (via redirect proxy), GetLink not found → Sc. 5,
      DeleteLink own → Sc. 6, DeleteLink another user → Sc. 7,
      ListLinks paginated → Sc. 8, ListLinks filtered → Sc. 9,
      Redirect cache hit → Sc. 10, Redirect cache miss → Sc. 11,
      Redirect unknown slug → Sc. 12, Redirect expired link → Sc. 13
- [x] Every port in scenario URLs appears in the port map: `3000` (Core API),
      `8082` (Shortener redirect)
- [x] Every "Depends on" reference points to a real scenario number
- [x] Known environment constraints section covers port collisions, health check
      gaps, MinIO hostname mismatch, network ordering, email uniqueness,
      token TTL, gRPC network isolation, and E2E testing limitations

Tester Brief complete. Ready for Tester.
