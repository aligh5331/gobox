# GoBox Violation Fix Plan v2 — Regression Check

**Author:** Architect
**Date:** 2026-06-30
**Source:** Regression check appended to `audit/violations.md`
**Scope:** Five new violations only. Session A-F fixes from `fix_plan.md` are already applied and verified.

---

## 1 — Fix Order

The order is constrained by a build/testing dependency: V-FILEUPLOAD-005 and V-SHORTENER-006 must be fixed first because V-CORE-008 cannot even be tested without migrations present in the container images.

| Session | Violations | Description | Depends on |
|---------|-----------|-------------|------------|
| **G** | V-FILEUPLOAD-005, V-SHORTENER-006 | Add `COPY migrations/` to both Dockerfiles | — |
| **H** | V-CORE-008 | Remove redundant `docker-entrypoint-initdb.d` mount from `core/docker-compose.yml` | G (need migrations in image first) |
| **I** | V-CORE-009 | Nil-guard shortener client in core (thumbgen pattern) | — |
| **J** | V-CROSS-004 | Resolve `auth/keys` vs `auth/certs` duplication | — |

### Order rationale

- **Session G first:** Without migrations in the Docker image, the golang-migrate runner (added by Session B and C fixes) will fail at runtime with `file://migrations: no such file or directory`. Session H removes the fallback init script mount, so the migration files MUST be in the image.
- **Session H second:** V-CORE-008 removes the `docker-entrypoint-initdb.d` fallback. This is safe only after Session G ensures migration files are inside the image.
- **Sessions I–J independent:** Can proceed in any order after H, or in parallel. The shortener nil-guard (V-CORE-009) and the certs directory consolidation (V-CROSS-004) have no code dependency on Sessions G–H.

---

## 2 — Violation Details and Fixes

---

### Session G: V-FILEUPLOAD-005 + V-SHORTENER-006 — Dockerfile missing migrations

**Violation IDs:** V-FILEUPLOAD-005, V-SHORTENER-006
**Services:** fileupload, shortener
**Files touched:** `fileupload/Dockerfile`, `shortener/Dockerfile`
**Builder sessions:** One session, two files, no interdependency between them.

#### V-FILEUPLOAD-005 — fileupload/Dockerfile

**Current content (line 9):**
```dockerfile
COPY . .
```

**Problem:** The `COPY . .` copies source code including `migrations/`, but this is the *build stage*. The runtime stage (`FROM alpine:3.21`) only copies the binary:
```dockerfile
COPY --from=builder /fileupload /usr/local/bin/fileupload
```

The `migrations/` directory is never copied into the runtime stage. When `fileupload/cmd/main.go` calls `runMigrations("file://migrations")`, the files won't exist at runtime.

**Fix — add to runtime stage, after the binary copy (line 19):**
```dockerfile
COPY --from=builder /app/migrations ./migrations
```

The full runtime stage becomes:
```dockerfile
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /fileupload /usr/local/bin/fileupload
COPY --from=builder /app/migrations ./migrations
EXPOSE 9090
ENTRYPOINT ["fileupload"]
```

**Verification:** `docker build -t fileupload -f fileupload/Dockerfile fileupload/ && docker run --rm fileupload ls migrations/` should show `001_create_files.up.sql`.

---

#### V-SHORTENER-006 — shortener/Dockerfile

**Current content (line 9):**
```dockerfile
COPY . .
```

Same problem as V-FILEUPLOAD-005. The runtime stage only has the binary and no `migrations/` directory.

**Fix — add to runtime stage, after the binary copy (line 19):**
```dockerfile
COPY --from=builder /app/migrations ./migrations
```

The full runtime stage becomes:
```dockerfile
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /shortener /usr/local/bin/shortener
COPY --from=builder /app/migrations ./migrations
EXPOSE 9091 8082
ENTRYPOINT ["shortener"]
```

**Verification:** `docker build -t shortener -f shortener/Dockerfile shortener/ && docker run --rm shortener ls migrations/` should show `001_create_short_links.up.sql`.

#### Cascade analysis

