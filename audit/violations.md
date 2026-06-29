# GoBox Spec Violations Audit — 2026-06-29

## Context

- **Mode:** FORENSIC SPECIALIST
- **Scope:** auth/, core/, fileupload/, shortener/, gobox-proto/, go.work, docker-compose files
- **Treatment:** thumbgen service treated as non-existent per audit instructions
- **Spec:** GOBOX_SPEC.md v0.2

---

## Violations

---

### V-AUTH-001 — Business logic bypassing use case layer in gRPC server
- **Service:** auth
- **Category:** Architecture
- **Severity:** WARNING
- **File:** auth/internal/interface/grpc/server.go
- **Location:** Lines 229–255 (ValidateSession method)
- **Symptom:** The `ValidateSession` gRPC method directly queries the session repository (`s.sessionRepo.FindByID`) and applies business rules (checking `Revoked`, checking `ExpiresAt`) instead of delegating to a use case. This bypasses the layered DDD architecture.
- **Spec ref:** Section 4 (Dependency rule: `interface → application → domain`)
- **Root cause:** ValidateSession was implemented as a direct repository call rather than creating a dedicated use case.
- **Fix:** Create a `ValidateSessionUseCase` that encapsulates the session validation logic, and call it from the gRPC handler.
- **Cascades to:** none

---

### V-AUTH-002 — Unapproved direct dependency: golang.org/x/crypto [RESOLVED]
- **Service:** auth
- **Category:** Dependency
- **Severity:** RESOLVED
- **File:** auth/go.mod
- **Location:** Line 13 (`golang.org/x/crypto v0.53.0`)
- **Symptom:** `golang.org/x/crypto` is a direct dependency in `auth/go.mod` but was NOT in the approved dependency list.
- **Resolution:** User confirmed `golang.org/x/crypto` should be added to the approved dependency list.
- **Fix:** Add `golang.org/x/crypto` to the approved dependency list in GOBOX_SPEC.md section 9 (requires Agent operating rules update via PR).

---

### V-AUTH-003 — Unapproved direct dependency: golang.org/x/sync [RESOLVED]
- **Service:** auth
- **Category:** Dependency
- **Severity:** RESOLVED
- **File:** auth/go.mod
- **Location:** Line 14 (`golang.org/x/sync v0.21.0`)
- **Symptom:** `golang.org/x/sync` is a direct dependency in `auth/go.mod` (used for errgroup in `cmd/main.go`) but was NOT in the approved dependency list.
- **Resolution:** User confirmed `golang.org/x/sync` should be added to the approved dependency list.
- **Fix:** Add `golang.org/x/sync` to the approved dependency list in GOBOX_SPEC.md section 9 (requires Agent operating rules update via PR).

---

### V-CORE-001 — THUMBGEN_GRPC_ADDR env var wired for forward compatibility (INFO)
- **Service:** core
- **Category:** ThumbgenCoupling
- **Severity:** INFO
- **File:** core/.env.example (line 9), core/docker-compose.yml (line 11), core/pkg/config/config.go (line 34)
- **Location:** Config wiring
- **Symptom:** `core/.env.example` defines `THUMBGEN_GRPC_ADDR=thumbgen:8083`. `core/pkg/config/config.go` defaults to `localhost:8083`. Core `cmd/main.go` passes it to `thumbgen.NewClient()`.
- **Assessment:** The `github.com/aligh5331/gobox-proto/gen/thumbgen/v1` proto package was developed and exists. The thumbgen server was never implemented. The core stub correctly imports the proto types (which exist) and uses a no-op client that never dials — it only validates addr is non-empty. This is an intentional forward-compatibility stub, not a violation. The env var and stub coexist gracefully without requiring a running thumbgen service.
- **Spec ref:** Section 5.2 (Core API gRPC clients: ThumbGen listed as downstream)
- **Root cause:** Proto types exist, server not implemented, stub is the correct placeholder.
- **Status:** NOT A VIOLATION. Stub is the intended pattern.

