# ADR-002 (File Upload): S3 Storage Key Convention and Soft-Delete

## Decision

1. **S3 object key format:** `uploads/{user_id}/{file_id}/{filename}`  
   - `{user_id}` — the UUID of the owning user (from JWT `sub` claim).
   - `{file_id}` — the UUID assigned to this file record at InitiateUpload.
   - `{filename}` — the original filename as provided by the client.
2. **Soft-delete for DB records:** A `deleted_at` timestamp column is added to the `File` table. The `DeleteFile` use case sets `deleted_at` to the current time (soft-delete) and enqueues an asynchronous goroutine to remove the object from S3.
3. **Async S3 deletion:** After the DB record is soft-deleted, a background goroutine performs the S3 `RemoveObject` call. If S3 deletion fails, the error is logged and the object is orphaned in S3. A future garbage-collection job (out of scope for v1) can reconcile orphaned objects.

## Context

GOBOX_SPEC.md §5.3 defines the File model with a `storage_key` field:

```
storage_key   string    (object key in S3/MinIO)
```

The spec does not specify the key format. Two architectural concerns arise:

1. **Key uniqueness and discoverability:** The key must be globally unique across all users (S3 is a flat namespace per bucket) and must allow the service to locate the object for download or deletion without storing extra state.
2. **Delete semantics:** The spec lists `DeleteFile` as a use case but does not specify whether deletion is physical (immediate S3 removal) or logical (soft-delete). The service must handle the tension between "fast response to user" and "reliable S3 cleanup."

Additionally, two side effects depend on the key format:
- The Thumbnail Generator (§5.5) reads the source file from S3 using `input_key` — it needs a deterministic key to locate the file.
- The presigned GET URL generator needs the exact key to produce a download link.

## Options considered

### Key format

#### Option 1: `uploads/{user_id}/{file_id}/{filename}` (chosen)

Hierarchical prefix by user, then by file, then by display name.

- **Pros:** Namespace isolation — one user's keys never collide with another's; easy S3 console browsing by prefix (`uploads/{user_id}/`); `{file_id}` provides uniqueness even if filename conflicts exist; filename preserved for human readability in storage; deterministic from the File record alone.
- **Cons:** Longer key strings (potential minor cost in S3 GET requests which are priced by key length — negligible).

#### Option 2: `uploads/{file_id}`

Flat key by file UUID only.

- **Pros:** Shortest possible key; no repeated user_id prefix in storage.
- **Cons:** Impossible to browse by user in S3 console; loses original filename; requires the service to map file_id → key via DB on every request (already stored in `storage_key`, so this is a minor point).

#### Option 3: `uploads/{user_id}/{hash}/{filename}`

Use a content hash (e.g., SHA-256 of the first few bytes) to deduplicate identical files across uploads.

- **Pros:** Storage deduplication — identical files uploaded by the same user map to the same object.
- **Cons:** Adds complexity at upload time (hash computation before the object is written); content-addressed storage conflicts with the "update in place" mental model; deduplication is not a spec requirement; potential security side-channel (hash reveals content equality).

### Delete semantics

#### Option 1: Logical soft-delete + async S3 removal (chosen)

DB record gets `deleted_at` set. A goroutine removes the S3 object asynchronously.

- **Pros:** Response to the client is fast (DB update only, no S3 latency); if S3 is unavailable, the record is still soft-deleted and the cleanup retries on the next goroutine run; delete is reversible within a grace window (admin only).
- **Cons:** S3 object persists briefly after the user sees "deleted"; goroutine failure (crash before S3 call) orphans the object; requires a periodic reconciliation job for robustness.

#### Option 2: Synchronous S3 deletion + hard DB delete

Delete the S3 object first, then delete the DB row in a transaction.

- **Pros:** No orphans — S3 and DB are always consistent at the point of the API response.
- **Cons:** Client waits for S3 latency (50–200 ms); if S3 is down, the delete API call fails entirely; no undo capability; violates the principle of failing fast for user-facing operations.

#### Option 3: S3 lifecycle policy (no code)

Set an S3 lifecycle rule that purges objects with tag `deleted=true` once a day. The service tags the object and deletes the DB record.

- **Pros:** No goroutine management in the service; S3 handles cleanup reliably.
- **Cons:** Tags require a separate PUT call to S3 (same latency as Option 2); lifecycle rules execute on S3's schedule (once per 24h); adds infrastructure dependency on lifecycle configuration; no hard-delete on demand.

## Chosen approach

**Key format:** `uploads/{user_id}/{file_id}/{filename}`  
**Delete:** Soft-delete with `deleted_at` + async S3 removal via goroutine.

### Detailed design

#### Key format specification

```
uploads/{user_id}/{file_id}/{filename}
```

