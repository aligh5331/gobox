# GoBox Deployment Configuration Audit — ADR Compliance

**Mode:** FORENSIC SPECIALIST
**Date:** 2026-06-30
**Scope:** `docker-compose.yml` (root), `auth/docker-compose.yml`, `core/docker-compose.yml`,
         `fileupload/docker-compose.yml`, `shortener/docker-compose.yml`, `npm/docker-compose.yml`,
         `core/cmd/main.go`, `core/pkg/config/config.go`, `core/internal/interface/rest/handler/share.go`,
         `core/internal/infrastructure/grpcclient/shortener/client.go`,
         `core/internal/infrastructure/grpcclient/thumbgen/stub.go`,
         `shortener/pkg/config/config.go`, all `.env.example` files
**Reference ADRs:** ADR-001 (network segmentation), ADR-002 (Nginx Proxy Manager),
                   ADR-003 (service discovery via env)
**Pre-existing reference:** `audit/violations.md` (V-CORE-009 context),
                           `audit/fix_plan_v2.md` (Session I — V-CORE-009 fix plan)

---

## Check 1 — V-CORE-009 code fix (code vs compose mismatch)

**Result: V-CORE-009 IS FIXED in code. No BLOCKER.**

### Per-address analysis in `core/cmd/main.go`

| Address variable | Line(s) | Dial pattern | log.Fatal on failure? | Nil-guard? |
|-----------------|---------|-------------|----------------------|------------|
| `AUTH_GRPC_ADDR` | 59–63 | Unconditional `grpcclient.NewAuthClient(ctx, cfg.AuthGRPCAddr)` | Yes (line 61) | No — expected; auth is a hard dependency, `service_started` with retry |
| `FILEUPLOAD_GRPC_ADDR` | 66–70 | Unconditional `fileupload.NewClient(ctx, cfg.FileUploadGRPCAddr)` | Yes (line 68) | No — expected; fileupload is a hard dependency |
| `SHORTENER_GRPC_ADDR` | 82–90 | Nil-guard: `if cfg.ShortenerGRPCAddr != "" { ... }` | Yes (line 87), but only if addr is non-empty AND dial fails | **YES** — pattern matches thumbgen (lines 73–80) |
| `THUMBGEN_GRPC_ADDR` | 73–80 | Nil-guard: `if cfg.ThumbGenGRPCAddr != "" { ... }` | Yes (line 77), but only if addr is non-empty AND dial fails | **YES** — ADR-003 Section "Nil-guard rule for ThumbGen" |

### V-CORE-009 specific finding

The fix described in `audit/fix_plan_v2.md` Session I (V-CORE-009) has been **applied**:

1. **`core/cmd/main.go` lines 82–90:** Shortener client now has the nil-guard wrapper (`var shortenerClient *shortener.Client` + `if cfg.ShortenerGRPCAddr != ""`), identical to the thumbgen pattern. No longer an unconditional `log.Fatal()`.

2. **`core/internal/interface/rest/handler/share.go`:** Uses `ShortLinker` interface instead of concrete `*shortener.Client`. All three methods (`CreateShare`, `ListLinks`, `DeleteLink`) have nil-guards at lines 42, 77, 99 returning `503 SHORTENER_DISABLED` when `h.shortener == nil`.

3. **`core/pkg/config/config.go` line 35:** `ShortenerGRPCAddr` defaults to `""` (empty), enabling the nil-guard to skip dialing when the env var is absent.

4. **`core/.env.example` line 9:** Missing the documentation comment requested by `fix_plan_v2.md` Section I Location 1. The comment `# Shortener gRPC address. Leave empty to disable short link functionality.` was supposed to be added above the `SHORTENER_GRPC_ADDR` line but is absent. This is a **documentation-only** gap, not a functional violation.

### Conclusion

V-CORE-009 is resolved at the code level. The `service_started` condition in the root compose file is NOT a cosmetic partial fix for shortener — it is the correct compose-level setting for a service whose Go client already handles nil-addr gracefully. No BLOCKER.

---

## Check 2 — Network membership matrix compliance

**Result: ALL containers match the ADR-001 matrix. 0 violations.**