---

### V-CORE-005 — Extra route not in spec: GET /api/v1/files/{id}/download
- **Service:** core
- **Category:** Contract
- **Severity:** WARNING
- **File:** core/internal/interface/rest/router.go
- **Location:** Line 41
- **Symptom:** Route `GET /api/v1/files/:id/download` is registered but is NOT in the spec's REST endpoints list (section 5.2). It provides GetDownloadURL functionality which is documented in the FileUpload use cases (section 5.3) but not as a Core API endpoint.
- **Spec ref:** Section 5.2 (REST endpoints)
- **Root cause:** GetDownloadURL was added as a convenience endpoint for clients to get a fresh download URL. Not explicitly specified as a Core REST endpoint.
- **Assessment:** This endpoint IS needed. The FileUpload spec (section 5.3) lists GetDownloadURL as a use case, and clients need a way to obtain fresh presigned download URLs (especially for the redirect flow in shortener). It should be added to the spec.
- **Fix:** Add `GET /api/v1/files/{id}/download` ← GetDownloadURL (fresh presigned GET URL) to the REST endpoints table in GOBOX_SPEC.md section 5.2.
- **Cascades to:** none

---

### V-CORE-006 — Extra routes not in spec: share list/delete endpoints
- **Service:** core
- **Category:** Contract
- **Severity:** WARNING
- **File:** core/internal/interface/rest/router.go
- **Location:** Lines 45-46
- **Symptom:** Routes `GET /api/v1/files/:id/links` and `DELETE /api/v1/links/:link_id` are registered but are NOT in the spec's REST endpoints list (section 5.2 only lists `POST /api/v1/files/{id}/share`).
- **Spec ref:** Section 5.2 (REST endpoints)
- **Root cause:** Extra share management endpoints were implemented beyond the spec's minimum.
- **Authorization:** User has authorized adding these to the spec.
- **Fix:** Add to GOBOX_SPEC.md section 5.2 REST endpoints table:
  ```  
  GET    /api/v1/files/{id}/links         ← list share links for a file  
  DELETE /api/v1/links/{link_id}           ← delete a specific share link  
  ```
- **Cascades to:** none

---

### V-CORE-007 — FileUpload port mismatch in .env.example and config default
- **Service:** core
- **Category:** Configuration
- **Severity:** WARNING
- **File:** core/.env.example (line 8), core/pkg/config/config.go (line 33)
- **Location:** Config
- **Symptom:** `core/.env.example` sets `FILEUPLOAD_GRPC_ADDR=fileupload:8082`. `core/pkg/config/config.go` defaults to `localhost:8082`. But the spec (section 5.3) says FileUpload gRPC port is 9090. The `core/docker-compose.yml` correctly overrides this to `fileupload:9090`, but the defaults and .env.example are wrong.
- **Spec ref:** Section 5.3 (FileUpload port 9090), Section 7 (cross-cutting config table: `FILEUPLOAD_GRPC_ADDR`)
- **Root cause:** Config was initialized with wrong port number (8082 instead of 9090).
- **Fix:** Change `FILEUPLOAD_GRPC_ADDR=fileupload:9090` in `.env.example` and default to `localhost:9090` in `config.go`.
- **Cascades to:** none

---

### V-FILEUPLOAD-001 — db.AutoMigrate() used instead of SQL migrations
- **Service:** fileupload
- **Category:** Architecture
- **Severity:** BLOCKER
- **File:** fileupload/cmd/main.go
- **Location:** Line 46
- **Symptom:** `cmd/main.go` calls `db.AutoMigrate(&model.File{})` instead of running SQL migration files through `github.com/golang-migrate/migrate/v4`.
- **Spec ref:** Section 9 ("Never use `db.AutoMigrate()` in production code. All schema changes go through SQL migration files.")
- **Root cause:** Developer used GORM AutoMigrate as a shortcut instead of implementing proper migration runner.
- **Fix:** Remove `db.AutoMigrate` call. Import `github.com/golang-migrate/migrate/v4` and `_ "github.com/golang-migrate/migrate/v4/database/postgres"` and `_ "github.com/golang-migrate/migrate/v4/source/file"`. Add a `runMigrations` function (same pattern as auth/cmd/main.go lines 177-188). Rename migration file to `001_create_files.up.sql` for golang-migrate compatibility.
- **Cascades to:** V-FILEUPLOAD-002