- **No cascade to other services:** These are Dockerfile-only changes. No other service references `fileupload/Dockerfile` or `shortener/Dockerfile` (the `core/docker-compose.yml` references the `../fileupload` build context).
- **No cascade to compose:** The `COPY` uses a relative path `./migrations` which resolves correctly because the Dockerfile WORKDIR is `/app` and migrations are at the service root relative to the build context.
- **No cascade to golang-migrate:** The migration runner in both services uses `"file://migrations"` which resolves relative to the working directory. Since alpine's `WORKDIR` defaults to `/`, and we copy to `./migrations`, the path resolves to `/migrations`. The `file://migrations` URL in the Go code resolves relative to the process working directory, which is `/` (not set in Dockerfile, so default). **This path must be explicitly verified.** See risk note below.

#### Risk: migration source path resolution

The `runMigrations` function in both services uses `"file://migrations"`. In Docker, the working directory of an `ENTRYPOINT ["fileupload"]` (no explicit `WORKDIR`) is `/`. So `file://migrations` resolves to `/migrations`, which is incorrect because we copy to `./migrations` (which is `/app/migrations` relative to `WORKDIR /app` at build time, but at runtime lands in `/migrations` only if the Dockerfile sets `WORKDIR /`).

**Pre-emptive verification needed:** Confirm whether the `runMigrations` source URL should be `"file:///migrations"` (absolute from container root) or whether a `WORKDIR /app` should be added to the runtime stage. The safest approach: match the thumbgen/fileupload pattern by adding `WORKDIR /app` to the runtime stage and copying to `./migrations` (so they land at `/app/migrations`). Then the `file://migrations` relative URL resolves correctly from `/app`.

**Recommended approach for both Dockerfiles:** Add `WORKDIR /app` to the runtime stage (matching `core/Dockerfile` which already has `WORKDIR /app`). The copy to `./migrations` then lands at `/app/migrations`, and the golang-migrate `file://migrations` resolves relative to `/app`.

---

### Session H: V-CORE-008 — Redundant docker-entrypoint-initdb.d mount

**Violation ID:** V-CORE-008
**Service:** core (docker-compose only)
**File:** `core/docker-compose.yml`, line 62

**Current content (lines 60-62 of fileupload-postgres service):**
```yaml
volumes:
  - fileupload_pgdata:/var/lib/postgresql/data
  - ../fileupload/migrations:/docker-entrypoint-initdb.d
```

**Problem:** The `../fileupload/migrations:/docker-entrypoint-initdb.d` volume mount was a fallback that copied SQL files into Postgres's one-time bootstrap mechanism during container initialization. This was useful before Session B added the golang-migrate runner. Now that `fileupload/cmd/main.go` calls `runMigrations()` at startup (which runs `migrate.Up()` via the `file://migrations` source), the init script mount is redundant.

#### Consumer analysis

**Does anything else rely on this mount?** I checked all references to `docker-entrypoint-initdb.d`, `fileupload/migrations`, and the `fileupload-postgres` service in the entire codebase:

| Potential consumer | Found? | Impact |
|---|---|---|
| Postgres healthcheck (`pg_isready -U fileupload`) | Yes, line 56-59 | Only checks Postgres is accepting connections, not whether schema exists. Unaffected by removing the mount. |
| fileupload service startup order | Yes, `depends_on: fileupload-postgres: condition: service_healthy` | fileupload waits for Postgres health, then runs `migrate.Up()`. The init script is redundant. |
| Seed-data scripts in `fileupload/migrations/` | No | Only `.up.sql` migration files exist. No seed data scripts. |
| Other services reading `fileupload/migrations/` | No | No other volume or service references this path. |
| `shortener/docker-compose.yml` or `fileupload/docker-compose.yml` | No | Those compose files use `./migrations:/docker-entrypoint-initdb.d` (their own migrations), not fileupload's. |

**Conclusion:** No other consumer. The mount is safe to remove.

**Fix — remove the mount line (line 62):**
```yaml
volumes:
  - fileupload_pgdata:/var/lib/postgresql/data
```

Remove line 62: `- ../fileupload/migrations:/docker-entrypoint-initdb.d`

**Verification:** After the fix, run `docker compose -f core/docker-compose.yml up -d fileupload-postgres` then wait for fileupload to start. `docker logs fileupload` should show `"database migrations completed"` without errors.

#### Cascade analysis