### Root `docker-compose.yml`

| Container | Expected (ADR-001) | Actual | Match |
|-----------|-------------------|--------|-------|
| nginx-proxy-manager | `net_edge` | `net_edge` | ✓ |
| core | `net_edge`, `net_core_auth`, `net_core_fileupload`, `net_core_shortener` | `net_edge`, `net_core_auth`, `net_core_fileupload`, `net_core_shortener` (+ comment for Phase 5) | ✓ |
| auth | `net_core_auth`, `net_auth_data` | `net_core_auth`, `net_auth_data` | ✓ |
| auth-postgres | `net_auth_data` | `net_auth_data` | ✓ |
| fileupload | `net_core_fileupload`, `net_shortener_fileupload`, `net_fileupload_data` | `net_core_fileupload`, `net_shortener_fileupload`, `net_fileupload_data` | ✓ |
| fileupload-postgres | `net_fileupload_data` | `net_fileupload_data` | ✓ |
| minio | `net_fileupload_data` | `net_fileupload_data` | ✓ |
| shortener | `net_core_shortener`, `net_shortener_fileupload`, `net_shortener_data`, `net_edge` | `net_core_shortener`, `net_shortener_fileupload`, `net_shortener_data`, `net_edge` | ✓ |
| shortener-postgres | `net_shortener_data` | `net_shortener_data` | ✓ |
| shortener-redis | `net_shortener_data` | `net_shortener_data` | ✓ |

### Per-service standalone compose files

| File | Container | Networks | Expected (from root) | Match |
|------|-----------|----------|---------------------|-------|
| `auth/docker-compose.yml` | auth | `net_core_auth`, `net_auth_data` | `net_core_auth`, `net_auth_data` | ✓ |
| `auth/docker-compose.yml` | auth-postgres | `net_auth_data` | `net_auth_data` | ✓ |
| `core/docker-compose.yml` | core | `net_edge`, `net_core_auth`, `net_core_fileupload`, `net_core_shortener` | `net_edge`, `net_core_auth`, `net_core_fileupload`, `net_core_shortener` | ✓ |
| `fileupload/docker-compose.yml` | fileupload | `net_core_fileupload`, `net_shortener_fileupload`, `net_fileupload_data` | `net_core_fileupload`, `net_shortener_fileupload`, `net_fileupload_data` | ✓ |
| `fileupload/docker-compose.yml` | fileupload-postgres | `net_fileupload_data` | `net_fileupload_data` | ✓ |
| `fileupload/docker-compose.yml` | minio | `net_fileupload_data` | `net_fileupload_data` | ✓ |
| `shortener/docker-compose.yml` | shortener | `net_core_shortener`, `net_shortener_fileupload`, `net_shortener_data`, `net_edge` | `net_core_shortener`, `net_shortener_fileupload`, `net_shortener_data`, `net_edge` | ✓ |
| `shortener/docker-compose.yml` | shortener-postgres | `net_shortener_data` | `net_shortener_data` | ✓ |
| `shortener/docker-compose.yml` | shortener-redis | `net_shortener_data` | `net_shortener_data` | ✓ |
| `npm/docker-compose.yml` | nginx-proxy-manager | `net_edge` | `net_edge` | ✓ |

### NPM network membership verification

Both `docker-compose.yml` (root, line 263) and `npm/docker-compose.yml` (line 21) join NPM to **only `net_edge`**. No leftover `net_core_shortener` membership from ADR-002's reasoning-in-progress text. ADR-002's own correction at lines 136–140 is correctly reflected in both files.

**No violations.**

---

## Check 3 — Port-publishing rule compliance

**Result: 0 violations (with documented exception).**

### Rule: Internal-only services must not have `ports:` stanza

The ADR-001 state: "internal: true networks must never have published host ports in the base compose file."

Services whose ONLY network memberships are `internal: true` networks (no `net_edge`):