Where:
- `{user_id}` — Lowercase UUID string (36 chars) from JWT `sub` claim, e.g., `550e8400-e29b-41d4-a716-446655440000`.
- `{file_id}` — Lowercase UUID v4 (36 chars) generated by the service at InitiateUpload, e.g., `f47ac10b-58cc-4372-a567-0e02b2c3d479`.
- `{filename}` — The original filename as provided by the client. Contains NO path separators (`/` or `\`). Filenames with path separators are rejected at InitiateUpload (return `InvalidArgument`). Non-ASCII characters are percent-encoded by the S3 client library automatically.

**Example:**
```
uploads/550e8400-e29b-41d4-a716-446655440000/f47ac10b-58cc-4372-a567-0e02b2c3d479/report_q3_2026.pdf
```

The format is **deterministic**: given the File record, the key is computable without any additional state. ThumbGen can construct the `input_key` for its `EnqueueJobRequest` from the File metadata returned by FileUpload's gRPC response.

**Rejected filenames:**
- Empty string
- Contains `/` or `\`
- Exceeds 255 bytes (OS filename limit)
- Reserved S3 characters are handled by the MinIO SDK's URL encoding

#### File table schema

```sql
CREATE TABLE files (
    id          UUID PRIMARY KEY,
    user_id     UUID NOT NULL,
    name        VARCHAR(255) NOT NULL,
    size        BIGINT NOT NULL DEFAULT 0,
    mime_type   VARCHAR(127) NOT NULL DEFAULT 'application/octet-stream',
    storage_key TEXT NOT NULL,
    status      VARCHAR(16) NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'ready', 'failed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ              -- soft-delete column; NULL = active
);

CREATE INDEX idx_files_user_id ON files (user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_files_status   ON files (status);
```

- `deleted_at` is `NULL` for active files, `NOT NULL` for soft-deleted files.
- All read queries (`ListFiles`, `GetFile`) include `WHERE deleted_at IS NULL`.
- The `status` column is independent of `deleted_at`. A `ready` file can be soft-deleted — the status indicates upload completion, not availability.

#### DeleteFile use case flow

```
Client → Core API (DELETE /api/v1/files/{id})
  → FileUpload gRPC: DeleteFile({file_id, user_id})
    1. Lookup File by id + user_id, ensure deleted_at IS NULL
    2. Set deleted_at = NOW()
    3. Save record
    4. Return Empty

  ← Empty

  (goroutine, fire-and-forget in the use case layer:)
  5. Call s3Client.RemoveObject(ctx, bucket, storage_key)
  6. On error: log.Error("failed to remove S3 object", "key", storage_key, "error", err)
  7. On success: log.Info("S3 object removed", "key", storage_key)
```

**Why the goroutine runs in the use case layer, not in a handler:**
- The use case owns the post-delete cleanup logic.
- If the process crashes between step 4 (DB update) and step 5 (S3 remove), the object is orphaned. This is acceptable for v1. A reconciliation job (out of scope) can later scan for `deleted_at IS NOT NULL` records and check S3 for orphaned objects.

**Goroutine safety:**
- The goroutine uses `context.Background()` (not the request context, which may be cancelled).
- It uses a `sync.WaitGroup` in the use case struct to track in-flight deletions for graceful shutdown (see Constraints and risks).
- Maximum 10 concurrent deletion goroutines (semaphore channel, configurable via env var `MAX_CONCURRENT_DELETIONS`).

#### S3 orphan detection (v1 scope, not implemented yet)

A future `cmd/orphan-cleaner` can be built that:
1. Queries `files` WHERE `deleted_at IS NOT NULL` AND `created_at < NOW() - INTERVAL '24 hours'`.
2. For each record, checks if the S3 object exists.
3. If it exists, removes it.
4. Optionally hard-deletes the DB record if S3 removal succeeds and the record is older than `PURGE_AFTER_DAYS` (30 days default).

This is explicitly out of scope for v1.

### Package structure

```
fileupload/
└── internal/
    ├── domain/
    │   └── model/
    │       └── file.go               ← File struct with Status, DeletedAt fields
    ├── infrastructure/
    │   └── s3/
    │       └── client.go             ← MinIO client, PresignPutObject, PresignGetObject, RemoveObject
    └── application/
        └── usecase/
            ├── initiate_upload.go    ← constructs storage_key, inserts File
            ├── confirm_upload.go     ← S3 HEAD check, sets status=ready
            ├── delete_file.go        ← soft-delete, enqueue async S3 removal
            └── get_download_url.go   ← presign GET for storage_key
```

## Constraints and risks

- **Orphaned S3 objects:** If the service crashes between the DB soft-delete and the S3 `RemoveObject` call, the object persists indefinitely. Mitigation: implement the orphan reconciliation job before production deployment. For v1, this is an accepted risk.
- **Filename collisions are impossible:** Because `{file_id}` is a UUID, two files with the same name uploaded by the same user always have different keys. There is no risk of overwrite.
- **Path traversal:** A malicious filename like `../../etc/passwd` could produce a key outside the expected prefix. Mitigation: filenames containing `/` or `\` are rejected at InitiateUpload. The MinIO SDK further sanitizes the key.
- **Long keys:** The full key is ~150 characters. S3 pricing includes key length in request cost, but the difference between 150 and 50 characters is ~0.0000001 USD per 10,000 requests. Negligible.
- **Graceful shutdown:** The goroutine pool must complete in-flight deletions before the service exits. The use case layer exposes a `Shutdown()` method that waits on the `sync.WaitGroup` with a timeout (default 10 seconds).
