# GoBox Violation Fix Plan

**Author:** Architect
**Date:** 2026-06-29
**Source:** audit/violations.md

---

## 1 — Fix Order

### Session A (Thumbgen Decoupling — fix first, no dependencies on other sessions)
| Violation | Service | Description |
|-----------|---------|-------------|
| V-CORE-001 (decouple) | core | Remove THUMBGEN_GRPC_ADDR from .env.example and docker-compose; blank the config default; nil-guard the stub in cmd/main.go and file handler |
| V-CORE-003 (decouple) | core | Same handler import dependency — shield behind an interface so file.go does not import thumbgen proto directly |
| V-CORE-004 (decouple) | core | Same cmd/main.go wiring — make the thumbgen client nil-safe |

These must be first because they determine whether `core` compiles when thumbgen service and its proto are absent.

---

### Session B (FileUpload — all internal, same service)
| Order | Violation | Description | Depends on |
|-------|-----------|-------------|------------|
| B.1 | V-FILEUPLOAD-001 | **BLOCKER** — Replace `db.AutoMigrate` with golang-migrate `runMigrations` | — |
| B.2 | V-FILEUPLOAD-002 | Rename `001_create_files.sql` → `001_create_files.up.sql` | B.1 |
| B.3 | V-FILEUPLOAD-003 | Replace `log/slog` with `github.com/rs/zerolog` | — |
| B.4 | V-FILEUPLOAD-004 | Remove GORM tags from domain model `file.go` | — |

Order within session: B.1 → B.2 (cascading), B.3 and B.4 independent (can be done in any order after B.2).

---

### Session C (Shortener — all internal, same service)
| Order | Violation | Description | Depends on |
|-------|-----------|-------------|------------|
| C.1 | V-SHORTENER-001 | **BLOCKER** — Add migration runner using golang-migrate | — |
| C.2 | V-SHORTENER-002 | Rename `001_create_short_links.sql` → `001_create_short_links.up.sql` | C.1 (must be done WITHIN C.1) |
| C.3 | V-SHORTENER-003 | **BLOCKER** — Write unit tests for all 5 use cases | — |
| C.4 | V-SHORTENER-004 | Remove GORM tags from domain model `shortlink.go` | — |
| C.5 | V-SHORTENER-005 | Create `pkg/logger/` package | — |

Order within session: C.1 → C.2 (cascading, fix together), C.3–C.5 independent (can be done in any order after C.2).

---

### Session D (Auth fix)
| Violation | Service | Description | Depends on |
|-----------|---------|-------------|------------|
| V-AUTH-001 | auth | Extract `ValidateSession` logic into a dedicated use case | — |

---

### Session E (Core spec + config)
| Violation | Service | Description | Depends on |
|-----------|---------|-------------|------------|
| V-CORE-007 | core | Fix `FILEUPLOAD_GRPC_ADDR` default from `:8082` to `:9090` in `.env.example` and `config.go` | — |
| V-CORE-005 | core | Add `GET /api/v1/files/{id}/download` to spec (GOBOX_SPEC.md §5.2) | — |
| V-CORE-006 | core | Add share list/delete endpoints to spec (GOBOX_SPEC.md §5.2) | — |

All independent — can be done in any order.

---

### Session F (Cross-service deployment)
| Violation | Service | Description | Depends on |
|-----------|---------|-------------|------------|
| V-CROSS-001 | cross (auth, fileupload) | Change fileupload's standalone Postgres host port from `5432` to `5435` | — |
| V-CROSS-002 | cross (auth, shortener) | Standardize docker network strategy — make `auth/docker-compose.yml` use `external: true` | — |
| V-CROSS-003 | cross (auth, core) | Resolve host port 8080 conflict for local dev in `.env.example` files | — |

All independent — can be done in any order.

---

## 2 — Pre-emptive Cascading Breakage

### Session A (Thumbgen Decoupling)