| Service | Networks (internal only?) | Has `ports:`? | Has `expose:`? | Verdict |
|---------|--------------------------|--------------|---------------|---------|
| auth-postgres | `net_auth_data` only | No | `5432` | ✓ |
| auth | `net_core_auth`, `net_auth_data` (no `net_edge`) | **Yes: `8084:8080`** | `8081` | **Exception** — documented comment at line 39: `# host-published for dev convenience, not for NPM routing` |
| fileupload-postgres | `net_fileupload_data` only | No | `5432` | ✓ |
| minio | `net_fileupload_data` only | No | `9000`, `9001` | ✓ |
| fileupload | `net_core_fileupload`, `net_shortener_fileupload`, `net_fileupload_data` (no `net_edge`) | No | `9090` | ✓ |
| shortener-postgres | `net_shortener_data` only | No | `5432` | ✓ |
| shortener-redis | `net_shortener_data` only | No | `6379` | ✓ |

Services on `net_edge` (explicitly allowed to have published ports):
- **core:** `ports: 3000:8080` on `net_edge` ✓
- **shortener:** `ports: 8082:8082` on `net_edge` ✓
- **nginx-proxy-manager:** `ports: 80:80, 443:443, 81:81` on `net_edge` ✓

### Standalone per-service compose files

Same check applied to each standalone file. The exception for auth's `ports: 8084:8080` carries the same comment in `auth/docker-compose.yml` line 10. Standalone fileupload has no `ports:` (only `expose:`). Standalone shortener has `ports: 8082:8082` but is on `net_edge`. All correct.

**No violations.**

---

## Check 4 — Healthcheck consistency between root and per-service files

**Result: 2 WARNINGs — fileupload and shortener missing healthchecks in their standalone compose files.**

### Comparison table

| Container | Root compose | Per-service standalone | Consistent? |
|-----------|-------------|----------------------|-------------|
| auth-postgres | `pg_isready + psql SELECT 1` | `pg_isready + psql SELECT 1` (auth/) | ✓ |
| auth | `wget http://localhost:8080/health` | `wget http://localhost:8080/health` (auth/) | ✓ |
| fileupload-postgres | `pg_isready + psql SELECT 1` | `pg_isready + psql SELECT 1` (fileupload/) | ✓ |
| minio | `mc ready local` | `mc ready local` (fileupload/) | ✓ |
| **fileupload** | **`ss -tln \| grep -q :9090`** | **NO healthcheck** (fileupload/) | **✗** |
| shortener-postgres | `pg_isready + psql SELECT 1` | `pg_isready + psql SELECT 1` (shortener/) | ✓ |
| shortener-redis | `redis-cli ping` | `redis-cli ping` (shortener/) | ✓ |
| **shortener** | **`wget http://localhost:8082/health`** | **NO healthcheck** (shortener/) | **✗** |
| core | `wget http://localhost:8080/health` | `wget http://localhost:8080/health` (core/) | ✓ |
| nginx-proxy-manager | No healthcheck | No healthcheck (npm/) | ✓ |

### Healthcheck protocol match

All healthcheck test commands match their service's protocol:
- Auth (port 8080 HTTP) → `wget http://localhost:8080/health` ✓
- Core (port 8080 HTTP) → `wget http://localhost:8080/health` ✓
- Shortener (port 8082 HTTP) → `wget http://localhost:8082/health` ✓
- FileUpload (gRPC only, port 9090, no HTTP) → `ss -tln | grep -q :9090` ✓
- All data stores use correct native healthcheck commands ✓

### Significance

The missing healthchecks in the standalone files are only a problem if a downstream compose file (e.g., core's standalone) references `condition: service_healthy` for fileupload or shortener. Currently, the standalone `core/docker-compose.yml` has no `depends_on` for these services (it's expected they exist externally), so the risk is low but inconsistent. The root compose file is the authoritative deployment target.

---

## Check 5 — Dual env-var convention compliance (ADR-003)

**Result: 1 WARNING (missing comment), 0 functional violations.**

### Complete variable catalogue verification

#### Core API variables