---

### V-FILEUPLOAD-002 — Migration file naming incompatible with golang-migrate
- **Service:** fileupload
- **Category:** Architecture
- **Severity:** WARNING
- **File:** fileupload/migrations/001_create_files.sql
- **Location:** Filename
- **Symptom:** Migration file is named `001_create_files.sql` instead of `001_create_files.up.sql`. The golang-migrate file source driver requires `.up.sql` suffix to discover migration files. Additionally, no `.down.sql` exists.
- **Spec ref:** Section 7 ("Use `github.com/golang-migrate/migrate/v4`. SQL files live in `migrations/`.")
- **Root cause:** Migration naming convention mismatch. Auth service uses correct naming (`001_create_users.up.sql`), but fileupload does not.
- **Fix:** Rename to `001_create_files.up.sql`. Optionally add `001_create_files.down.sql`.
- **Cascades to:** V-FILEUPLOAD-001 must be fixed first (need migration runner)

---

### V-FILEUPLOAD-003 — Uses log/slog instead of zerolog
- **Service:** fileupload
- **Category:** Architecture
- **Severity:** WARNING
- **File:** fileupload/cmd/main.go
- **Location:** Lines 6, 28
- **Symptom:** `cmd/main.go` imports and uses `log/slog` (stdlib structured logger) instead of `github.com/rs/zerolog` as required by the spec. No zerolog import exists anywhere in the fileupload service.
- **Spec ref:** Section 7 ("Use `github.com/rs/zerolog`. All logs are structured JSON to stdout.")
- **Root cause:** Developer chose `log/slog` (new in Go 1.21) over zerolog. Not consistent with other services.
- **Fix:** Replace `log/slog` imports with `github.com/rs/zerolog`. Add `pkg/logger/` package consistent with auth and core. Replace all `slog.Error`/`slog.Info` calls with zerolog equivalents.
- **Cascades to:** none

---

### V-FILEUPLOAD-004 — GORM tags on domain model (infrastructure coupling)
- **Service:** fileupload
- **Category:** Architecture
- **Severity:** WARNING
- **File:** fileupload/internal/domain/model/file.go
- **Location:** All field declarations
- **Symptom:** The `File` domain struct has GORM struct tags (`gorm:"type:uuid;primaryKey"`, `gorm:"index"`, etc.) directly on the domain entity. This creates an infrastructure coupling: changing ORM requires changing domain models.
- **Spec ref:** Section 4 ("Domain layer must not import infrastructure.")
- **Root cause:** Domain model was designed with GORM's serialization model instead of keeping it pure.
- **Fix:** Remove GORM tags from domain models. Define a separate DTO or use GORM's `TableName` and model mapping in the repository layer instead.
- **Cascades to:** none

---

### V-SHORTENER-001 — No migration runner at startup
- **Service:** shortener
- **Category:** Architecture
- **Severity:** BLOCKER
- **File:** shortener/cmd/main.go
- **Location:** No migration call anywhere
- **Symptom:** `shortener/cmd/main.go` contains no call to run migrations. No `migrate.New`, `migrate.Up`, or `db.AutoMigrate` exists. The Postgres connection is opened but no schema initialization occurs. The `short_links` table will never be created.
- **Spec ref:** Section 7 ("Migrations run automatically at startup (`migrate.Up()`).")
- **Root cause:** Migration runner was never implemented for the shortener service.
- **Fix:** Import `github.com/golang-migrate/migrate/v4`, `_ "github.com/golang-migrate/migrate/v4/database/postgres"`, and `_ "github.com/golang-migrate/migrate/v4/source/file"`. Add a `runMigrations` function and call it after DB connection in main.go. Rename migration file to `001_create_short_links.up.sql`.
- **Cascades to:** V-SHORTENER-002