#### Trigger: V-CORE-001 decouple — blanking `THUMBGEN_GRPC_ADDR` default
- **Config struct field removed or made optional:** `core/pkg/config/config.go` — `ThumbGenGRPCAddr` field.
- **Pre-emptive fix A.1:** If the field is removed from `Config`, update all references:
  - `core/cmd/main.go` line 73: `cfg.ThumbGenGRPCAddr` → remove the call to `thumbgen.NewClient`
  - `core/internal/interface/rest/handler/file.go` line 22: `thumb *thumbgen.Client` → replace with a local interface `Thumbnailer` (defined in handler package) so the proto import is not needed
  - `core/internal/interface/rest/handler/file.go` lines 104-115: wrap the `EnqueueJob` call in a nil-guard on `h.thumb`
- **Pre-emptive fix A.2:** In `core/docker-compose.yml`, remove the `THUMBGEN_GRPC_ADDR` env var (line 11). No other docker-compose file references this var.

#### Trigger: V-CORE-003 decouple — handler imports thumbgen proto directly
- **Pre-emptive fix A.3:** Define a local interface in `handler/file.go`:
  ```go
  type Thumbnailer interface {
      EnqueueJob(ctx context.Context, req *thumbgenv1.EnqueueJobRequest) (*thumbgenv1.EnqueueJobResponse, error)
  }
  ```
  This shields the handler from the concrete proto type. The stub constructor can return a nil-safe wrapper. No gobox-proto changes needed.

#### Trigger: V-CORE-004 decouple — cmd/main.go wiring
- **Pre-emptive fix A.4:** In `core/cmd/main.go`, replace the `thumbgen.NewClient` call with:
  ```go
  var thumbgenClient *thumbgen.Client
  if cfg.ThumbGenGRPCAddr != "" {
      thumbgenClient, err = thumbgen.NewClient(ctx, cfg.ThumbGenGRPCAddr)
      if err != nil {
          log.Fatal().Err(err).Msg("failed to create thumbgen client")
      }
      defer thumbgenClient.Close()
  }
  ```
  Pass `thumbgenClient` (which may be nil) to `handler.NewFileHandler`. The handler nil-guards it.

**Verification:** After Session A, `core` must compile and start without a running thumbgen service. `core go vet ./...` must pass.

---

### Session B (FileUpload)

#### Trigger: V-FILEUPLOAD-001 — migration runner replaces AutoMigrate
- **Pre-emptive fix B.1:** Add `github.com/golang-migrate/migrate/v4`, `_ "github.com/golang-migrate/migrate/v4/database/postgres"`, and `_ "github.com/golang-migrate/migrate/v4/source/file"` to `fileupload/go.mod`. Run `go mod tidy` in the `fileupload/` directory.
- **Pre-emptive fix B.2:** The `runMigrations` function signature must match the pattern in `auth/cmd/main.go` lines 177-188. Use `"file://migrations"` as the source URL.

#### Trigger: V-FILEUPLOAD-002 — migration rename (part of B.1)
- **Pre-emptive fix B.3:** Rename `fileupload/migrations/001_create_files.sql` → `fileupload/migrations/001_create_files.up.sql`. Create an empty `001_create_files.down.sql` with a `DROP TABLE IF EXISTS files;` statement for completeness.
- **No cascade to core:** The `core/docker-compose.yml` mounts `../fileupload/migrations` as `/docker-entrypoint-initdb.d` (line 63). This is a Postgres init script fallback, not a golang-migrate source. The init script copies the `.sql` file regardless of suffix. **No change needed** — renaming to `.up.sql` is compatible with both methods.

#### Trigger: V-FILEUPLOAD-003 — slog → zerolog
- **Pre-emptive fix B.4:** Remove `log/slog` import. Add `github.com/rs/zerolog` to `fileupload/go.mod` (if not already present as transitive). Create `fileupload/pkg/logger/` following `auth/pkg/logger/logger.go` pattern. Replace all `slog.Error`/`slog.Info` calls.
- **No cascade:** This is entirely internal to `fileupload/`. No other service imports `fileupload/cmd/main.go`.

