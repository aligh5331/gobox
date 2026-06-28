# ADR-002: Redirect Flow

## Decision

The redirect endpoint `GET /s/{slug}` follows a read-through cache pattern:

1. Check Redis for `slug:{slug}` — if found, the value is the `file_id` (UUID).
2. On cache miss, query Postgres `short_links` by slug. If not found, return 404.
   Populate Redis with `slug → file_id` and a 5-minute TTL.
3. Call FileUpload gRPC `GetDownloadURL(file_id)` to obtain a fresh presigned S3 URL.
4. Return HTTP 302 with `Location` set to the presigned URL.
5. Increment `hit_count` on the `short_links` row asynchronously (fire-and-forget goroutine).

**Never cache the presigned URL itself.** Only the `slug → file_id` mapping is cached.

## Context

- GOBOX_SPEC.md §5.4 defines the redirect flow as: "lookup slug in Redis (TTL 5 min),
  on miss: lookup Postgres, populate Redis, call FileUpload gRPC: GetDownloadURL
  (fresh presigned URL), 302 redirect to presigned URL, increment hit_count async."
- Presigned S3 URLs have their own expiration TTL (typically 15–60 minutes, configurable
  in FileUpload). Caching them would serve stale, expired URLs to clients.
- The `slug → file_id` mapping is stable for the lifetime of the ShortLink record — it
  never changes. A 5-minute TTL on this mapping is safe: it only adds a small read
  amplification (one Postgres query per 5 minutes per popular slug).
- The redirect endpoint is **public** (no JWT required) and **read-only**. It must be fast
  and handle high traffic.
- FileUpload may optionally verify that the requesting user owns the file (for the gRPC
  call), but in practice the redirect is anonymous — FileUpload's `GetDownloadURL`
  should accept a `user_id`-optional or system-level request.

## Options considered

### 1. Read-through cache: Redis → Postgres → gRPC → redirect (chosen)

Cache the `slug → file_id` mapping in Redis for 5 minutes. Always call FileUpload
for a fresh presigned URL on each redirect.

- **Pro:** Fast path (Redis hit) skips Postgres entirely. ~1ms latency for cache hit.
- **Pro:** Presigned URL is always fresh — respects FileUpload's expiration policy.
- **Pro:** Postgres is only queried once per 5 minutes per unique slug.
- **Con:** Every redirect still makes a gRPC call to FileUpload. This is acceptable
  because the gRPC call is in-memory localhost (same cluster) and FileUpload's
  `GetDownloadURL` is a lightweight metadata lookup + S3 presign operation.
- **Verdict:** **Chosen.**

### 2. Full response caching: cache the presigned URL in Redis

Cache the entire `slug → presigned_url` mapping with a shorter TTL (e.g. 2 minutes).

- **Pro:** Eliminates the gRPC call on cache hit. Faster redirects.
- **Con:** Presigned URL expiry and cache TTL are two independent clocks that must be
  coordinated — if they drift, clients receive expired URLs. The cache TTL would need
  to be strictly shorter than the presigned URL TTL, creating a complex coupling
  between Shortener config and FileUpload config.
- **Con:** If a file is deleted or its permissions change, the cached presigned URL
  from FileUpload would still be valid until the cache TTL expires, creating a
  window where a deleted file is still downloadable.
- **Verdict:** Rejected — too risky for production.

### 3. Direct Postgres-only redirect (no Redis)

Skip Redis entirely. Always query Postgres for the `slug → file_id` mapping, then
call FileUpload gRPC.

- **Pro:** Simplest architecture. No Redis dependency.
- **Con:** Postgres becomes the hot path for every redirect. Under high traffic,
  this stresses the DB needlessly. Postgres is harder to scale horizontally
  than Redis; connection pool exhaustion is a real risk.
- **Con:** The spec mandates Redis.
- **Verdict:** Rejected.

## Chosen approach

### Request flow (detailed)