---

### V-SHORTENER-002 — Migration file naming incompatible with golang-migrate
- **Service:** shortener
- **Category:** Architecture
- **Severity:** WARNING
- **File:** shortener/migrations/001_create_short_links.sql
- **Location:** Filename
- **Symptom:** Migration file is named `001_create_short_links.sql` instead of `001_create_short_links.up.sql`. The golang-migrate file source driver requires `.up.sql` suffix to discover migration files.
- **Spec ref:** Section 7 ("Use `github.com/golang-migrate/migrate/v4`. SQL files live in `migrations/`.")
- **Root cause:** Same issue as V-FILEUPLOAD-002 — naming convention mismatch.
- **Fix:** Rename to `001_create_short_links.up.sql`. Optionally add `001_create_short_links.down.sql`.
- **Cascades to:** FIX MUST BE DONE WITHIN V-SHORTENER-001

---

### V-SHORTENER-003 — No unit tests for any use case
- **Service:** shortener
- **Category:** Test
- **Severity:** BLOCKER
- **File:** shortener/internal/ (no *_test.go files in application/usecase/)
- **Location:** Entire service
- **Symptom:** Shortener has zero unit test files in its `internal/` or `pkg/` directories. The only test file is `e2e/shortener_e2e_test.go` (with build tag `//go:build e2e`). The 5 use cases (CreateLink, GetLink, DeleteLink, ListLinks, IncrementHitCount) have no corresponding unit tests.
- **Spec ref:** Section 9 ("Every use case gets a unit test. Mock the repository interface; do not hit a real DB in unit tests.")
- **Root cause:** Shortener was implemented without unit tests.
- **Fix:** Create unit tests for each use case in `internal/application/usecase/`. Use mock repository pattern (similar to auth's `mock_repository_test.go` and fileupload's mock).
- **Cascades to:** none

---

### V-SHORTENER-004 — GORM tags on domain model (infrastructure coupling)
- **Service:** shortener
- **Category:** Architecture
- **Severity:** WARNING
- **File:** shortener/internal/domain/model/shortlink.go
- **Location:** All field declarations
- **Symptom:** The `ShortLink` domain struct has GORM struct tags (`gorm:"type:uuid;primaryKey"`, `gorm:"uniqueIndex"`, etc.) directly on the domain entity. Same violation as V-FILEUPLOAD-004.
- **Spec ref:** Section 4 ("Domain layer must not import infrastructure.")
- **Root cause:** Same as V-FILEUPLOAD-004.
- **Fix:** Remove GORM tags from domain models. Handle ORM mapping in the repository layer.
- **Cascades to:** none

---

### V-SHORTENER-005 — Missing pkg/logger package
- **Service:** shortener
- **Category:** Architecture
- **Severity:** WARNING
- **File:** shortener/pkg/ (directory listing)
- **Location:** Missing logger/
- **Symptom:** Shortener's `pkg/` directory has `config/` and `slug/` but is missing `pkg/logger/`. The `newLogger` function is defined inline in `cmd/main.go` instead of in a reusable logger package.
- **Spec ref:** Section 4 (monorepo layout shows `pkg/logger/` for every service)
- **Root cause:** Logger package was not created for shortener.
- **Fix:** Create `shortener/pkg/logger/logger.go` following the same pattern as `auth/pkg/logger/logger.go`.
- **Cascades to:** none

---