#### Trigger: V-FILEUPLOAD-004 — GORM tags removed from domain model
- **Pre-emptive fix B.5:** Create a DTO struct in the repository layer (`internal/infrastructure/postgres/`) that carries the GORM tags. Update the repository methods to map between DTO and domain model.
- **No cascade:** Domain model is internal to `fileupload/internal/domain/`. No other service imports it. However, if the database schema changes (column name, type), the migration SQL file (`001_create_files.up.sql`) must be kept in sync.

---

### Session C (Shortener)

#### Trigger: V-SHORTENER-001 — migration runner added
- **Pre-emptive fix C.1:** Add `github.com/golang-migrate/migrate/v4` + sub-packages to `shortener/go.mod`. Add `_ "github.com/golang-migrate/migrate/v4/database/postgres"` and `_ "github.com/golang-migrate/migrate/v4/source/file"` imports. Run `go mod tidy` in `shortener/`.
- **Pre-emptive fix C.2:** Insert `runMigrations(cfg.DatabaseURL, log)` call after the `sqlDB` connection (line 59) and before the `redisCache` line (line 62). Use the same pattern as auth.

#### Trigger: V-SHORTENER-002 — migration rename (part of C.1)
- **Pre-emptive fix C.3:** Rename `shortener/migrations/001_create_short_links.sql` → `001_create_short_links.up.sql`. Add a down migration.

#### Trigger: V-SHORTENER-003 — unit tests (BLOCKER)
- **Pre-emptive fix C.4:** No cascade to other services. Test files are internal to `shortener/internal/`. Follow the pattern in `auth/` mock repository tests. Five use cases → five test files, each with a mock repository.
- **No signature changes needed:** The use case constructors and methods stay the same.

#### Trigger: V-SHORTENER-004 — GORM tags removed
- **Pre-emptive fix C.5:** Same strategy as V-FILEUPLOAD-004: create a DTO in `shortener/internal/infrastructure/postgres/`. No cascade to other services.

#### Trigger: V-SHORTENER-005 — pkg/logger created
- **Pre-emptive fix C.6:** Create `shortener/pkg/logger/logger.go`. Then update `shortener/cmd/main.go` to use `logger.New(cfg.LogLevel)` instead of the inline `newLogger` function.
- **No cascade to other services.**

---

### Session D (Auth)

#### Trigger: V-AUTH-001 — ValidateSession use case
- **Pre-emptive fix D.1:** Create `auth/internal/application/usecase/validate_session.go` with a `ValidateSessionUseCase` that encapsulates:
  - Calling `sessionRepo.FindByID`
  - Checking `Revoked`
  - Checking `ExpiresAt`
- **Pre-emptive fix D.2:** Change `auth/internal/interface/grpc/server.go` to call the use case instead of the repository directly.
- **No cascade to gobox-proto:** The gRPC proto definition (`ValidateSession` RPC) does not change. The request/response types stay the same. No other service imports `auth/internal/interface/grpc`.
- **No cascade to core:** The core `grpcclient/auth/client.go` calls the same gRPC method. As long as the proto and method signature don't change, no change needed in core.

---

### Session E (Core spec + config)

#### Trigger: V-CORE-007 — FileUpload port fix
- **Pre-emptive fix E.1:** Change `core/.env.example` line 8 from `fileupload:8082` to `fileupload:9090`.
- **Pre-emptive fix E.2:** Change `core/pkg/config/config.go` line 33 default from `localhost:8082` to `localhost:9090`.
- **Cascade check:** `core/docker-compose.yml` line 10 already has `FILEUPLOAD_GRPC_ADDR: "fileupload:9090"`. Also `shortener/docker-compose.yml` line 14 has `FILEUPLOAD_GRPC_ADDR: fileupload:9090`. **No change needed** — other docker-compose files already use 9090. The `.env.example` and config default were the only incorrect references.
- **No cascade to other services.**

