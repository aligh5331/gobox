# ADR-003 (File Upload): Presigned URL TTL Configuration

## Decision

Two separate TTL environment variables control presigned URL expiration:

| Operation | Env Var | Default | Purpose |
|-----------|---------|---------|---------|
| PUT (upload) | `PRESIGN_UPLOAD_TTL_MINUTES` | **15** | Time window for the client to upload bytes to S3 |
| GET (download) | `PRESIGN_DOWNLOAD_TTL_MINUTES` | **60** | Time window for the download link to remain valid |

The TTL values are in **minutes** (integer). The minimum allowed value is 1. There is no maximum enforcement, but values above 1440 (24 hours) trigger a warning log.

## Context

GOBOX_SPEC.md §5.3 requires FileUpload to return presigned S3 URLs for two distinct operations:

1. **InitiateUpload** returns a presigned PUT URL so the client can upload the file directly to S3.
2. **GetDownloadURL** returns a presigned GET URL so the client (or the Link Shortener's redirect) can download the file.

The spec does not specify TTL values. Three architectural concerns drive the need for two different defaults:

1. **Security posture:** A PUT URL grants write access to a specific S3 key. A long-lived PUT URL means an attacker who intercepts the InitiateUpload response (or a stale copy in client logs) can overwrite the file with arbitrary content long after the intended upload window.
2. **User experience:** Download links are shared (via the Shortener's short URLs). A very short download TTL (e.g., 1 minute) makes shared links nearly useless. A longer TTL balances convenience with the ability to regenerate fresh URLs on each redirect.
3. **Cleanup window:** The cleanup goroutine (see ADR-001) needs a deterministic grace period to identify stale `pending` records. The PUT TTL defines this window.

## Options considered

### 1. Separate TTLs: 15 min PUT / 60 min GET (chosen)

Upload window is short (15 min); download window is longer (60 min).

- **Pros:** Tight security on upload; usable download links; aligns with industry practice (AWS S3 docs recommend 5–15 min for uploads, 30–60 min for downloads).
- **Cons:** Two env vars to manage; requires documentation in `.env.example`.

### 2. Single TTL for both operations

One env var (e.g., `PRESIGN_TTL_MINUTES`) applies to both PUT and GET.

- **Pros:** Simpler configuration surface.
- **Cons:** If set to 15 minutes, download links expire too quickly for sharing. If set to 60 minutes, upload window is unnecessarily wide (4x the recommended max). Poor security/UX tradeoff.

### 3. Dynamic TTL based on file size

Compute TTL as `min(60, max(1, ceil(file_size / 1MB)))` minutes for uploads, and a fixed 60 minutes for downloads.

- **Pros:** Automatically gives more time for large files.
- **Cons:** Adds configuration surprise — clients can't predict TTL from config; larger files are not necessarily slower to upload (depends on client bandwidth); violates principle of least surprise.

### 4. No TTL (infinite)

Presigned URL never expires.

- **Pros:** Simplest implementation; URLs can be bookmarked.
- **Cons:** A leaked PUT URL grants permanent write access to that key. A leaked GET URL never stops working. This is a security anti-pattern.

## Chosen approach

**Option 1 — Separate TTLs: 15 min PUT / 60 min GET.**

### Why 15 minutes for PUT?

- **Upload window realism:** A 15-minute window is sufficient for files up to 100 MiB (the default `MAX_FILE_SIZE`) on any modern connection. Even at 1 Mbps, a 100 MiB file uploads in ~13 minutes.
- **Short-lived risk window:** If the presigned URL is intercepted (e.g., in client-side logs, browser dev tools), the window for exploiting it is small.
- **Industry standard:** AWS SDK documentation and S3 best practices consistently recommend 5–15 minutes for presigned PUT URLs. 15 minutes is the upper bound of that range.
- **Cleanup alignment:** The stale-pending cleanup goroutine (ADR-001) uses `PRESIGN_UPLOAD_TTL_MINUTES + 5` minutes as its threshold. At 15 minutes, the total orphan record lifespan is ~20 minutes before cleanup.

### Why 60 minutes for GET?

- **Shareable links:** The Link Shortener (§5.4) redirects users to presigned GET URLs. If the GET URL expires in 15 minutes, a shared short link becomes a dead end quickly — the user clicks the link, gets a 302 to S3, and S3 returns `AccessDenied` because the presigned URL expired.
- **Regeneration on redirect:** The Shortener calls `GetDownloadURL` on every redirect (spec §5.4: "call FileUpload gRPC: GetDownloadURL (fresh presigned URL)"). This means the TTL only needs to cover the user's download time (browser → S3), not the link's lifetime. 60 minutes is generous for a single download.
- **Cold storage downloads:** For very large files or slow connections, a shorter TTL may cause the download to fail mid-stream (S3 checks expiration at request start, not byte-by-byte, so this is unlikely but possible). 60 minutes provides a comfortable buffer.

### Why separate env vars instead of a single var with operation prefix?

Two env vars (`PRESIGN_UPLOAD_TTL_MINUTES` and `PRESIGN_DOWNLOAD_TTL_MINUTES`) are clearer than a single var like `PRESIGN_TTL_MINUTES` with an operation prefix (`PRESIGN_TTL_PUT=15, PRESIGN_TTL_GET=60`). The latter adds parsing complexity for no benefit.

### Config reading and validation

```go
// In pkg/config/config.go

type Config struct {
    // ... other fields ...

    PresignUploadTTLMinutes   int `env:"PRESIGN_UPLOAD_TTL_MINUTES" envDefault:"15"`
    PresignDownloadTTLMinutes int `env:"PRESIGN_DOWNLOAD_TTL_MINUTES" envDefault:"60"`
}
```

Validation at startup:
- Both values must be ≥ 1.
- If either value is > 1440, a warning is logged: `"presign TTL exceeds 24 hours (1440 minutes): set=%d"`.

### Usage in use cases

```go
// InitiateUpload use case
putTTL := time.Duration(cfg.PresignUploadTTLMinutes) * time.Minute
uploadURL, err := s3Client.PresignPutObject(ctx, bucket, storageKey, putTTL)

// GetDownloadURL use case
getTTL := time.Duration(cfg.PresignDownloadTTLMinutes) * time.Minute
downloadURL, err := s3Client.PresignGetObject(ctx, bucket, storageKey, getTTL)
```

### Interaction with the Shortener

The Shortener (§5.4) calls `GetDownloadURL` on every GET `/s/{slug}` request. This means:

- The download TTL (60 min) starts when the user clicks the short link.
- If the user waits >60 minutes before clicking "Download" on the S3 redirect page (unlikely), the URL will have expired.
- The Shortener could regenerate a fresh presigned URL on the redirect page (a meta-refresh or JS redirect), but this is out of scope for v1.

The 60-minute TTL is sufficient for the common case: user clicks link → user's browser begins download → download completes within 60 minutes.

### Env vars to document in `.env.example`

```
# Presigned URL TTLs (minutes)
PRESIGN_UPLOAD_TTL_MINUTES=15
PRESIGN_DOWNLOAD_TTL_MINUTES=60
```

## Constraints and risks

- **Clock skew between services:** If FileUpload's clock is ahead of S3's clock by more than a few minutes, presigned URLs may be rejected as expired even when the client uses them within the TTL. Mitigation: run `ntpd` or `chronyd` on all containers; S3/MinIO tolerates ~5 minutes of clock skew by default.
- **Large files and network latency:** A 100 MiB file uploaded over a 500 Kbps connection takes ~27 minutes — exceeding the 15-minute PUT TTL. Mitigation: the `MAX_FILE_SIZE` env var should be tuned to match the upload TTL and expected client bandwidth. Alternatively, increase `PRESIGN_UPLOAD_TTL_MINUTES` in environments with slow clients.
- **GET TTL and shared links:** If a short link is shared publicly and the first user's download takes >60 minutes (e.g., paused/resumed), the URL may expire. This is an accepted limitation for v1. A future enhancement could generate multiple presigned URLs or use S3's range-request capability.
- **Leaked PUT URLs within the TTL window:** If a client-side attacker intercepts the presigned PUT URL (e.g., from browser dev tools, HTTP logs), they have a 15-minute window to overwrite the file. Mitigation: the `ConfirmUpload` HEAD check can verify that the S3 object was created with the expected Content-MD5 (computed from the InitiateUpload metadata). This is a hardening step for v2.