### V-CROSS-001 — Host port 5432 collision between auth and fileupload docker-compose
- **Service:** cross-service (auth, fileupload)
- **Category:** Deployment
- **Severity:** WARNING
- **File:** auth/docker-compose.yml (line 34), fileupload/docker-compose.yml (line 31)
- **Location:** Port mapping
- **Symptom:** Both `auth/docker-compose.yml` and `fileupload/docker-compose.yml` map host port `5432:5432` to their respective Postgres containers. If both compose files run simultaneously (as required for integration testing in Phase 6), the host port 5432 will conflict and the second compose file will fail to start.
- **Spec ref:** Section 3 (Core API is the only service with a public port), Phase 6 (integration tests)
- **Root cause:** Both compose files default to standard Postgres port 5432 on the host.
- **Fix:** Use different host ports for each service's Postgres. Auth's docker-compose.yml already maps to auth-network internally; fileupload's core docker-compose.yml already uses `5433:5432`. The standalone fileupload docker-compose.yml should also use a different port (e.g., `5435:5432`).
- **Cascades to:** Phase 6 integration test execution

---

### V-CROSS-002 — Auth and shortener docker-compose files do not join external "gobox" network
- **Service:** cross-service (auth, shortener)
- **Category:** Deployment
- **Severity:** WARNING
- **File:** auth/docker-compose.yml (line 49-52), shortener/docker-compose.yml (line 58-61)
- **Location:** Networks configuration
- **Symptom:** `auth/docker-compose.yml` defines its own `gobox` network with `name: gobox` and `driver: bridge` — it creates a new network named "gobox" (not declared as external). `shortener/docker-compose.yml` declares `gobox` network as `external: true` with `name: gobox`. Meanwhile `core/docker-compose.yml` also declares `gobox` as `external: true`. This means:
  - Running `auth/docker-compose.yml` alone creates a network named "gobox"
  - Running `core/docker-compose.yml` expects an externally-created "gobox" network
  - If no network named "gobox" exists when starting core's compose, it will fail because `external: true` means the network must already exist
- **Spec ref:** Section 3 (internal service topology — all services communicate via internal docker network)
- **Root cause:** Inconsistent network strategy across services. Auth creates it, core expects it pre-created.
- **Fix:** Standardize network strategy. Either all compose files declare `external: true` (requiring a one-time `docker network create gobox`), or one compose file creates it and others reference it without `external: true`.
- **Cascades to:** Phase 6 integration test execution

---

### V-CROSS-003 — Auth HTTP port 8080 conflict with core HTTP port 8080 (container-exposed, same host only)
- **Service:** cross-service (auth, core)
- **Category:** Deployment
- **Severity:** WARNING
- **File:** auth/docker-compose.yml (line 6), core/docker-compose.yml (line 5)
- **Location:** Port mappings
- **Symptom:** Auth maps `8084:8080` (host 8084 → container 8080). Core maps `3000:8080` (host 3000 → container 8080). Internally both use container port 8080 but on different host ports. This is fine. However, a developer running outside docker-compose with `.env.example` defaults would start auth on `:8080` (HTTP) and core on `:8080` (HTTP), causing a conflict on host port 8080.
- **Spec ref:** Section 5.1 (Auth HTTP port 8080), Section 5.2 (Core port 8080)
- **Root cause:** Both services default to the same host port 8080 in their .env.example files.
- **Fix:** Document in each .env.example that port overrides are needed when running multiple services locally, or change auth's default HTTP port to 8084 (matching its docker-compose host port).
- **Cascades to:** Local development outside docker-compose

---

## Blocker summary

| ID | Service | Category | File | Fix (one line) |
|----|---------|----------|------|----------------|
| V-FILEUPLOAD-001 | fileupload | Architecture | fileupload/cmd/main.go:46 | Replace `db.AutoMigrate` with golang-migrate `runMigrations` call |
| V-SHORTENER-001 | shortener | Architecture | shortener/cmd/main.go | Add migration runner using golang-migrate at startup |
| V-SHORTENER-003 | shortener | Test | shortener/internal/application/usecase/ | Write unit tests for all 5 use cases with mocked repository |

