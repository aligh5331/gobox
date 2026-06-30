# ADR-001: Network Segmentation for GoBox

## Decision

Replace the single flat `gobox` network with **one named network per
service-to-service relationship**. Every Docker Compose file in the project
must follow this network topology as a contract.

Nine networks are defined:

| Network | Internal | Members | Purpose |
|---------|----------|---------|---------|
| `net_edge` | **false** | nginx-proxy-manager, core (HTTP), shortener (HTTP) | Public ingress — the only network with external internet access |
| `net_core_auth` | true | core, auth | gRPC: core calls auth |
| `net_core_fileupload` | true | core, fileupload | gRPC: core calls fileupload |
| `net_core_shortener` | true | core, shortener | gRPC: core calls shortener |
| `net_shortener_fileupload` | true | shortener, fileupload | gRPC: shortener calls fileupload (`GetDownloadURL` on redirect) |
| `net_core_thumbgen` | true | core, thumbgen | gRPC: core calls thumbgen — **WILL EXIST in Phase 5**; do not create before then |
| `net_auth_data` | true | auth, auth-postgres | Auth's database |
| `net_fileupload_data` | true | fileupload, fileupload-postgres, minio | FileUpload's database and S3/MinIO |
| `net_shortener_data` | true | shortener, shortener-postgres, shortener-redis | Shortener's database and Redis cache |

## Context

As of this writing, every GoBox service uses a single flat `gobox` network
(`external: true`). All containers can reach each other. This works for
development but violates the security principle of least privilege: a
compromised fileupload container could, in the current setup, probe the auth
Postgres or the shortener Redis directly.

The spec (§2) defines five independent services. The communication diagram
(derived from §3, §5.3, §5.4) shows exactly which services talk to which:

- Core calls Auth, FileUpload, Shortener, and ThumbGen (all via gRPC)
- Shortener calls FileUpload (`GetDownloadURL` on redirect)
- No other inter-service gRPC calls exist

Auth has zero outbound service dependencies (no gRPC calls to peers).
FileUpload has zero outbound service dependencies (it serves gRPC but never
initiates one).

## Options considered

### Option 1: Single flat network (current state)

One `gobox` network, all containers joined, `internal: false`.

- **Pros:** Simple. Every container can reach every other container. No
  cross-network Docker DNS confusion.
- **Cons:** No lateral movement protection. Violates least-privilege.
  Fileupload can probe auth-postgres. Any compromised container becomes a
  pivot point.

### Option 2: Two networks — edge + internal

Two networks: `net_edge` (external internet access) and `net_internal`
(isolated). All service containers on `net_internal`; only NPM and
public-facing ports on `net_edge`.

- **Pros:** Much better than flat. Internet-isolated internal network.
- **Cons:** Still flat internally. Core can reach shortener's Redis.
  Shortener can reach auth's Postgres. More attack surface than necessary.

### Option 3: One named network per service-to-service relationship (chosen)

Nine networks, each scoped to exactly the communication it needs. A container
joins a network **only** if it has a direct, named dependency across that
network.

- **Pros:** Strongest isolation. No lateral movement between unrelated
  networks. Each network's set of members is an explicit, auditable list.
  Adding a new service-to-service relationship requires creating a new named
  network — making the change visible in the compose diff.
- **Cons:** More verbose compose files. Operator must understand the
  per-network membership model.

## Chosen approach

**Option 3 — One network per relationship.**

### Network membership rules (immutable)

1. A service joins a network **if and only if** it has a direct, named
   dependency that crosses that network.
2. No service joins a network "just in case."
3. If a future service needs to reach fileupload, a **new** named network is
   added for that pair — existing networks are never widened to include new
   members.
4. A data-network (e.g., `net_auth_data`) is always separate from a
   service-network (e.g., `net_core_auth`). No service joins its own data
   network — only the data stores live there.

### Net-edge rules

- `net_edge` is the **only** non-internal network (`internal: false`).
  nginx-proxy-manager needs internet access for Let's Encrypt / ACME
  certificate issuance.
- Only three things join `net_edge`:
  - nginx-proxy-manager (port 80/443 ingress)
  - core (HTTP port — the only public service port in v1)
  - shortener (HTTP port — public redirects at `/s/{slug}`)
- No database, no Redis, no MinIO, no auth ever joins `net_edge`.

### Special note: ThumbGen does not exist yet

Per GOBOX_SPEC.md §8, ThumbGen is built in **Phase 5**. It does not exist
in the current commit. The following rules apply:

- `net_core_thumbgen` is **documented but not created** until Phase 5. No
  docker-compose.yml should define this network before then.
- The existing `docker-compose.yml` files must compile and boot with no
  `net_core_thumbgen` definition and no thumbgen container.
- Core already has a **nil-guarded gRPC client stub** at
  `core/internal/infrastructure/grpcclient/thumbgen/stub.go` (committed).
  This stub:
  - Accepts the `THUMBGEN_GRPC_ADDR` env var at startup.
  - Returns a no-op response without opening any gRPC connection.
  - Allows core to compile and run even when the thumbgen binary is absent.