#### Trigger: V-CORE-005 / V-CORE-006 — Spec additions
- **Pre-emptive fix E.3:** Update `GOBOX_SPEC.md` §5.2 REST endpoints table. These are **spec-only changes** — no code changes needed. Builder must not modify `GOBOX_SPEC.md` autonomously per AGENTS.md rules. **Hand off to Librarian.**
- **No code cascade.**

---

### Session F (Cross-service deployment)

#### Trigger: V-CROSS-001 — Postgres port 5432 collision
- **Pre-emptive fix F.1:** In `fileupload/docker-compose.yml` line 31, change `"5432:5432"` to `"5435:5432"`.
- **Cascade check:** No other docker-compose file references `fileupload`'s Postgres host port. The `core/docker-compose.yml` already uses `5433:5432` for its own fileupload-postgres. The `shortener/docker-compose.yml` uses `5434:5432`. **No cascade** — each Postgres already has a unique host port.
- **Pre-emptive fix F.2:** Update `fileupload/.env.example` line 4 from `localhost:5432` to `localhost:5435` to match.

#### Trigger: V-CROSS-002 — Docker network strategy mismatch
- **Pre-emptive fix F.3:** In `auth/docker-compose.yml` lines 49-52, change:
  ```yaml
  networks:
    gobox:
      external: true
      name: gobox
  ```
  This makes auth consistent with core and shortener (both use `external: true`).
- **Pre-emptive fix F.4:** Add a `networks:` block to `fileupload/docker-compose.yml`:
  ```yaml
  networks:
    gobox:
      external: true
      name: gobox
  ```
  And add `networks: [gobox]` to each service in that file. Currently `fileupload/docker-compose.yml` has no networks section at all — services join the default bridge network, making them unreachable from other compose files.
- **Cascade to documentation:** After this fix, running any service standalone requires `docker network create gobox` first. Add a comment to each `.env.example` or a `DEVELOPMENT.md` note.

#### Trigger: V-CROSS-003 — HTTP port 8080 conflict
- **Pre-emptive fix F.5:** Either:
  (a) Change `auth/.env.example` HTTP_PORT default from `8080` to `8084` (matching its docker-compose host mapping), or
  (b) Add a comment to both `.env.example` files warning about the conflict.
- **Pre-emptive fix F.6:** If option (a) is chosen, update `auth/pkg/config/config.go` default from `8080` to `8084` as well.
- **No cascade to docker-compose:** Both compose files already use unique host mappings (`8084:8080` for auth, `3000:8080` for core).

---

## 3 — Thumbgen Decoupling Plan (Session A specifics)

The thumbgen service does not exist in this commit and must not be required. The project compiles and runs without it. Below are all coupling locations and the exact decoupling strategy for each.

### Location 1: `core/.env.example` line 9
**Content:** `THUMBGEN_GRPC_ADDR=thumbgen:8083`
**Strategy:** Remove the line entirely. The config struct field can remain with an empty default, but the env var suggestion should not exist.

### Location 2: `core/docker-compose.yml` line 11
**Content:** `THUMBGEN_GRPC_ADDR: "thumbgen:8083"`
**Strategy:** Remove the env var entry from the `core` service environment block.

### Location 3: `core/pkg/config/config.go` line 17-18, 34
**Content:** `ThumbGenGRPCAddr string` field + default `localhost:8083`
**Strategy:** Either remove the field entirely, or keep it with default `""` (empty string). Since the config struct also has other "connectivity" fields, keeping the field with an empty default is cleaner — no callers need to change if they check `!= ""` before use.