## Warning summary

| ID | Service | Category | File | Fix (one line) |
|----|---------|----------|------|----------------|
| V-AUTH-001 | auth | Architecture | auth/internal/interface/grpc/server.go:229 | Extract ValidateSession logic into a dedicated use case |
| V-CORE-005 | core | Contract | core/internal/interface/rest/router.go:41 | Add GET /files/:id/download to spec endpoint table |
| V-CORE-006 | core | Contract | core/internal/interface/rest/router.go:45 | Add share list/delete endpoints to spec |
| V-CORE-007 | core | Configuration | core/.env.example:8 | Fix FILEUPLOAD_GRPC_ADDR from :8082 to :9090 |
| V-FILEUPLOAD-002 | fileupload | Architecture | fileupload/migrations/ | Rename migration to 001_create_files.up.sql for golang-migrate compat |
| V-FILEUPLOAD-003 | fileupload | Architecture | fileupload/cmd/main.go:6 | Replace log/slog with github.com/rs/zerolog |
| V-FILEUPLOAD-004 | fileupload | Architecture | fileupload/internal/domain/model/file.go | Remove GORM tags from domain model |
| V-SHORTENER-002 | shortener | Architecture | shortener/migrations/ | Rename migration to 001_create_short_links.up.sql |
| V-SHORTENER-004 | shortener | Architecture | shortener/internal/domain/model/shortlink.go | Remove GORM tags from domain model |
| V-SHORTENER-005 | shortener | Architecture | shortener/pkg/ | Add pkg/logger/ package |
| V-CROSS-001 | cross-service | Deployment | auth/docker-compose.yml:34 | Change host port for fileupload Postgres to avoid 5432 collision |
| V-CROSS-002 | cross-service | Deployment | auth/docker-compose.yml:49 | Standardize docker network strategy across all compose files |
| V-CROSS-003 | cross-service | Deployment | auth/.env.example | Resolve host port 8080 conflict between auth and core for local dev |

---

## Cascade map

- V-FILEUPLOAD-001 must be fixed before V-FILEUPLOAD-002 (migration naming fix without a runner is useless)
- V-SHORTENER-001 must be fixed before V-SHORTENER-002 (same reason)
- V-CROSS-001 + V-CROSS-002 both affect Phase 6 integration test execution; both must be fixed before E2E tests can run across all services
- V-CROSS-003 affects local development; no cascade to deployed environments using docker-compose

## Resolved items

The following items from the original audit are no longer violations:

| ID | Reason |
|----|--------|
| V-AUTH-002 | User approved `golang.org/x/crypto` as a dependency |
| V-AUTH-003 | User approved `golang.org/x/sync` as a dependency |
| V-CORE-002 | Proto types exist in gobox-proto; the thumbgen server was never implemented. The stub correctly imports the existing proto and never dials. Not a violation. |
| V-CORE-003 | Same as V-CORE-002 — handler imports proto types that exist, calls a stub that never dials. Correct pattern. |
| V-CORE-004 | Same as V-CORE-002 — cmd/main.go wires the stub, which never dials. Proto types are valid deps. |

## Pending spec updates

The following changes to GOBOX_SPEC.md are authorized by the user:

1. Add `golang.org/x/crypto` and `golang.org/x/sync` to the approved dependency list (section 9)
2. Add `GET /api/v1/files/{id}/download` ← GetDownloadURL (fresh presigned GET URL) to the REST endpoints table (section 5.2)
3. Add share management endpoints to the REST endpoints table (section 5.2):
   - `GET    /api/v1/files/{id}/links         ← list share links for a file`
   - `DELETE /api/v1/links/{link_id}           ← delete a specific share link`

These spec updates should be performed by a **Librarian** session before the next Builder session.

---

Audit complete. 3 blockers, 13 warnings. Ready for Architect.
