# ADR-001 (File Upload): Upload Flow — Direct-to-S3 with Presigned URLs

## Decision

The File Upload Center implements a **two-phase upload flow** that never touches the service's own network path with binary data:

1. **InitiateUpload** — Client requests a file upload. The service creates a metadata record (`status=pending`) and returns a presigned S3 PUT URL.
2. **Client uploads directly to S3** using the presigned URL. The binary stream goes from client to S3 only.
3. **ConfirmUpload** — Client notifies the service that the upload is complete. The service verifies the object exists in S3, then sets `status=ready`.

Core API never receives or forwards file bytes.

## Context

Per GOBOX_SPEC.md §5.3, the File Upload Center is an internal gRPC-only service (port 9090). All client interactions go through Core API (REST gateway). The upload flow diagram in the spec shows:

```
Client → Core API (POST /files)
  → FileUpload gRPC: InitiateUpload
    ← presigned S3 PUT URL + file_id
  ← 202 Accepted: { file_id, upload_url }

Client → S3/MinIO (PUT upload_url, raw bytes)
  ← 200 OK from S3

Client → Core API (POST /files/{id}/confirm)
  → FileUpload gRPC: ConfirmUpload
    ← FileResponse (status=ready)
  ← 200 OK: file metadata

Core API → ThumbGen gRPC: EnqueueJob (async, fire-and-forget)
```

Three design questions must be resolved:
1. Should Core API proxy the binary upload, or should the client upload directly to S3?
2. What are the semantics of `pending` vs `ready` vs `failed` status?
3. How does the service verify that the client actually uploaded the bytes before marking a file `ready`?

## Options considered

### Option 1: Direct-to-S3 with presigned URLs (chosen)

Client uploads directly to S3/MinIO using a time-limited presigned PUT URL. The service only manages metadata.

- **Pros:** No binary data passes through the service; memory and bandwidth usage stays low regardless of file size; S3 handles multipart for large files natively; scales horizontally without a throughput bottleneck.
- **Cons:** Requires the client to make two API calls (initiate + confirm); S3 must be reachable from the client (in-cluster vs public depends on network topology); presigned URL TTL management.

### Option 2: Core API proxies the binary

Core API receives the multipart upload, streams it to FileUpload via gRPC, and FileUpload writes it to S3.

- **Pros:** Single API call for the client; S3 access key is never exposed outside the cluster; easier to add post-upload processing.
- **Cons:** Doubles the data path — every byte passes through two services (Core and FileUpload), saturating their memory and bandwidth; Core API becomes a throughput bottleneck; violates the spec's thin-gateway design for Core (§5.2: "Core API is a thin orchestrator"); gRPC streaming adds complexity.

### Option 3: FileUpload receives multipart directly over HTTP

FileUpload exposes its own HTTP endpoint for multipart uploads, separate from gRPC.

- **Pros:** Single service handles the upload; client doesn't need direct S3 access.
- **Cons:** Violates the spec's requirement that FileUpload is "gRPC only — port 9090" (§5.3: "Public port: none (internal gRPC only)"); adds a second transport and a public port; duplicates HTTP handling that Core already provides.

## Chosen approach

**Option 1 — Direct-to-S3 with presigned URLs.**

### Detailed flow (step by step)

#### Step 1: InitiateUpload (gRPC)

**Request:**
```protobuf
message InitiateUploadRequest {
  string user_id   = 1;  // extracted from JWT by Core, forwarded in gRPC metadata
  string name      = 2;  // original filename
  int64  size      = 3;  // declared file size
  string mime_type = 4;  // MIME type, optional
}
```

**Service action:**
1. Validate inputs: filename not empty, size > 0, size does not exceed `MAX_FILE_SIZE` (env var, default 100 MiB).
2. Generate `file_id` (uuid v4).
3. Compute `storage_key` = `uploads/{user_id}/{file_id}/{name}` (see ADR-002).
4. Insert a `File` record into Postgres with `status=pending`.
5. Generate a presigned S3 PUT URL for `storage_key` with TTL = `PRESIGN_UPLOAD_TTL_MINUTES` (default 15, see ADR-003).
6. Return `{ file_id, upload_url }`.

**Response:**
```protobuf
message InitiateUploadResponse {
  string file_id    = 1;
  string upload_url = 2;  // presigned S3 PUT URL
}
```