- The network topology must not require the presence of `net_core_thumbgen`
  for core to start. Core's compose file must remain valid during Phases 1–4.

### Container membership matrix

| Container | Networks joined |
|-----------|----------------|
| nginx-proxy-manager | `net_edge` |
| core | `net_edge`, `net_core_auth`, `net_core_fileupload`, `net_core_shortener` (not `net_core_thumbgen` until Phase 5) |
| auth | `net_core_auth`, `net_auth_data` |
| auth-postgres | `net_auth_data` |
| fileupload | `net_core_fileupload`, `net_shortener_fileupload`, `net_fileupload_data` |
| fileupload-postgres | `net_fileupload_data` |
| minio | `net_fileupload_data` |
| shortener | `net_core_shortener`, `net_shortener_fileupload`, `net_shortener_data` |
| shortener-postgres | `net_shortener_data` |
| shortener-redis | `net_shortener_data` |
| thumbgen _(Phase 5)_ | `net_core_thumbgen`, `net_fileupload_data` (reads source files from MinIO) |

### Port publishing rules

- Only containers on `net_edge` have published host ports. The base compose
  file publishes exactly the ports needed for public ingress (80, 443, 81 for
  NPM; core's HTTP port for health checks; shortener's HTTP port for
  redirects).
- `internal: true` networks must **never** have published host ports in the
  base compose file. If a development workflow requires reaching a database
  from the host, a `docker-compose.override.yml` adds `ports:` — never the
  base file.
- All containers on `internal: true` networks omit the `ports:` stanza
  entirely (or use `expose:` as documentation). This signals "this container
  is not meant to be reached from the host in production."

### Docker Compose structural contract

The project uses a **two-tier** compose structure:

1. **Root compose file** (`docker-compose.yml` — at the project root):
   Defines all eight currently-active networks (see container membership
   matrix above), all service containers with per-network membership, and
   inter-service `depends_on` ordering. Networks are declared inline
   (`external: false`). This is the production-grade composition for the
   full stack. Run it with:
   ```shell
   docker compose up -d
   ```
   from the project root.

2. **Per-service compose files** (`auth/docker-compose.yml`,
   `core/docker-compose.yml`, `fileupload/docker-compose.yml`,
   `shortener/docker-compose.yml`): Used for **standalone development**
   of that service only. Each declares only its own service container
   plus its data stores (e.g., `auth` + `auth-postgres`). Networks are
   declared as `external: true` — the root compose at the project root is
   the single source of truth for network definitions.

   Each per-service compose file must include a comment at the top:
   ```yaml
   # Standalone development only. For the full stack, use docker-compose.yml at the project root.
   # Networks are external; create them with:
   #   docker network create net_core_<this-service>
   #   docker network create net_<this-service>_data
   ```

### Dependency chain (root compose)

The root `docker-compose.yml` declares explicit `depends_on` relationships
to enforce startup ordering without hardcoding addresses:

| Service | Depends on | Rationale |
|---------|------------|-----------|
| `auth-postgres` | — | Standalone data store |
| `auth` | `auth-postgres` (healthy) | Needs DB before accepting gRPC |
| `fileupload-postgres`, `minio` | — | Standalone data stores |
| `fileupload` | `fileupload-postgres` (healthy), `minio` (healthy) | Needs DB + S3 before serving gRPC |
| `shortener-postgres`, `shortener-redis` | — | Standalone data stores |
| `shortener` | `shortener-postgres` (healthy), `shortener-redis` (healthy) | Needs DB + cache before serving |
| `core` | `auth` (started), `fileupload` (started), `shortener` (started) | Needs upstream gRPC targets resolvable by DNS |
| `nginx-proxy-manager` | `core` (started), `shortener` (started) | NPM health-checks backends at startup; should not start before them |

Note: `core` uses `condition: service_started` (not `service_healthy`)
because core's own startup validates that gRPC targets are reachable at
the DNS level. The actual gRPC connection is established lazily or with
back-off — core does not hard-fail if the target is not yet accepting
connections at compose startup time.

## Constraints and risks

- **Complexity:** Nine networks are harder to reason about than one flat
  network. Mitigation: the container membership matrix above is the single
  source of truth. Every Builder agent must verify the matrix before
  modifying a compose file.
- **Docker DNS scope:** A container resolves other containers only on
  networks they share. If a container is accidentally joined to the wrong
  network (or omitted), DNS resolution fails. Mitigation: the root
  `docker-compose.yml` is the authoritative definition; override files must
  not add network membership to containers defined in the root file.
- **ThumbGen bootstrap risk:** When ThumbGen is built in Phase 5, the
  `net_core_thumbgen` network must be added to core's compose file, and core
  must be restarted. Mitigation: the ThumbGen ADR (to be written in Phase 5)
  must explicitly list the network changes required.
- **No shared network for inter-data communication:** Auth's Postgres,
  FileUpload's Postgres, and Shortener's Postgres are each on their own data
  network. They cannot reach each other. This is intentional — no service
  needs cross-DB access.
