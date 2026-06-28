# ADR-003: Two-Port Architecture

## Decision

The Shortener service listens on two ports inside the container:

| Port | Protocol | Visibility | Purpose |
|------|----------|------------|---------|
| 9091 | gRPC     | Internal   | Core API creates, reads, deletes, and lists links |
| 8082 | HTTP     | Public     | Anonymous redirect: `GET /s/{slug}` → 302 to presigned URL |

The gRPC server requires JWT authentication on every RPC (validated locally from
the cached JWKS). The HTTP server has **no authentication middleware** — it only
serves public redirects.

## Context

- GOBOX_SPEC.md §3 states: "Core API is the **only** service with a public port;
  all others are cluster-internal." However, §5.4 adds an exception: the redirect
  endpoint `GET /s/{slug}` must be publicly reachable.
- The spec defines: "Port: 9091 (gRPC internal) + 8082 (HTTP for public redirects only)."
- The redirect flow (ADR-002) does not require a JWT — anyone with the slug can
  access the file. This is by design: "all shares are public links" (non-goals §1).
- The gRPC service (`ShortenerService`) has four RPCs: `CreateLink`, `GetLink`,
  `DeleteLink`, `ListLinks`. All of these must be authenticated — only the Core
  API (acting on behalf of an authenticated user) should be able to create or
  delete links.

## Options considered

### 1. Two ports: gRPC (9091) + HTTP (8082) — chosen

One binary, two `net.Listener`s, two servers (gRPC and Echo/HTTP). The HTTP server
only has one route: `GET /s/{slug}`. The gRPC server has the full `ShortenerService`.

- **Pro:** Clean separation of concerns. No risk of accidentally exposing an
  authenticated endpoint to anonymous traffic.
- **Pro:** gRPC stays internal (not load-balanced by the public ingress). Only
  HTTP port 8082 needs to be exposed on the load balancer / reverse proxy.
- **Pro:** HTTP server can be optimized independently (e.g. minimal middleware
  stack: just request ID + logger, no JWT, no rate limiter).
- **Con:** Two listeners per container — slightly more complex `cmd/main.go`.
- **Verdict:** **Chosen.**

### 2. Single port with path-based routing

Serve both gRPC and HTTP on the same port using a multiplexer (e.g. cmux or
grpc-gateway). A single public port handles both `GET /s/{slug}` and gRPC calls.

- **Pro:** Single port to configure and expose. Simpler deployment manifest.
- **Con:** The internal gRPC endpoint would be reachable from the public internet
  (even if behind a load balancer). Adds attack surface — an attacker could
  probe the gRPC service from the redirect URL.
- **Con:** Multiplexing adds a subtle failure mode (protocol detection at the
  connection level) and a dependency on `cmux`.
- **Verdict:** Rejected — security concern outweighs operational simplicity.

### 3. Separate deployment (two containers)

Run two containers: `shortener-grpc` (internal, port 9091) and `shortener-http`
(public, port 8082). Both share the same Postgres and Redis.

- **Pro:** Independent scaling — the HTTP redirect endpoint might need more
  replicas than the gRPC control plane.
- **Pro:** Independent failure domains — a bug in the HTTP handler doesn't
  take down the gRPC service.
- **Con:** Two Docker images to build, two deployments to manage. Shared
  database and Redis between them adds operational overhead (connection pools,
  migration ordering).
- **Con:** Overkill for v1 — the redirect endpoint is stateless and fast.
  If scaling becomes an issue, this option can be adopted later without
  changing the API surface.
- **Verdict:** Deferred — reconsider at post-v1 if redirect traffic dominates.

## Chosen approach

### Server startup (`cmd/main.go`)

```go
func main() {
    // 1. Load config, connect DB, connect Redis, init gRPC clients, etc.

    // 2. Start gRPC server on port 9091
    grpcListener, _ := net.Listen("tcp", ":"+cfg.GRPCPort)    // "9091"
    grpcServer := grpc.NewServer(
        grpc.UnaryInterceptor(authInterceptor),                // JWT validation
    )
    shortenerv1.RegisterShortenerServiceServer(grpcServer, grpcHandler)
    go grpcServer.Serve(grpcListener)

    // 3. Start HTTP server on port 8082
    e := echo.New()
    e.GET("/s/:slug", redirectHandler)
    // Optional: health endpoint
    e.GET("/health", healthHandler)
    go e.Start(":" + cfg.HTTPPort)   // "8082"

    // 4. Wait for shutdown signal
    // ...
}
```

### gRPC server — middleware stack

The gRPC server uses a **unary interceptor** for JWT validation:

```go
func authInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
    // Extract Bearer token from gRPC metadata
    // Validate JWT signature + expiry using cached JWKS public key
    // Inject user_id into context
    return handler(ctx, req)
}
```

No JWT validation on the HTTP server — none is needed.

### HTTP server — middleware stack

```go
e := echo.New()
e.Use(middleware.RequestID())   // request tracking
e.Use(middleware.Logger())      // structured access log
// No JWT middleware — this port is intentionally anonymous
e.GET("/s/:slug", redirectHandler)
```

### Health check

Both servers expose a health endpoint:
- gRPC: standard gRPC health check (`grpc.health.v1.Health`)
- HTTP: `GET /health → 200 OK {"status":"ok"}`

Docker Compose health check should target the HTTP port (8082).

### Deployment configuration

```yaml
# docker-compose.yml excerpt
shortener:
  build: ./shortener
  ports:
    - "9091:9091"   # gRPC — internal network only
    - "8082:8082"   # HTTP — public (exposed via reverse proxy)
  environment:
    GRPC_PORT: "9091"
    HTTP_PORT: "8082"
    DATABASE_URL: "postgres://..."
    REDIS_URL: "redis://..."
    FILEUPLOAD_GRPC_ADDR: "fileupload:9090"
```

### Environment variables

The `.env.example` should include:

```
GRPC_PORT=9091
HTTP_PORT=8082
```

## Constraints and risks

- **Two listeners = two shutdown paths.** The signal handler must gracefully
  shut down both servers. Use `Shutdown` (HTTP) and `GracefulStop` (gRPC)
  with a shared context and timeout.
- **gRPC port must be internal only.** The deployment (Docker Compose, k8s)
  must not publish port 9091 to the public internet. Only port 8082 should
  be attached to the load balancer / reverse proxy.
- **No rate limiting on HTTP redirect.** The public redirect endpoint is
  unauthenticated and could be abused for traffic amplification. Consider
  adding a lightweight rate limiter (per IP, e.g. token bucket) in a future
  iteration.
- **HTTP server has no CSRF protection** — it only serves GET redirects with
  no side effects. This is safe: there is no POST/PUT/DELETE on the public port.

## References

- GOBOX_SPEC.md §5.4 — "Link Shortener", ports and redirect details
- GOBOX_SPEC.md §3 — "External (client → Core API)" and "Internal (service → service)"
- ADR-001 — Slug generation (slug is the URL path parameter for redirect)
- ADR-002 — Redirect flow (the logic served on port 8082)
