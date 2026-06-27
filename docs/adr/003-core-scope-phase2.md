# ADR-003: Core API Phase 2 Scope Delimitation

## Decision

Phase 2 of Core API implements **only** the auth and `/me` endpoints plus the JWT validation middleware. File, share, and thumbnail routes are explicitly excluded and will be built in Phases 3–5.

## Context

Per GOBOX_SPEC.md §8 (Build order), the project is built in 6 phases. Phase 2 gate:

> *"register, login, refresh, logout work end-to-end through Core → Auth gRPC; JWT validation middleware rejects expired/invalid tokens with 401."*

The spec's REST endpoint list (§5.2) shows endpoints for all services. Without explicit scoping, a Builder agent might attempt to implement everything at once, violating the "one service per session" rule and the file-tree allowlist (Phase 2 allows only `core/**` — auth endpoints + middleware).

## Options considered

### 1. Explicit phase-scope document (chosen)

Write an ADR listing every endpoint included in Phase 2 and every endpoint deferred. The Builder agent checks this document before creating any route.

- **Pros:** Removes ambiguity. Prevents scope creep. Enables automated verification (grep for unwanted paths).
- **Cons:** Requires updating ADR-003 when transitioning to Phase 3.

### 2. Rely on file-tree allowlist only

The existing `AGENTS.md` allowlist restricts Phase 2 to `core/**` (auth endpoints + middleware only). No additional document needed.

- **Cons:** The allowlist does not say which endpoints are in scope — only which directories. An agent could create `core/internal/interface/rest/file*.go` thinking it's allowed because it's under `core/`. The allowlist is too coarse.

### 3. Tag each endpoint in GOBOX_SPEC.md

Add phase annotations directly in the spec (e.g., `[phase=2]` next to each endpoint).

- **Pros:** Single source of truth.
- **Cons:** Requires modifying GOBOX_SPEC.md, which AGENTS.md declares read-only. Breaks the rule.

## Chosen approach

**Option 1 — Explicit phase-scope document.**

### In-scope (Phase 2)

All endpoints below are built under `core/internal/interface/rest/`. They use:
- Echo v4 router
- JWT middleware (from `pkg/jwtutil/`, see ADR-001)
- Error mapping (from `internal/interface/rest/middleware/`, see ADR-002)
- Auth gRPC client (wrapping `gobox-proto/gen/auth/v1`)

```
POST   /api/v1/auth/register       ← public (no JWT)
POST   /api/v1/auth/login           ← public (no JWT)
POST   /api/v1/auth/refresh         ← public (no JWT; uses refresh token in body)
DELETE /api/v1/auth/logout          ← authenticated (JWT required)

GET    /api/v1/me                   ← authenticated → calls Auth.GetUser
PUT    /api/v1/me                   ← authenticated → calls Auth.UpdateProfile
PUT    /api/v1/me/password          ← authenticated → calls Auth.ChangePassword

GET    /health                      ← public, no auth
```

**Architecture per endpoint:**

| Endpoint | HTTP → Use Case → gRPC |
|----------|----------------------|
| `POST /api/v1/auth/register` | Handler validates body → calls Auth.Register gRPC → returns user + tokens |
| `POST /api/v1/auth/login` | Handler validates body → calls Auth.Login gRPC → returns user + tokens + session |
| `POST /api/v1/auth/refresh` | Handler validates body → calls Auth.RefreshToken gRPC → returns new token pair |
| `DELETE /api/v1/auth/logout` | Handler reads `session_id` from body (optional: from JWT claims) → calls Auth.Logout gRPC |
| `GET /api/v1/me` | Handler extracts `user_id` from JWT claims → calls Auth.GetUser gRPC → returns user |
| `PUT /api/v1/me` | Handler extracts `user_id` + validates body → calls Auth.UpdateProfile gRPC → returns user |
| `PUT /api/v1/me/password` | Handler extracts `user_id` + validates body → calls Auth.ChangePassword gRPC |

### Out-of-scope (Phases 3–5)