```
1. Client → GET /s/{slug}
2. Shortener HTTP handler (interface/rest/)
3. RedirectUseCase (application/usecase/)

   3a. Redis: GET slug:{slug}
       ── HIT  → file_id = result           (skip 3b)
       ── MISS → continue to 3b

   3b. Postgres: SELECT file_id FROM short_links WHERE slug = $1
       ── NOT FOUND → return 404
       ── FOUND    → Redis SETEX slug:{slug} file_id 300  (5 min TTL)

   3c. gRPC call to FileUpload: GetDownloadURL(file_id=file_id)
       ── returns PresignedURL

4. HTTP 302 Location: PresignedURL
   (goroutine) Postgres: UPDATE short_links SET hit_count = hit_count + 1 WHERE slug = $1
```

### Cache key design

```
Key:     slug:{slug}
Value:   {file_id}   (UUID string, e.g. "550e8400-e29b-41d4-a716-446655440000")
TTL:     300 seconds (5 minutes)
```

- The `slug:` prefix prevents key collisions with any other Redis data in the same instance.
- The value is the `file_id` UUID — not the presigned URL, not the full ShortLink row.
- This is intentionally narrow: the only data needed for the next step (gRPC call) is the `file_id`.

### Hit count: fire-and-forget

The `hit_count` increment must not block the response. It runs in a goroutine:

```go
go func() {
    // Use a background context — the request context may be cancelled after 302 is sent
    if err := repo.IncrementHitCount(context.Background(), slug); err != nil {
        // Log and swallow — redirect already succeeded
        logger.Error().Err(err).Str("slug", slug).Msg("failed to increment hit count")
    }
}()
```

- **No retry:** If the UPDATE fails, the hit count is silently not incremented.
  This is acceptable — hit counts are approximate counters, not financial ledger entries.
- **No queue:** A goroutine is sufficient for this service's projected scale.
  If hit count accuracy becomes critical, a background batch processor can be added later.

### Error handling

| Failure point | Behaviour |
|---|---|
| Redis down | Treat as cache miss → fall through to Postgres. Log warning. |
| Postgres down (on cache miss) | Return 500. The error is terminal — we cannot resolve the slug. |
| FileUpload gRPC down | Return 502. The file cannot be served without a presigned URL. |
| Postgres down (on hit_count) | Swallow error, log warning. Redirect has already succeeded. |
| Slug not found (Postgres) | Return 404 with JSON error body. |

### Redis TTL choice: 5 minutes

- **Short enough** that a deleted or expired link stops being served within 5 minutes.
  (The use case must invalidate the Redis key on delete — see below.)
- **Long enough** to absorb the vast majority of repeat requests for popular slugs.
- If a slug receives requests in bursts (e.g. a social media spike), the TTL is
  refreshed on every cache hit (Redis `GET` does not refresh TTL — use `TTL` + `EXPIRE`
  or switch to `SETEX` on every request if desired, but the 5-minute TTL is measured
  from the last Postgres fetch, so a spike within 5 minutes gets all cache hits).

### Cache invalidation

When a link is deleted (via gRPC `DeleteLink`):

```go
// In the delete-link use case:
repo.Delete(ctx, linkID)                // Postgres: soft-delete or hard-delete
redisCache.Del(ctx, "slug:"+slug)       // Remove from cache
```

- The delete handler also has the slug (fetched from the ShortLink record).
- If Redis is unavailable during delete, the stale entry will naturally expire
  within 5 minutes. No explicit purge is required for correctness.

## Constraints and risks

- **Redis is a hard dependency.** If Redis is down, every redirect hits Postgres.
  Ensure Postgres connection pool has headroom for this fail-open scenario.
- **FileUpload gRPC is in the hot path.** If FileUpload is down, redirects fail
  even for cached slugs. Consider a circuit breaker if this becomes a problem.
- **Hit count is approximate.** Concurrent requests may lose some increments
  (the goroutine races on the same row). For v1 this is acceptable — hit counts
  are displayed as "~X downloads" not exact counters.
- **Presigned URL generation must be fast.** FileUpload's `GetDownloadURL` should
  return within single-digit milliseconds. If S3 presigning is slow, consider
  pre-generating URLs and caching them for a shorter window (revisit Option 2).

## References

- GOBOX_SPEC.md §5.4 — "Link Shortener", "Public HTTP redirect (no auth, high cache)"
- GOBOX_SPEC.md §5.3 — FileUpload gRPC: `GetDownloadURL`
- ADR-003 — Port separation (gRPC 9091, HTTP 8082)