- **No cascade to other services:** The mount is internal to `core/docker-compose.yml`. No other compose file or service references this specific mount.
- **No cascade to fileupload:** The fileupload service runs `migrate.Up()` via its own `runMigrations` function. Removing the init script mount does not affect fileupload's migration logic.
- **No cascade to shortener:** Shortener's own `docker-entrypoint-initdb.d` mount (in `shortener/docker-compose.yml`) is separate and unrelated.
- **First-time DB creation:** The only observable change is that on a fresh database (empty volume), the schema will be created by `runMigrations()` (via gRPC startup) instead of by the Postgres init script (during container bootstrap). The end result is identical: the `files` table exists before any API requests arrive, because fileupload's health check in `core/docker-compose.yml` requires the service to start, and `runMigrations()` is called before the gRPC server starts listening.

---

### Session I: V-CORE-009 — Shortener client not nil-guarded (thumbgen pattern)

**Violation ID:** V-CORE-009
**Service:** core
**Files touched:**
- `core/cmd/main.go`
- `core/internal/interface/rest/handler/share.go`
- `core/pkg/config/config.go` (may need default change)
- `core/.env.example` (may need comment update)

**Pattern reference:** Session A of the original `fix_plan.md` defined a 6-location thumbgen decoupling strategy. The identical pattern is applied here for the shortener client. Below is the location-by-location mapping.

---

#### Context: Current state vs thumbgen reference

The thumbgen nil-guard was already applied (Session A fixes). Its structure is:

| Location | Thumbgen (applied) | Shortener (current — needs fix) |
|----------|-------------------|----------------------------------|
| 1. `.env.example` | Line removed | Line 9: `SHORTENER_GRPC_ADDR=shortener:9091` — keep but document empty=disabled |
| 2. `docker-compose.yml` | Env var removed | Line 11: `SHORTENER_GRPC_ADDR: "shortener:9091"` — keep for now; may be documented as optional |
| 3. `config.go` | Default blanked to `""` | Line 35: `getEnv("SHORTENER_GRPC_ADDR", "localhost:9091")` — keep default; nil-guard at usage site |
| 4. `cmd/main.go` | Nil-guard wrapped | Lines 83-87: unconditional dial + fatal on error — **must add nil-guard** |
| 5. grpcclient package | Stub kept as-is | `client.go` — keep as-is; the concrete type is used internally |
| 6. Handler | Interface + nil-guard | `share.go` — concrete `*shortener.Client`, no interface, no nil-check — **must add interface + nil-guard** |

---

#### Location 1: `core/.env.example` line 9

**Current:** `SHORTENER_GRPC_ADDR=shortener:9091`

**Strategy:** Keep the line but add a comment above it documenting the empty=disabled convention. This mirrors the thumbgen approach where the env var was removed entirely (thumbgen was pure stub; shortener is a real service that is typically present). The comment makes the convention discoverable.

**Fix:**
```
# Shortener gRPC address. Leave empty to disable short link functionality.
SHORTENER_GRPC_ADDR=shortener:9091
```

---

#### Location 2: `core/docker-compose.yml` line 11

**Current:** `SHORTENER_GRPC_ADDR: "shortener:9091"`

**Strategy:** Keep as-is. The compose file deploys the shortener service, so the address should be populated. The nil-guard is for resilience and for local-dev scenarios where shortener may not be running. Adding a comment is sufficient.

---

#### Location 3: `core/pkg/config/config.go` line 35

**Current:** `ShortenerGRPCAddr: getEnv("SHORTENER_GRPC_ADDR", "localhost:9091")`

**Strategy:** Keep default of `"localhost:9091"`. Unlike thumbgen (which defaults to `""`), shortener is a real service that most deployments will use. The nil-guard at the call site handles the empty case. No config struct change needed.

---

#### Location 4: `core/cmd/main.go` lines 82-87

**Current (unconditional dial):**
```go
// Dial Shortener gRPC.
shortenerClient, err := shortener.NewClient(ctx, cfg.ShortenerGRPCAddr)
if err != nil {
    log.Fatal().Err(err).Msg("failed to connect to shortener gRPC")
}
defer shortenerClient.Close()
```