Core API maps this to `202 Accepted` (not 200 — the upload hasn't happened yet).

#### Step 2: Client uploads to S3

The client performs `HTTP PUT {upload_url}` with the raw file bytes as the request body. S3 returns `200 OK` (or `201 Created`) on success.

- The client **must** set `Content-Type` to the same MIME type declared in InitiateUpload (S3 does not enforce this; the service validates it in ConfirmUpload).
- Multipart upload (for files > 5 GiB) is handled by the client directly against S3's multipart API. The presigned URL covers the first part; subsequent parts require additional presigned URLs. This is an optimization outside this ADR's scope.

#### Step 3: ConfirmUpload (gRPC)

**Request:**
```protobuf
message ConfirmUploadRequest {
  string file_id = 1;
  string user_id = 2;  // for ownership verification
}
```

**Service action:**
1. Look up the `File` record by `file_id` and `user_id`.
2. Verify `status == "pending"`. If already `ready`, return success (idempotent). If `failed`, return `FailedPrecondition`.
3. Perform a **HEAD request** to S3/MinIO for `storage_key`:
   - **If object exists:** Compare `Content-Length` and `Content-Type` with the values declared in InitiateUpload. On mismatch, set `status=failed` and return `InvalidArgument`. On match, update `size` (actual), `mime_type` (actual), set `status=ready`, return `FileResponse`.
   - **If object does NOT exist:** Return `NotFound` with message "upload not yet completed". The client should retry after a short delay. This handles the race between S3 write propagation and ConfirmUpload.
4. On success, Core API fires a fire-and-forget gRPC call to ThumbGen's `EnqueueJob` (async, not blocking the response).

### Status lifecycle

```
InitiateUpload → status=pending
                     ↓
              Client uploads to S3 (external)
                     ↓
              ConfirmUpload → S3 HEAD check
                  /             \
            object exists    object missing
                 |                 |
            status=ready     return NotFound
                 |            (client retries)
            Core enqueues
            ThumbGen job (async)
```

- **`pending`:** The metadata record exists but no bytes have been verified in S3. The presigned URL is valid. No other operation (GetFile, GetDownloadURL) returns the file while it is pending. The file is invisible to ListFiles (filtered out).
- **`ready`:** The file has been verified in S3 and metadata is complete. All read operations (GetFile, ListFiles, GetDownloadURL) work normally.
- **`failed`:** The S3 HEAD check found a size/MIME mismatch, or the object never arrived before the presigned URL expired. The file record is kept for auditing but is invisible to normal list operations.

### Why Core API does NOT proxy the binary

Three reasons:

1. **Memory saturation:** A 100 MiB file passing through Core means Core must allocate at least 100 MiB of buffer per concurrent upload. With 100 concurrent uploads: 10 GiB of heap pressure. With direct-to-S3, Core handles only tiny JSON/gRPC payloads (~500 bytes per request).

2. **Bandwidth bottleneck:** Core runs on a single VM/container. If it proxies all uploads, its network throughput becomes the limit. Direct-to-S3 uses S3's own multi-Gbps bandwidth with no intermediary.

3. **Spec alignment:** GOBOX_SPEC.md §5.2 defines Core as "a thin orchestrator" and "stateless gateway." Proxying binary data contradicts this role. The spec's own upload diagram (§5.3) shows the client talking directly to S3.

### S3 access

- FileUpload (not Core) holds the S3 credentials (`S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`).
- Presigned URLs are generated using MinIO's SDK (`PresignPutObject`, `PresignGetObject`).
- The S3 client is initialized once at startup in `internal/infrastructure/s3/`.

### Cleanup of stale pending records

A periodic cleanup goroutine (configurable interval, default every 1 hour) deletes/sets-to-failed `File` records that have been `pending` for longer than `PRESIGN_UPLOAD_TTL_MINUTES + 5` (grace period). This prevents metadata accumulation from abandoned uploads.

## Constraints and risks

- **Client-side S3 access:** The client must be able to reach the S3/MinIO endpoint. In dev (Docker Compose), MinIO is on the same Docker network; presigned URLs may resolve to internal IPs. Mitigation: configure `S3_PUBLIC_ENDPOINT` env var that overrides the endpoint used in presigned URL generation (defaults to `S3_ENDPOINT` for in-cluster access).
- **S3 HEAD consistency:** S3 read-after-write consistency for new PUTs is guaranteed for most regions, but eventual consistency may cause ConfirmUpload to return `NotFound` immediately after a successful PUT. The client must retry with backoff (recommended: 3 retries, 1s / 2s / 4s delay).
- **Idempotency:** ConfirmUpload must be idempotent. A duplicate ConfirmUpload (e.g., from client retry) should not fail. The service checks `status == ready` and returns success without re-verifying S3.
- **No upload progress:** The service has no way to track upload progress. If the client abandons the upload, the record remains `pending` until the cleanup goroutine removes it. Acceptable for v1.
- **Maximum file size:** Default 100 MiB. Controlled by `MAX_FILE_SIZE` env var. Files exceeding this are rejected at InitiateUpload. Larger files require multipart upload planning (out of scope for this ADR).
