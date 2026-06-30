# ADR-003: Service Discovery via Environment Variables

## Decision

**Every address that one GoBox service uses to reach another service (gRPC
or HTTP) is injected exclusively through an environment variable.** No
hardcoded hostnames, IPs, or ports appear in source code, Dockerfiles, or
default configuration structs.

## Context

GOBOX_SPEC.md §7 (Cross-cutting concerns / Configuration) states:

> Every service reads config from environment variables. `.env.example`
> documents all vars. No config files in the binary — 12-factor.

The spec's config table lists these cross-service address variables:

| Var | Used by | Spec reference |
|-----|---------|---------------|
| `AUTH_GRPC_ADDR` | core | §7 table |
| `FILEUPLOAD_GRPC_ADDR` | core, thumbgen | §7 table |
| `SHORTENER_GRPC_ADDR` | core | §7 table |
| `THUMBGEN_GRPC_ADDR` | core | §7 table |

However, GOBOX_SPEC.md §5.4 (redirect flow) also requires shortener to call
FileUpload's `GetDownloadURL` gRPC. The spec table omits shortener from
`FILEUPLOAD_GRPC_ADDR`. The shortener's current `docker-compose.yml` already
sets `FILEUPLOAD_GRPC_ADDR: fileupload:9090`, confirming the real
requirement. This ADR documents the corrected list.

## Options considered

### Option 1: Environment variables only (chosen)

Every cross-service address is an env var. The `.env.example` file documents
defaults for local development. The `docker-compose.yml` sets the container
address for production.

- **Pros:** 12-factor compliant. Works identically in Docker and local dev.
  No service discovery infrastructure needed. Trivially debuggable (`env |
  grep _ADDR`). Changing an address requires a container restart, not a code
  rebuild.
- **Cons:** Manual address configuration per environment (dev, staging,
  prod). No dynamic DNS or service registry for auto-scaling. Acceptable for
  v1 — GoBox is a fixed set of five services, not an auto-scaled cluster.

### Option 2: Docker DNS hostnames hardcoded in source

Services use fixed Docker hostnames (e.g., `"auth:8081"`) directly in Go
source code or default config values.

- **Pros:** Fewer env vars to configure. Works without `.env.example`.
- **Cons:** Breaks local development (no Docker DNS when running `go run
  ./cmd`). Makes the service Docker-dependent at the code level. Violates
  12-factor. The same binary cannot run in Docker and natively without
  recompilation.

### Option 3: Centralised service registry (Consul / etcd)

All services register themselves and discover peers via a central registry
at startup.

- **Pros:** Dynamic. Supports health-based routing. Industry standard for
  large microservice deployments.
- **Cons:** Adds an entire infrastructure dependency (Consul cluster) for a
  system with five fixed services. Unnecessary operational complexity.
  Violates the "minimal deps" constraint from AGENTS.md.

## Chosen approach

**Option 1 — Environment variables only.**

### Complete variable catalogue

Every cross-service address env var, sorted by the service that reads it:

#### Core API

| Variable | Target | Compose value | `.env.example` value |
|----------|--------|---------------|---------------------|
| `AUTH_GRPC_ADDR` | Auth gRPC | `auth:8081` | `localhost:8081` |
| `AUTH_HTTP_ADDR` | Auth HTTP (JWKS) | `http://auth:8080` | `http://localhost:8080` |
| `FILEUPLOAD_GRPC_ADDR` | FileUpload gRPC | `fileupload:9090` | `localhost:9090` |
| `SHORTENER_GRPC_ADDR` | Shortener gRPC | `shortener:9091` | `localhost:9091` |
| `THUMBGEN_GRPC_ADDR` | ThumbGen gRPC | `thumbgen:9092` | `localhost:9092` |

#### Shortener

| Variable | Target | Compose value | `.env.example` value |
|----------|--------|---------------|---------------------|
| `FILEUPLOAD_GRPC_ADDR` | FileUpload gRPC | `fileupload:9090` | `localhost:9090` |

#### Auth

Auth has **zero** outbound service dependencies. It holds no `*_GRPC_ADDR`
or `*_HTTP_ADDR` variables pointing to another GoBox service.

#### FileUpload

FileUpload has **zero** outbound service dependencies. It holds no
`*_GRPC_ADDR` or `*_HTTP_ADDR` variables pointing to another GoBox service.

### Dual convention rationale

The compose value and `.env.example` value differ deliberately:

- **Compose value:** Uses the Docker container name as the hostname (e.g.,
  `auth:8081`). Docker Compose creates a DNS entry for each container on
  every network the container joins. The container name is the canonical
  hostname. This requires no extra DNS configuration.

- **`.env.example` value:** Uses `localhost` with the host-mapped port (e.g.,
  `localhost:8081`). When a developer runs `go run ./cmd` outside Docker,
  there is no Docker DNS. The dependent services must be running locally (or
  in Docker with published ports) and reachable at `localhost:{port}`.

**This is not a configuration drift.** The two files serve different
runtimes: Docker (compose) and native (`.env.example`). A developer running
`docker compose up` inside core/ uses the compose env; a developer running
`go run ./cmd` inside core/ sources `.env.example`. Having two values is
correct — what matters is that the **variable name** is identical between
the two files.

### Config loading rule

Every Go service must load these variables using the same pattern:

```go
// pkg/config/config.go (or equivalent per service)
type Config struct {
    AuthGRPCAddr       string `env:"AUTH_GRPC_ADDR"`        // core, shortener
    FileuploadGRPCAddr string `env:"FILEUPLOAD_GRPC_ADDR"`  // core, shortener
    ShortenerGRPCAddr  string `env:"SHORTENER_GRPC_ADDR"`   // core only
    ThumbgenGRPCAddr   string `env:"THUMBGEN_GRPC_ADDR"`    // core only
    // ...
}
```