**Fix (nil-guard, identical structure to thumbgen at lines 72-80):**
```go
// Create Shortener client (nil-safe — only dials if addr is configured).
var shortenerClient *shortener.Client
if cfg.ShortenerGRPCAddr != "" {
    shortenerClient, err = shortener.NewClient(ctx, cfg.ShortenerGRPCAddr)
    if err != nil {
        log.Fatal().Err(err).Msg("failed to create shortener client")
    }
    defer shortenerClient.Close()
}
```

**Note:** This requires `err` to be in scope before the block. In `cmd/main.go`, `err` is used elsewhere (lines 48, 59, 66). The variable is already declared via `:=` in other client creation blocks. The nil-guard block mirrors the existing thumbgen block exactly — `var` declaration for the client, `if addr != "" { ... }`, then pass to constructor.

---

#### Location 5: `core/internal/infrastructure/grpcclient/shortener/client.go`

**Strategy:** Keep as-is. The `Client` struct already has a compile-time interface check (lines 87-92) proving it satisfies the `CreateLink`/`ListLinks`/`DeleteLink` interface. No changes needed here.

---

#### Location 6: `core/internal/interface/rest/handler/share.go`

**Current (concrete type, no interface):**
```go
import (
    shortenerv1 "github.com/aligh5331/gobox-proto/gen/shortener/v1"
    "github.com/aligh5331/gobox/core/internal/infrastructure/grpcclient/shortener"
)

type ShareHandler struct {
    shortener *shortener.Client
}

func NewShareHandler(shortener *shortener.Client) *ShareHandler {
    return &ShareHandler{shortener: shortener}
}
```

