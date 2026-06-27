# ADR-002: gRPC-to-HTTP Error Mapping for Core API

## Decision

Core API maps gRPC status codes to HTTP status codes using a fixed table. All errors are returned in a JSON envelope `{"error":{"code":"...","message":"..."}}`. Handlers never leak gRPC status details to the client.

## Context

Core API acts as a REST gateway that calls downstream services via gRPC. Every handler flow is:

```
Client (HTTP) → Core Handler → gRPC call → downstream service
                     ↓
              HTTP response ← error mapped to HTTP
```

The gobox-proto generated gRPC clients return `status.Status` errors. Core handlers must translate these into consumer-friendly HTTP responses using the spec's error envelope (§5.2). Without a consistent mapping, clients would receive raw gRPC statuses, implementation details, or inconsistent error envelopes.

## Options considered

### 1. Fixed mapping table in a shared error translator (chosen)

A single `grpcerror.HTTPStatus(err error) (int, string)` function maps `status.Code` → HTTP status + error code string. Used by every REST handler.

- **Pros:** Single source of truth, easy to test, consistent across all endpoints.
- **Cons:** A new gRPC status not in the table will panic or fall through to 500.

### 2. Per-handler error handling

Each handler maps errors individually.

- **Pros:** Flexible — handler can add context.
- **Cons:** Duplication, inconsistent responses. Violates DRY.

### 3. Echo HTTP error handler override

Set `echo.DefaultHTTPErrorHandler` to catch all return-site errors.

- **Pros:** Catches errors from middleware too.
- **Cons:** Still needs the mapping table internally. Cannot differentiate between gRPC and non-gRPC errors without inspecting the error chain. Less explicit.

## Chosen approach

**Option 1 — Fixed mapping table in a shared translator package.**

### Mapping table

| gRPC Code | HTTP Status | Error Code Field | Condition |
|-----------|-------------|------------------|-----------|
| `NotFound` | 404 | `NOT_FOUND` | User, session, or resource does not exist |
| `Unauthenticated` | 401 | `UNAUTHORIZED` | JWT missing/invalid/expired |
| `AlreadyExists` | 409 | `CONFLICT` | Email already registered, duplicate resource |
| `InvalidArgument` | 400 | `BAD_REQUEST` | Missing or malformed fields |
| `Internal` | 500 | `INTERNAL_ERROR` | Catch-all for server errors |
| All other codes | 500 | `INTERNAL_ERROR` | Fallback for unexpected gRPC codes |

### Error envelope

All error responses follow exactly:

```json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "token expired"
  }
}
```

- `code` is always a UPPER_SNAKE_CASE string (from the mapping table).
- `message` is a human-readable string. It may be derived from the gRPC error message but should **never** include stack traces, internal hostnames, or raw gRPC status strings.
- The HTTP response body is always valid JSON.

### Implementation sketch

Package `internal/interface/rest/middleware/error.go`:

```go
package middleware

import (
    "net/http"

    "github.com/labstack/echo/v4"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

type ErrorResponse struct {
    Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}

// MapGRPCError returns an echo.HTTPError (handled by Echo's error handler)
// or can be called directly in handlers.
func MapGRPCError(err error) *echo.HTTPError {
    st, ok := status.FromError(err)
    if !ok {
        return echo.NewHTTPError(http.StatusInternalServerError, ErrorResponse{
            Error: ErrorDetail{Code: "INTERNAL_ERROR", Message: "unexpected error"},
        })
    }
    httpStatus, code := grpcToHTTP(st.Code())
    return echo.NewHTTPError(httpStatus, ErrorResponse{
        Error: ErrorDetail{Code: code, Message: st.Message()},
    })
}

func grpcToHTTP(c codes.Code) (int, string) {
    switch c {
    case codes.NotFound:
        return http.StatusNotFound, "NOT_FOUND"
    case codes.Unauthenticated:
        return http.StatusUnauthorized, "UNAUTHORIZED"
    case codes.AlreadyExists:
        return http.StatusConflict, "CONFLICT"
    case codes.InvalidArgument:
        return http.StatusBadRequest, "BAD_REQUEST"
    case codes.Internal:
        return http.StatusInternalServerError, "INTERNAL_ERROR"
    default:
        return http.StatusInternalServerError, "INTERNAL_ERROR"
    }
}
```

### Usage in handlers

```go
func (h *AuthHandler) Login(c echo.Context) error {
    var req LoginRequest
    if err := c.Bind(&req); err != nil {
        return middleware.MapGRPCError(status.Error(codes.InvalidArgument, "invalid request body"))
    }
    resp, err := h.authClient.Login(ctx, &pb.LoginRequest{...})
    if err != nil {
        return middleware.MapGRPCError(err)   // ← single call, no per-case logic
    }
    return c.JSON(http.StatusOK, resp)
}
```

### Domain errors (non-gRPC)

Core API has no database, so domain errors come from two sources:
1. **gRPC calls** (mapped as above).
2. **Input validation** (performed in the handler or use case before gRPC call).

Input validation errors use `BAD_REQUEST` with a descriptive message. The same `ErrorResponse` struct is reused.

### Echo HTTP error handler

The global Echo error handler is replaced to ensure all unhandled panics and middleware errors also use the correct envelope:

```go
e.HTTPErrorHandler = func(err error, c echo.Context) {
    // If it's already an echo.HTTPError with our ErrorResponse, return as-is.
    // Otherwise wrap in INTERNAL_ERROR.
}
```

## Constraints and risks

- **Leaking gRPC messages:** Downstream gRPC errors may contain internal details (e.g., "connection refused", "deadline exceeded"). The mapping function should sanitize the message to avoid leaking infrastructure info. For Unexpected/Internal codes, return a generic message like "internal server error" and log the original.
- **Missing codes:** If a new gRPC code is introduced (e.g., `PermissionDenied`), the fallback maps it to 500. This is safe but produces a poor UX. ADR-002 should be updated when new codes are needed.
- **Testing surface:** Every handler-gRPC pair must be tested for correct error mapping. This is covered by use-case unit tests with mocked gRPC clients.