| Variable | Compose value (root) | .env.example value | Host port matches? |
|----------|---------------------|-------------------|-------------------|
| `AUTH_GRPC_ADDR` | `auth:8081` | `localhost:8081` | ✓ (auth container exposes 8081 gRPC) |
| `AUTH_HTTP_ADDR` | `http://auth:8080` | `http://localhost:8080` | ✓ (auth container port 8080 HTTP) |
| `FILEUPLOAD_GRPC_ADDR` | `fileupload:9090` | `localhost:9090` | ✓ (fileupload container port 9090 gRPC) |
| `SHORTENER_GRPC_ADDR` | `shortener:9091` | `localhost:9091` | ✓ (shortener container port 9091 gRPC) |
| `THUMBGEN_GRPC_ADDR` | `thumbgen:9092` | `localhost:9092` | ✓ (placeholder — no thumbgen container yet) |

#### Shortener variables

| Variable | Compose value (root) | .env.example value | Host port matches? |
|----------|---------------------|-------------------|-------------------|
| `FILEUPLOAD_GRPC_ADDR` | `fileupload:9090` | `localhost:9090` | ✓ |

#### Shortener config validation

`shortener/pkg/config/config.go` lines 50–52:
```go
if c.FileUploadGRPCAddr == "" {
    return fmt.Errorf("FILEUPLOAD_GRPC_ADDR is required")
}
```

Shortener **does fail at startup** if `FILEUPLOAD_GRPC_ADDR` is empty. Per ADR-003's "fail at startup if a required address variable is empty" rule. ✓

#### Missing documentation comment (WARNING)

`core/.env.example` line 9 has `SHORTENER_GRPC_ADDR=localhost:9091` but is **missing** the documenting comment recommended by `fix_plan_v2.md` Session I Location 1:
```
# Shortener gRPC address. Leave empty to disable short link functionality.
```
The nil-guard in `cmd/main.go` already enables the empty=disabled behavior, but the .env.example does not document this convention. Developers reading `.env.example` would not know they can leave this empty.

---

## Check 6 — Thumbgen absence is still clean

**Result: 0 violations. Thumbgen isolation is intact.**

| Criterion | Finding | Status |
|-----------|---------|--------|
| No compose file defines `net_core_thumbgen` | Confirmed: only comments reference it (root line 280, 240; core line 33) | ✓ |
| No compose file has a `thumbgen` service block | Confirmed: zero thumbgen service entries | ✓ |
| THUMBGEN_GRPC_ADDR env var present | Root: line 219 `THUMBGEN_GRPC_ADDR: "thumbgen:9092"`; core/docker-compose: line 19 same | ✓ |
| `stub.go` implements no-op with no live dial | `NewClient` returns `&Client{enabled: false}, nil` — never calls `grpc.NewClient` (stub.go line 23–28) | ✓ |
| Core's nil-guard in `cmd/main.go` | Lines 73–80: only dials `if cfg.ThumbGenGRPCAddr != ""` | ✓ |

The only observation: `THUMBGEN_GRPC_ADDR` is set to `thumbgen:9092` in both compose files, but no `net_core_thumbgen` network exists. If the env var is non-empty (it is, in compose), core will attempt the nil-guard check — which passes because addr is non-empty — and then call `thumbgen.NewClient()`, which returns a no-op stub successfully without dialing. So there is no DNS resolution failure. This is correct behavior per ADR-003's nil-guard rule and ADR-001's Phase 5 plan.

---

## Violations

---

### V-DEPLOY-001 — fileupload missing healthcheck in standalone compose
- **Service:** fileupload
- **Category:** HealthcheckConsistency
- **Severity:** WARNING
- **File:** fileupload/docker-compose.yml
- **Location:** fileupload service definition (lines 8–31, no healthcheck block)
- **Symptom:** Root `docker-compose.yml` defines a `ss -tln | grep -q :9090` healthcheck for fileupload (line 125–130). The standalone `fileupload/docker-compose.yml` has no healthcheck on the fileupload service. Any compose file using `condition: service_healthy` for a dependency on standalone fileupload will hang indefinitely.
- **ADR ref:** ADR-001 § "Docker Compose structural contract" — per-service compose files should mirror root definitions for consistency.
- **Root cause:** Standalone fileupload was developed for data-store-only health deps (postgres, minio) but never had its own service healthcheck defined.
- **Fix:** Add to `fileupload/docker-compose.yml` fileupload service:
  ```yaml
    healthcheck:
      test: ["CMD-SHELL", "ss -tln | grep -q :9090"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 5s
  ```