**Fix (interface shielding, identical pattern to `file.go`'s `Thumbnailer`):**

Define a `ShortLinker` interface in `share.go`:
```go
// ShortLinker is the interface for short link operations.
// It shields ShareHandler from the concrete shortener client dependency.
type ShortLinker interface {
    CreateLink(ctx context.Context, req *shortenerv1.CreateLinkRequest) (*shortenerv1.CreateLinkResponse, error)
    ListLinks(ctx context.Context, req *shortenerv1.ListLinksRequest) (*shortenerv1.ListLinksResponse, error)
    DeleteLink(ctx context.Context, req *shortenerv1.DeleteLinkRequest) (*emptypb.Empty, error)
}
```

Change the struct field and constructor:
```go
type ShareHandler struct {
    shortener ShortLinker
}

func NewShareHandler(shortener ShortLinker) *ShareHandler {
    return &ShareHandler{shortener: shortener}
}
```

Add nil-guards in each method. The pattern is consistent with the thumbgen approach: nil-check before calling the interface method. Use a pattern that returns an appropriate error when the service is disabled:

```go
func (h *ShareHandler) CreateShare(c echo.Context) error {
    if h.shortener == nil {
        return middleware.NewHTTPError(http.StatusServiceUnavailable, "SHORTENER_DISABLED", "short link service is not configured")
    }
    // ... rest of method unchanged
}
```

Same nil-guard for `ListLinks` and `DeleteLink`.

**Imports to add:** `"context"`, `google.golang.org/protobuf/types/known/emptypb"` (for `*emptypb.Empty` in the interface).

**Imports to remove:** The direct `shortener` client import can be removed from `share.go` since it's now accessed through the interface. However, the `shortenerv1` proto import must stay for the request/response types in the interface method signatures.

---

#### Cascade analysis for V-CORE-009

| What changes | Cascade risk | Mitigation |
|---|---|---|
| `share.go` changes from concrete `*shortener.Client` to `ShortLinker` interface | `router.go` passes `*shortener.Client` to `NewShareHandler` — the concrete type satisfies the interface via compile-time check | No change needed in `router.go` — Go interface satisfaction is automatic |
| `cmd/main.go` passes `shortenerClient` (now possibly nil) to `NewShareHandler` | Previously was always non-nil (fatal on dial error) | Handler now nil-guards each method. Passing nil is safe. |
| `.env.example` comment | None | Documentation-only change |
| `config.go` default | None | No default changed |
| `docker-compose.yml` | None | No compose changes |
| Proto import in share.go | `shortenerv1` proto import remains | Only the concrete `shortener.Client` import is removed; proto types stay |

**Verification:** `core go vet ./...` must pass. `go build ./cmd/` must succeed. No changes to `gobox-proto/` or `go.work`.

---

### Session J: V-CROSS-004 — auth/keys vs auth/certs duplication

**Violation ID:** V-CROSS-004
**Service:** auth (cross-service concern)
**Files touched:** `auth/docker-compose.yml`, `auth/.env.example`, potentially `auth/certs/` and `auth/keys/`

**Problem:** Two directories exist at `auth/keys/` and `auth/certs/` containing identical PEM files (`private.pem`, `public.pem`). The docker-compose mounts `./keys:/app/keys:ro`, and the `.env.example` references `JWT_PRIVATE_KEY_PATH=./private.pem` (relative to working directory). This duplication creates confusion about which directory is canonical and risks one set of keys falling out of sync with the other.

---

#### Option A: Change mount to `./certs` (consolidate on `certs/`)

**Steps:**
1. Remove the `auth/keys/` directory (or keep as a symlink to `certs/` for backward compat).
2. In `auth/docker-compose.yml` line 14, change:
   ```yaml
   volumes:
     - ./certs:/app/keys:ro
   ```
3. In `auth/.env.example` line 10, update the comment to reference `certs/`:
   ```
   JWT_PRIVATE_KEY_PATH=/app/keys/private.pem
   ```
   (The container path `/app/keys` stays the same; only the host-side mount source changes.)

**Tradeoffs:**
- ✅ Eliminates duplicate directories.
- ✅ Uses the more standard name `certs/` for cryptographic material.
- ⚠️ Backward compatibility: existing `docker compose` invocations using `./keys` bind-mount continue to work if `keys/` is kept as a symlink or the file is kept until the next cleanup pass.
- ⚠️ If someone has `keys/` in their `.gitignore` or local workflow, they'd need to update to `certs/`.
- ❌ Does not solve the "keys must exist before first run" problem — a fresh clone still requires generating keys.

---

#### Option B: Add a key-generation step (auto-gen if missing)

**Steps:**
1. Create `auth/entrypoint.sh` that checks for key files at `JWT_PRIVATE_KEY_PATH` and generates them with `openssl genrsa` if missing.
2. Update `auth/Dockerfile` to copy `entrypoint.sh` and use it as `ENTRYPOINT`.
3. Keep either `keys/` or `certs/` as the canonical directory — remove the duplicate.
4. The docker-compose mount remains as-is (`./keys:/app/keys:ro`), but the entrypoint script makes it optional (if the mount is missing, keys are generated inside the container).

**Tradeoffs:**
- ✅ Zero friction on fresh clone — no need to run `openssl genrsa` manually or keep PEM files in version control.
- ✅ Can keep `keys/` or `certs/` as the canonical directory (choose one, remove the other).
- ⚠️ Adds an `entrypoint.sh` and a build-time dependency on `openssl` in the Docker image.
- ⚠️ Generated keys are lost when the container is recreated (ephemeral). For production, a persistent mount is still required.
- ⚠️ More complex than Option A. Requires modifying the Dockerfile and adding a shell script.
- ⚠️ The entrypoint script must handle the case where one key exists but the other doesn't.

---

#### Decision required

| Aspect | Option A (change mount) | Option B (key-gen step) |
|--------|------------------------|------------------------|
| Effort | 1 file edit (compose) + directory cleanup | 3+ files (entrypoint.sh, Dockerfile, compose, .env.example) |
| Friction on fresh clone | Still need to generate keys manually | Zero — auto-gen |
| Production readiness | Requires key management out of band | Same — keys are ephemeral without mount |
| Security | Keys in bind-mount, controlled by host | Generated in container; may need volume to persist |
| Consistency with rest of repo | Matches the pattern of other services (pre-placed config files) | New pattern not used elsewhere |

**Recommendation:** Flag this for user decision. The two options are not mutually exclusive — they can be applied together: consolidate on `certs/` (Option A) AND add an auto-generation entrypoint (Option B) for the "happy path" of local development.

**Minimal fix (if user chooses Option A only):**
1. Remove `auth/keys/` directory (or deprecate).
2. Change `auth/docker-compose.yml` line 14 from `./keys:/app/keys:ro` to `./certs:/app/keys:ro`.
3. No change to `.env.example` — the container path `/app/keys/private.pem` stays consistent.
4. Update `.env.example` comment if desired.

**Minimal fix (if user chooses Option B only):**
1. Add `auth/entrypoint.sh` with key generation.
2. Update `auth/Dockerfile` to include entrypoint.
3. Remove one of `keys/` or `certs/` (pick canonical name).
4. Keep docker-compose mount path as-is or update.

#### Cascade analysis

- **No cascade to core:** Core reads auth's JWKS endpoint via HTTP (`AUTH_HTTP_ADDR`), not from a shared key file. No other service mounts `auth/keys/` or `auth/certs/`.
- **No cascade to testdata:** `auth/testdata/private.pem` is a separate file used only by unit tests. Not affected.
- **No cascade to other compose files:** No other docker-compose file references `auth/keys/` or `auth/certs/`.

---

## 3 — Summary Table

| Session | Violation ID | Service | File(s) | Fix summary | Dependencies |
|---------|-------------|---------|---------|-------------|-------------|
| G.1 | V-FILEUPLOAD-005 | fileupload | `fileupload/Dockerfile` | `COPY --from=builder /app/migrations ./migrations` in runtime stage | Must fix before H |
| G.2 | V-SHORTENER-006 | shortener | `shortener/Dockerfile` | Same COPY line in runtime stage | Must fix before H |
| H | V-CORE-008 | core | `core/docker-compose.yml` | Remove `../fileupload/migrations:/docker-entrypoint-initdb.d` mount | G must be done first |
| I | V-CORE-009 | core | `core/cmd/main.go`, `core/internal/interface/rest/handler/share.go`, `core/.env.example` | Nil-guard shortener client (thumbgen pattern): interface in share.go, nil-check in share.go methods, conditional dial in cmd/main.go | None |
| J | V-CROSS-004 | auth | `auth/docker-compose.yml`, `auth/.env.example`, `auth/keys/`, `auth/certs/` | Either change mount to `./certs` (Option A) or add key-generation entrypoint (Option B) — **requires user confirmation** | None |

---

## 4 — Pre-emptive Cascade Notes

### From Session G to H
- Removing the `docker-entrypoint-initdb.d` mount in H will cause Postgres to start without schema on a fresh volume. The `runMigrations()` in fileupload's startup will create it moments later. The healthcheck ordering (`depends_on: fileupload-postgres: condition: service_healthy` → fileupload starts → `migrate.Up()` → gRPC starts) ensures the schema exists before any API route can be hit. No race condition.

### From Session I to existing code
- The `ShortLinker` interface in `handler/share.go` must match the method signatures exposed by `shortener.Client`. The compile-time interface check at the bottom of `client.go` already proves the client satisfies `CreateLink`, `ListLinks`, and `DeleteLink`. No signature mismatch risk.
- `router.go` passes `shortenerClient` (now `*shortener.Client` which satisfies `ShortLinker`) to `NewShareHandler`. No change needed in `router.go`.

### From Session J to auth tests
- If Option A is chosen (change mount to `./certs`), update `auth/testdata/` path references if any test reads from `keys/`. Check `auth/internal/interface/grpc/server_test.go` for hardcoded paths. This is a low-risk separation — the test file already uses `testdata/private.pem`, not `keys/private.pem`.

---

## 5 — Risks and Open Issues

1. **Migration source path resolution (Session G):** The `file://migrations` URL in both `runMigrations` functions resolves relative to the process working directory. In the Docker runtime stage, the default working directory is `/` (alpine base image). The `COPY ./migrations` on line 14 (if added after `COPY --from=builder`) lands at `/migrations`. The relative URL `file://migrations` then resolves to `/migrations` which is correct. **No action needed** as long as the runtime stage has no explicit `WORKDIR` directive. However, adding `WORKDIR /app` (matching `core/Dockerfile`'s pattern) would be more consistent and would require updating the source URL to `file:///app/migrations`. **Flag for Builder to confirm during Session G.**

2. **V-CROSS-004 requires user confirmation:** Both options have valid tradeoffs. The Builder must not proceed without explicit user choice.

3. **No changes to `gobox-proto/` or `go.work`:** None of the five violations touch proto definitions, generated code, or workspace configuration.

---

Fix plan v2 complete. Ready for Builder.