- Services must **fail at startup** if a required address variable is empty.
  Core requires `AUTH_GRPC_ADDR` to be non-empty (it is needed for JWKS
  fetch at boot). All `*_GRPC_ADDR` variables are required — except
  `THUMBGEN_GRPC_ADDR`, which is handled specially (see nil-guard rule
  below).
- No variable has a default value in code for addresses. Defaults are only
  in `.env.example` and `docker-compose.yml`.
- The `envconfig` or `caarlos0/env` pattern is not mandated — each service
  can use `os.Getenv` or a struct-tag library. The contract is the variable
  name and the "fail on empty" behavior.

### Nil-guard rule for ThumbGen

`THUMBGEN_GRPC_ADDR` is **required** to be set in the environment (the
variable must exist and be non-empty), but the ThumbGen gRPC client **may
be a no-op stub** that never opens a gRPC connection.

Core's existing implementation at
`core/internal/infrastructure/grpcclient/thumbgen/stub.go` demonstrates this
pattern:

```go
func NewClient(_ context.Context, addr string) (*Client, error) {
    if addr == "" {
        return nil, fmt.Errorf("grpcclient/thumbgen: addr must not be empty")
    }
    return &Client{enabled: false}, nil
}
```

The stub:
1. Validates that `THUMBGEN_GRPC_ADDR` is present (fail-fast on config
   error).
2. Returns a working client that logs calls instead of making them.
3. Allows core to compile and run without the ThumbGen service or its
   network (`net_core_thumbgen`).

This pattern is mandatory for any future "optional" service dependency: the
env var is always declared, the client always compiles, and the runtime
behavior degrades gracefully when the target is absent.

### Verification: every cross-service invocation must trace to an env var

| RPC call | Called by | Env var | Network |
|----------|-----------|---------|---------|
| Auth.GetUser | core | `AUTH_GRPC_ADDR` | `net_core_auth` |
| Auth.ValidateSession | core | `AUTH_GRPC_ADDR` | `net_core_auth` |
| Auth.GetPublicKey | core | `AUTH_GRPC_ADDR` | `net_core_auth` |
| FileUpload.InitiateUpload | core | `FILEUPLOAD_GRPC_ADDR` | `net_core_fileupload` |
| FileUpload.ConfirmUpload | core | `FILEUPLOAD_GRPC_ADDR` | `net_core_fileupload` |
| FileUpload.GetFile | core | `FILEUPLOAD_GRPC_ADDR` | `net_core_fileupload` |
| FileUpload.ListFiles | core | `FILEUPLOAD_GRPC_ADDR` | `net_core_fileupload` |
| FileUpload.DeleteFile | core | `FILEUPLOAD_GRPC_ADDR` | `net_core_fileupload` |
| FileUpload.GetDownloadURL | core | `FILEUPLOAD_GRPC_ADDR` | `net_core_fileupload` |
| Shortener.CreateLink | core | `SHORTENER_GRPC_ADDR` | `net_core_shortener` |
| Shortener.GetLink | core | `SHORTENER_GRPC_ADDR` | `net_core_shortener` |
| Shortener.DeleteLink | core | `SHORTENER_GRPC_ADDR` | `net_core_shortener` |
| Shortener.ListLinks | core | `SHORTENER_GRPC_ADDR` | `net_core_shortener` |
| ThumbGen.EnqueueJob | core | `THUMBGEN_GRPC_ADDR` | `net_core_thumbgen` |
| FileUpload.GetDownloadURL | shortener | `FILEUPLOAD_GRPC_ADDR` | `net_shortener_fileupload` |

Every row in this table is a **contract**: if the env var is missing at
startup, the service must fail. If the network is missing, the RPC will fail
at runtime with a DNS resolution error.

### Spec discrepancy note

GOBOX_SPEC.md §7 (cross-cutting concerns) lists `FILEUPLOAD_GRPC_ADDR` as
used by `core, thumbgen`. This ADR declares an additional consumer:
**shortener**. The spec's redirect flow in §5.4 requires shortener to call
`FileUpload.GetDownloadURL` on every redirect. The shortener's
`docker-compose.yml` already sets `FILEUPLOAD_GRPC_ADDR`, confirming this
is the correct state. The spec table should be updated in the next revision
to include shortener.

## Constraints and risks

- **Env var sprawl:** Core alone requires five address variables plus
  service-specific vars (database URLs, S3 endpoint, etc.). Mitigation:
  acceptable — no more than 15 env vars per service. A config struct in
  `pkg/config/` groups them with struct tags. No env var is optional
  (except implicitly via the ThumbGen nil-guard pattern).
- **Host-port mismatch:** If a developer runs `docker compose up` with
  `.env.example` values (localhost), the service will fail because the
  target container's published port is mapped to a different host port (e.g.,
  auth's gRPC port `8081:8081` maps to host port 8081, but the container
  DNS expects `auth:8081`). Mitigation: the compose file sets env vars
  via `environment:` (not `env_file:`), overriding any `.env.example`
  values. The `.env.example` is only used when running `go run ./cmd`
  directly.
- **No dynamic reconfiguration:** Changing an env var requires a container
  restart. Acceptable for v1 — no hot-reload requirement exists in the spec.
- **Secret leakage:** Address variables are not secrets (they contain no
  credentials), but they do reveal network topology. Production deployments
  may set them via a secret manager. The env-var interface makes this
  straightforward — override the var at container start.