The following endpoints must **not** be implemented, registered, or stubbed with TODO bodies in Phase 2:

```
POST   /api/v1/files                   ← Phase 3 (File Upload)
GET    /api/v1/files                   ← Phase 3
GET    /api/v1/files/{id}              ← Phase 3
DELETE /api/v1/files/{id}              ← Phase 3
POST   /api/v1/files/{id}/share        ← Phase 4 (Link Shortener)
GET    /api/v1/files/{id}/thumbnail    ← Phase 5 (Thumbnail Generator)
```

No `FileUpload`, `Shortener`, or `ThumbGen` gRPC clients should be created or wired in Phase 2. These will be added in their respective phases.

### What Phase 2 produces

```
core/
├── cmd/
│   └── main.go                       ← wires: Echo server, JWT middleware, Auth gRPC client, routes
├── internal/
│   ├── application/
│   │   └── usecase/                  ← use case structs (if orchestration beyond raw gRPC call is needed)
│   ├── infrastructure/
│   │   └── grpcclient/
│   │       └── auth.go               ← Auth gRPC client wrapper (connection pool, dial options)
│   └── interface/
│       └── rest/
│           ├── middleware/
│           │   ├── error.go          ← gRPC→HTTP error mapping (ADR-002)
│           │   └── jwt.go            ← JWT validation middleware (if not in pkg/jwtutil)
│           ├── auth_handler.go       ← register, login, refresh, logout
│           └── me_handler.go         ← get, update profile, change password
├── pkg/
│   └── jwtutil/
│       ├── jwks.go                   ← JWKS cache (ADR-001)
│       ├── claims.go                 ← custom JWT claims
│       └── middleware.go             ← Echo middleware (or lives in interface/rest/middleware/)
├── .env.example
└── Dockerfile
```

### What Phase 2 does NOT produce

- No S3/MinIO client
- No Redis client
- No FileUpload, Shortener, or ThumbGen gRPC stubs
- No file upload or share routes
- No thumbnail processing
- No database migrations (Core is stateless)

### Route registration strategy

A single `RegisterRoutes(e *echo.Echo, h *AuthHandler, m *jwtutil.JWTMiddleware)` function registers all Phase 2 routes. This function is extended in later phases by adding new handler parameters and route groups, never by modifying existing route definitions.

```go
func RegisterRoutes(e *echo.Echo, authHandler *AuthHandler, jwtMiddleware echo.MiddlewareFunc) {
    // Public group
    public := e.Group("")
    public.POST("/api/v1/auth/register", authHandler.Register)
    public.POST("/api/v1/auth/login", authHandler.Login)
    public.POST("/api/v1/auth/refresh", authHandler.Refresh)
    public.GET("/health", healthHandler)

    // Authenticated group
    authed := e.Group("")
    authed.Use(jwtMiddleware)
    authed.DELETE("/api/v1/auth/logout", authHandler.Logout)
    authed.GET("/api/v1/me", authHandler.GetMe)
    authed.PUT("/api/v1/me", authHandler.UpdateMe)
    authed.PUT("/api/v1/me/password", authHandler.ChangePassword)
}
```

## Constraints and risks

- **Future route conflicts:** Phase 3 will add `POST /api/v1/files` which needs the JWT middleware. The route registration pattern above makes it easy to add new authed routes — just add them under `authed`. No risk.
- **Handler file split:** Auth vs Me handlers can be in the same file (e.g., `auth_handler.go`) or split. For clarity, split into `auth_handler.go` and `me_handler.go`. Both are in `internal/interface/rest/`.
- **Endpoint naming:** `/api/v1/me` uses the path prefix `/me`, not `/auth/me`. This matches the spec. The JWT middleware extracts `user_id` from claims, so no URL parameter is needed.
- **No `/auth/logout/all`** in Phase 2 scope: The spec lists `LogoutAll` as an Auth use case but does not expose it as a Core REST endpoint in §5.2. If needed later, it can be added without breaking changes.
