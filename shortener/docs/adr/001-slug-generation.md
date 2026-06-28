# ADR-001: Slug Generation

## Decision

Generate 6-character case-sensitive slugs from `crypto/rand`. Read 6 bytes, base62-encode,
yield the first 6 characters. On database uniqueness violation, retry up to 5 times.
If all 5 retries collide, return an error to the caller.

## Context

- GOBOX_SPEC.md §5.4 specifies: "6-character base62 random string, collision retry up to
  5 times; fail with error if exhausted; slugs are case-sensitive."
- The slug is the public-facing identifier in the short URL (e.g. `https://gobox.example/s/aB3kR9`).
- It must be short (6 chars), URL-safe, and unpredictable (no sequential IDs or hash prefixes
  that leak information about the file or user).
- It must be unique in the `short_links` table (database-level UNIQUE constraint).
- The redirect endpoint `GET /s/{slug}` does not require authentication, so slugs must not
  be guessable or enumerable.

## Options considered

### 1. crypto/rand + base62 (chosen)

Read 6 bytes from `crypto/rand` (48 bits of entropy), encode with base62 alphabet
(`[0-9A-Za-z]`), take first 6 characters. On DB duplicate-key error, retry.

- **Pro:** Simple, no extra dependencies, pure stdlib. 48 bits of entropy → ~2^48 possible
  values → collision probability for 10M slugs is ~0.02% (birthday bound
  ≈ N² / 2^49 ≈ 10^14 / 5.6e14 ≈ 0.18). Acceptable.
- **Pro:** `crypto/rand` is kernel-backed (getrandom on Linux) — not predictable from
  process state.
- **Pro:** base62 avoids URL-unsafe characters (`+` and `/` present in base64).
- **Con:** Retry logic adds a write-path latency tail if collisions spike (unlikely at
  <10M slugs).
- **Verdict:** **Chosen.**

### 2. crypto/rand + hex (16 chars)

Read 8 bytes, encode as hex → 16-character slug.

- **Pro:** No collision risk for practical scale (64 bits of entropy).
- **Con:** Slug is 2.7× longer than the 6-char requirement.
- **Verdict:** Rejected — violates the "short URL" constraint.

### 3. Base64 URL-safe (crypto/rand + RawURLEncoding)

Read 6 bytes, encode with `base64.RawURLEncoding` → 8 characters.

- **Pro:** Stdlib, no custom alphabet.
- **Con:** Output is 8 chars (33% longer) and contains `-` and `_` which, while URL-safe,
  are visually ambiguous in some fonts. Also violates 6-char spec.
- **Verdict:** Rejected.

### 4. Sequential ID + hashid / obfuscation (e.g. hashids)

Use a database sequence and encode with a third-party library like `hashids`.

- **Pro:** No collision — slugs are deterministic from an auto-increment ID.
- **Con:** Predictable if the salt is leaked. Adds a dependency. Slugs are enumerable
  (sequential IDs are easy to brute-force even with obfuscation).
- **Con:** The spec says "random string" — sequential IDs are the opposite of random.
- **Verdict:** Rejected.

### 5. Pre-generate and batch-insert slugs

Maintain a worker that pre-generates slugs and inserts them into a pool table, then
assign from the pool on CreateLink.

- **Pro:** Eliminates write-path collision retry entirely.
- **Con:** Adds a background worker, a pool table, a starvation failure mode, and
  operational complexity. Overkill for this scale.
- **Verdict:** Rejected.

## Chosen approach

### Algorithm (pseudocode)

```
func GenerateSlug() string {
    buf := make([]byte, 6)
    _, _ = rand.Read(buf)                    // crypto/rand, never fails on Linux
    n := binary.BigEndian.Uint64(buf[:])     // interpret as 48-bit integer
    return base62Encode(n)[:6]               // encode, take first 6 chars
}

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func base62Encode(n uint64) string {
    // standard base62 encoding — 6 chars needed for 48 bits
}
```

### Collision retry (domain service layer)

```go
const maxSlugRetries = 5

type SlugGenerator interface {
    Generate(ctx context.Context) (string, error)
}

// In the create-link use case:
for range maxSlugRetries {
    slug := gen.Generate(ctx)
    link := &ShortLink{Slug: slug, ...}
    err := repo.Create(ctx, link)
    if err == nil {
        return link, nil
    }
    if !errors.Is(err, ErrDuplicateSlug) {
        return nil, fmt.Errorf("create link: %w", err)
    }
    // collision — retry
}
return nil, ErrSlugCollisionExhausted
```

### Database constraint

The `short_links` table enforces uniqueness at the database level:

```sql
CREATE TABLE short_links (
    -- ...
    slug       VARCHAR(6)  NOT NULL,
    -- ...
    CONSTRAINT uq_short_links_slug UNIQUE (slug)
);
```

This is the collision-detection mechanism — no separate "check and insert" race window.

### Case sensitivity

- Slugs are stored and compared **as-is** (case-sensitive).
- PostgreSQL string comparison with `=` is case-sensitive by default for `VARCHAR`.
- Redis lookup is case-sensitive by default.
- The redirect handler does **not** normalize the slug before lookup.

## Constraints and risks

- **Entropy exhaustion at extreme scale:** At 10M slugs, collision probability on
  a single insert is ~0.18% per attempt, and with 5 retries it drops to near zero.
  If the service grows to 100M+ slugs, a longer slug (8 chars) should be adopted.
- **No custom alphabet encoding in stdlib:** Go does not ship a base62 encoder.
  The implementation must be hand-written (or a small utility) — this is trivial
  but must be verified for correctness.
- **crypto/rand never blocks on Linux:** Since Go 1.22+ uses `getrandom()` with
  `GRND_NONBLOCK` internally, `rand.Read` cannot block. On older kernels or
  Docker with low-entropy, it could block — but this is not a concern on any
  modern Linux distro.
- **Slug must be indexed:** The `slug` column needs a unique index or constraint
  for fast lookup in both Postgres and Redis.

## References

- GOBOX_SPEC.md §5.4 — "Link Shortener", "Slug generation"
- Go `crypto/rand` documentation
- Base62 alphabet: `[0-9A-Za-z]` (62 characters)