### Location 4: `core/cmd/main.go` lines 72-77
**Content:** Calls `thumbgen.NewClient(ctx, cfg.ThumbGenGRPCAddr)` and fatally exits on error.
**Strategy:** Wrap in a nil-guard:
```go
var thumbgenClient *thumbgen.Client
if cfg.ThumbGenGRPCAddr != "" {
    thumbgenClient, err = thumbgen.NewClient(ctx, cfg.ThumbGenGRPCAddr)
    if err != nil {
        log.Fatal().Err(err).Msg("failed to create thumbgen client")
    }
    defer thumbgenClient.Close()
}
```
Pass `thumbgenClient` (may be nil) to `handler.NewFileHandler`.

### Location 5: `core/internal/infrastructure/grpcclient/thumbgen/stub.go`
**Content:** Imports `thumbgenv1 "github.com/aligh5331/gobox-proto/gen/thumbgen/v1"`. The stub compiles against the proto types that exist in gobox-proto.
**Strategy:** This is a compile dependency on existing proto types. The stub file already works. **No change needed** — keep the stub as-is. The stub is the correct placeholder. The decoupling is achieved by making the *caller* (cmd/main.go, handler/file.go) nil-safe so the stub is never invoked unless the env var is explicitly set.

### Location 6: `core/internal/interface/rest/handler/file.go` lines 9, 22, 104-115
**Content:**
- Import: `thumbgenv1 "github.com/aligh5331/gobox-proto/gen/thumbgen/v1"`
- Field: `thumb *thumbgen.Client`
- Goroutine: `h.thumb.EnqueueJob(...)` — fire-and-forget
**Strategy:** 
- **Compile dependency:** Replace `*thumbgen.Client` with a local interface `Thumbnailer` defined in the same file/package. Move the proto import into the interface definition file.
- **Fire-and-forget goroutine:** Wrap in a nil-guard:
  ```go
  if h.thumb != nil {
      go func() { ... }()
  }
  ```
  This makes the goroutine a no-op when thumbgen is disabled.

### Location 7: (No violation ID — implicit) `gobox-proto/gen/thumbgen/v1/`
**Content:** Generated Go code for the thumbgen service proto.
**Strategy:** Keep. The proto types exist and are a valid compile dependency for the stub. They are auto-generated by `buf generate` and should stay until Phase 5.

---

## Summary

**Total sessions:** 6

**Estimated violations resolved per session:**
| Session | Violations | Touches |
|---------|-----------|---------|
| A (Thumbgen decouple) | 3 coupling locations | core/.env.example, core/docker-compose.yml, core/pkg/config/config.go, core/cmd/main.go, core/internal/interface/rest/handler/file.go |
| B (FileUpload) | 4 (V-FILEUPLOAD-001, -002, -003, -004) | fileupload/cmd/main.go, fileupload/migrations/, fileupload/internal/domain/model/file.go, fileupload/go.mod, fileupload/pkg/logger/ (new) |
| C (Shortener) | 5 (V-SHORTENER-001, -002, -003, -004, -005) | shortener/cmd/main.go, shortener/migrations/, shortener/internal/domain/model/shortlink.go, shortener/internal/application/usecase/ (tests), shortener/pkg/logger/ (new), shortener/go.mod |
| D (Auth) | 1 (V-AUTH-001) | auth/internal/application/usecase/ (new), auth/internal/interface/grpc/server.go |
| E (Core spec+config) | 3 (V-CORE-005, -006, -007) | core/.env.example, core/pkg/config/config.go, GOBOX_SPEC.md (Librarian only) |
| F (Cross-service) | 3 (V-CROSS-001, -002, -003) | fileupload/docker-compose.yml, fileupload/.env.example, auth/docker-compose.yml, auth/.env.example, auth/pkg/config/config.go |

**Sessions that touch gobox-proto or go.work:** **None.** No fix in this plan modifies `gobox-proto/` or `go.work`. The thumbgen proto types in `gobox-proto/gen/thumbgen/v1/` are kept as-is. They are a valid compile dependency for the stub.

**Sessions that touch GOBOX_SPEC.md:** Session E only (V-CORE-005, V-CORE-006). Must be performed by a **Librarian** session, not Builder.

---

Fix plan complete. Ready for Builder.
