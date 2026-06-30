# ADR-002: Nginx Proxy Manager Routing Strategy

## Decision

Use **subdomain-based routing** via **nginx-proxy-manager** (NPM) as the sole
reverse proxy for all public HTTP(S) traffic. The routing table is:

| Subdomain | Target | Port | Notes |
|-----------|--------|------|-------|
| `api.{{domain}}` | core | `CORE_HTTP_PORT` (8080) | All REST API traffic |
| `s.{{domain}}` | shortener | `SHORTENER_HTTP_PORT` (8082) | Public redirects (`/s/{slug}` → 302) |
| `auth.{{domain}}` | — | — | **NOT exposed** — see prohibition below |

NPM runs as a Docker container with the following configuration:

- Published host ports: **80** (HTTP), **443** (HTTPS), **81** (admin UI)
- Joined networks: `net_edge` (required) + any network where a routed
  service publishes its HTTP port
- Data persistence: a **named Docker volume** (`npm_data`), never a bind
  mount, so the config survives container recreation
- All other configuration is done through NPM's admin UI or API at port 81

### Key architectural rule

**Routed services do NOT know about NPM.** The `docker-compose.yml` for core
and shortener publish their HTTP ports and join `net_edge`, but they hold no
environment variable, configuration key, or code path that references NPM.
NPM is an edge concern, not a service concern.

## Context

GOBOX_SPEC.md §3 states:

> Core API is the **only** service with a public port; all others are
> cluster-internal

However, §5.4 (Link Shortener) specifies a public redirect endpoint:

```
GET /s/{slug} → lookup → call FileUpload gRPC → 302 redirect
```

This endpoint must be publicly reachable. Two options exist: route it through
Core (path-based) or expose Shortener directly (subdomain-based).

Additionally, the spec does not define how TLS termination or Let's Encrypt
certificate management works. Nginx Proxy Manager handles both: it terminates
TLS, proxies to the backend over plain HTTP, and automates ACME certificate
renewal.

## Options considered

### Option 1: Subdomain-based routing (chosen)

Each public-facing service gets its own subdomain. NPM routes by hostname.

- **Pros:** Clean separation of concerns. Shortener's redirect endpoint is
  directly reachable without going through Core (reduces latency for
  redirects). NPM can terminate TLS per-subdomain independently. No path
  rewriting complexity. Easy to add future public services (e.g.,
  `docs.{{domain}}`, `status.{{domain}}`).
- **Cons:** Requires DNS configuration for each subdomain. Slightly more
  NPM configuration (one proxy host per subdomain). Two public-facing
  service ports instead of one.

### Option 2: Path-based routing through Core only

Only Core publishes an HTTP port. Shortener's redirect is proxied through
Core: `api.{{domain}}/s/{slug}` → Core reads the path → Core proxies to
Shortener's internal gRPC endpoint → Core returns the redirect.

- **Pros:** Single public port. No DNS entries beyond the base domain.
  Aligns with "Core is the only public service" rule.
- **Cons:** Adds a hop for every redirect (Core must handle the request,
  call Shortener gRPC, and return a 302). Increases Core's surface area.
  Path routing in NPM would still be needed if Core doesn't handle it, but
  that defeats the purpose. Violates "redirects should be fast and cacheable"
  from §5.4.

### Option 3: NPM with path-based routing

NPM routes `/s/{slug}` to shortener and everything else to core, all under
the same domain.

- **Pros:** Single subdomain. One DNS entry.
- **Cons:** NPM path matching is fragile and order-dependent. Adding a new
  public route requires reviewing all existing path rules to avoid conflicts.
  If core adds a route like `/s/config` in the future, it clashes with
  shortener's `/s/{slug}`. NPM does not support regex path matching as
  gracefully as subdomain matching. The path-prefix coupling makes
  independent service deployment harder.

## Chosen approach

**Option 1 — Subdomain-based routing.**

### Auth subdomain: explicit prohibition

`auth.{{domain}}` must **never** be configured in NPM as a proxy host. Auth
has no public endpoints in v1 per GOBOX_SPEC.md (§5.1 — HTTP endpoints are
health and JWKS, which Core proxies internally). Exposing auth would:

1. Bypass Core's JWT middleware for auth operations.
2. Allow direct gRPC calls to Auth (if the gRPC port were published — which
   it is not, but a misconfigured NPM rule could make it reachable).
3. Violate the spec's rule that Core is the single public entry point.

This prohibition is documented here so that no Builder agent or operator
adds `auth.{{domain}}` to NPM by mistake. If a future requirement justifies
exposing auth directly, a new ADR must reverse this decision.

### NPM container spec

```yaml
services:
  nginx-proxy-manager:
    image: jc21/nginx-proxy-manager:latest
    ports:
      - "80:80"       # HTTP — redirects to HTTPS
      - "443:443"     # HTTPS — proxied backends
      - "81:81"       # Admin UI (bind to 127.0.0.1 in production)
    volumes:
      - npm_data:/data
      - npm_letsencrypt:/etc/letsencrypt
    networks:
      - net_edge
      - net_core_shortener    # needed to reach shortener's HTTP port
    restart: unless-stopped
```

Note: The `net_edge` network alone is insufficient for NPM to reach
shortener because `net_edge` is a shared network — NPM is on it, and
shortener is on it. Actually, NPM reaches both core and shortener through
`net_edge` alone, since both services publish their HTTP ports on
`net_edge`. NPM does NOT need to join `net_core_shortener` — it reaches
shortener via `net_edge`.

Correction from the matrix above: NPM joins **only** `net_edge`. It proxies
to core and shortener using container DNS names (`core:8080`,
`shortener:8082`) resolved through `net_edge`.

### NPM configuration (admin UI)

The following proxy hosts must be configured in NPM's admin UI:

**Proxy Host: `api.{{domain}}`**
- Scheme: `http`
- Forward Hostname: `core`
- Forward Port: `8080`
- Websockets Support: `off`
- Block Common Exploits: `on`
- SSL: Let's Encrypt, force SSL

**Proxy Host: `s.{{domain}}`**
- Scheme: `http`
- Forward Hostname: `shortener`
- Forward Port: `8082`
- Websockets Support: `off`
- Block Common Exploits: `on`
- SSL: Let's Encrypt, force SSL

**Access List:** All proxy hosts use NPM's default "Public" access list (no
IP whitelisting in v1).

### Environment variables

NPM itself requires no environment variables from the GoBox project. It uses
its own internal SQLite database (stored on the `npm_data` volume) for
configuration. No secret, port, or hostname is shared between NPM and the
GoBox services via environment variables.

### HTTP → HTTPS redirect

NPM handles this automatically when SSL is enabled for a proxy host. HTTP
port 80 receives a 301 redirect to the HTTPS URL. No service-level redirect
logic is needed.

### Health checks

NPM monitors backend health via the proxy host configuration. If a backend
is down, NPM returns 502. The health check interval defaults to NPM's
built-in setting (configurable in the admin UI, but not from compose).

## Constraints and risks

- **DNS dependency:** Each subdomain must resolve to the host machine's
  public IP. For local development, `/etc/hosts` entries can simulate this
  (e.g., `127.0.0.1 api.gobox.local s.gobox.local`). Document this in the
  project README (or a dev setup guide).
- **NPM admin UI is not secured by GoBox auth:** Port 81 (admin UI) is
  protected only by NPM's built-in authentication (default credentials). In
  production, this port should be bound to `127.0.0.1:81` and accessed via
  SSH tunnel, or a separate NPM access rule with IP whitelist.
- **Single point of failure:** All traffic passes through NPM. If NPM is
  down, the entire system is unreachable. Mitigation: NPM's restart policy
  is `unless-stopped`; data persists in named volumes.
- **No caching layer:** NPM does not cache responses. Shortener's redirect
  responses are lightweight (302 with Location header) — caching is not
  needed. Core API responses can be cached at the CDN layer (out of scope
  for v1).
- **Certificate renewal:** NPM auto-renews Let's Encrypt certificates.
  Renewal requires port 80 to be reachable from the internet. `net_edge` is
  non-internal, so this works. No manual certificate management needed.