- **Cascades to:** None directly (root compose is authoritative), but inconsistent standalone behavior.

---

### V-DEPLOY-002 — shortener missing healthcheck in standalone compose
- **Service:** shortener
- **Category:** HealthcheckConsistency
- **Severity:** WARNING
- **File:** shortener/docker-compose.yml
- **Location:** shortener service definition (lines 9–32, no healthcheck block)
- **Symptom:** Root `docker-compose.yml` defines a `wget http://localhost:8082/health` healthcheck for shortener (lines 193–198). The standalone `shortener/docker-compose.yml` has no healthcheck on the shortener service. Same inconsistency as V-DEPLOY-001.
- **ADR ref:** ADR-001 § "Docker Compose structural contract"
- **Root cause:** Standalone shortener was developed for data-store-only health deps, not for its own healthcheck.
- **Fix:** Add to `shortener/docker-compose.yml` shortener service:
  ```yaml
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8082/health"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 5s
  ```
- **Cascades to:** None directly.

---

### V-DEPLOY-003 — Missing "empty=disabled" comment for SHORTENER_GRPC_ADDR in core/.env.example
- **Service:** core
- **Category:** EnvVarConvention
- **Severity:** WARNING
- **File:** core/.env.example
- **Location:** Line 9
- **Symptom:** `SHORTENER_GRPC_ADDR=localhost:9091` is declared with no comment. The nil-guard in `cmd/main.go` (lines 82–90) supports leaving this empty to disable short link functionality, but this convention is undocumented in `.env.example`. Developers reading the file would not know the empty=disabled behavior exists.
- **ADR ref:** ADR-003 § "Nil-guard rule for ThumbGen" — the convention is implicit but should be documented per the fix plan.
- **Root cause:** The `fix_plan_v2.md` Session I Location 1 documentation change was not applied.
- **Fix:** Add a comment above line 9:
  ```
  # Shortener gRPC address. Leave empty to disable short link functionality.
  SHORTENER_GRPC_ADDR=localhost:9091
  ```
- **Cascades to:** None.

---

## Blocker summary

| ID | Service | Category | File | Fix (one line) |
|----|---------|----------|------|----------------|
| — | — | — | — | No blockers found |

## Warning summary

| ID | Service | Category | File | Fix (one line) |
|----|---------|----------|------|----------------|
| V-DEPLOY-001 | fileupload | HealthcheckConsistency | fileupload/docker-compose.yml | Add `ss -tln` healthcheck to fileupload service in standalone compose |
| V-DEPLOY-002 | shortener | HealthcheckConsistency | shortener/docker-compose.yml | Add `wget` healthcheck to shortener service in standalone compose |
| V-DEPLOY-003 | core | EnvVarConvention | core/.env.example | Add "empty=disabled" comment above `SHORTENER_GRPC_ADDR` |

---

## Summary of checks

| Check | Result |
|-------|--------|
| Check 1 — V-CORE-009 code fix | **FIXED.** Shortener nil-guard applied at all 3 required locations (cmd/main.go, share.go, config.go). `.env.example` comment missing (minor doc gap). No BLOCKER. |
| Check 2 — Network membership matrix | **PASS.** All containers across all 6 compose files match ADR-001 matrix exactly. NPM joins only `net_edge` in both root and standalone. |
| Check 3 — Port-publishing rule | **PASS.** Auth's `8084:8080` is the sole exception, correctly commented. All internal-only services use `expose:` only. |
| Check 4 — Healthcheck consistency | **2 WARNINGS.** fileupload and shortener missing healthchecks in their standalone compose files. |
| Check 5 — Dual env-var convention | **1 WARNING.** Missing documentation comment for SHORTENER_GRPC_ADDR empty=disabled in core/.env.example. All functional requirements met. |
| Check 6 — Thumbgen absence | **PASS.** No net_core_thumbgen, no thumbgen service block, stub is pure no-op. Clean. |

---

Deployment audit complete. **0 blockers, 3 warnings.** Ready for Architect.
