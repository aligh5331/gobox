# ADR-002: Session Lifecycle

## Decision
Use a single `Session` table to track active login sessions. Refresh tokens are rotated on every use via an atomic delete-then-create within a database transaction. LogoutAll unconditionally sets `revoked = true` on all sessions for a user.

## Context
- Auth service owns the session database (Postgres via GORM).
- Sessions are created on Login and Register (implicit login).
- Logout and LogoutAll revoke sessions; access tokens expire via short 15-minute TTL.
- Refresh tokens are opaque random byte strings, bcrypt-hashed in the `Session` table.
- Other services never query Auth's session DB — they validate only the JWT signature and expiry.

## Options considered

### 1. Soft-delete rotation (mark old session revoked, create new)
On refresh: set `revoked = true` on the old session, then INSERT a new session row.
- **Pro:** Audit trail — old session is retained with `revoked` flag and timestamp.
- **Con:** Two writes instead of one; old row is "zombie" for the overlap. Refresh token theft becomes detectable but not preventable — the stolen old token is still valid until this refresh happens.
- **Verdict:** Rejected — the atomic DELETE+INSERT approach (chosen option) is simpler and provides the same security property (token theft detection) with fewer rows.

### 2. Atomic delete-then-create (chosen)
On refresh: BEGIN transaction → DELETE session WHERE id = old_session_id → INSERT new session → COMMIT.
- **Pro:** Exactly one session row per active refresh token. No zombie rows. If the old refresh token is used again (stolen token scenario), the DELETE will affect zero rows (already deleted) and the refresh fails cleanly.
- **Con:** Loses the old session row entirely. Acceptable — the audit trail lives in application logs, not the session table.
- **Verdict:** **Chosen approach.**

### 3. Token family chain (linked by a token_family_id)
Each token family has a chain: `id1 → id2 → id3` in a separate table. Rotation writes a new row linked to the previous.
- **Pro:** Full lineage for detecting token theft (reuse of an older token in the chain).
- **Con:** Significant schema and code complexity. Not needed for v1.
- **Verdict:** Defer to post-v1 if token theft detection becomes a hard requirement.

## Chosen approach

### Session creation triggers

| Trigger | Action |
|---------|--------|
| `Register` use case | Create Session immediately (user is logged in after registration). |
| `Login` use case | Create Session on successful password verification. |
| `RefreshToken` use case | Delete old Session, create new Session (rotation). |

### Session schema (Postgres)

```sql
CREATE TABLE sessions (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token     TEXT        NOT NULL,         -- bcrypt hash of the opaque token
    user_agent        TEXT        NOT NULL DEFAULT '',
    ip                VARCHAR(45) NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at        TIMESTAMPTZ NOT NULL,
    revoked           BOOLEAN     NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
```

- `refresh_token` stores the bcrypt hash (cost 10) of the opaque refresh token, never the raw token.
- `expires_at` = `NOW() + 30 days` (matching refresh token TTL from the spec).
- Index on `user_id` for LogoutAll queries. Index on `expires_at` for potential cleanup jobs.

### Refresh token rotation (atomic)

1. Receive opaque refresh token from client.
2. Query Session by iterating over non-revoked, non-expired sessions for the user (note: we don't know which session the token belongs to yet — we must hash and compare).
   - **Performance note:** For v1 with reasonable session counts per user (<100), scanning and comparing bcrypt hashes is acceptable. If needed, a `refresh_token_hash_prefix` index could be added later.
3. Verify bcrypt hash matches.
4. Within a database transaction:
   - `DELETE FROM sessions WHERE id = <found_session_id>`
   - If `RowsAffected == 0`, fail (race or already deleted — treat as token theft, abort).
   - Generate new opaque refresh token (32 random bytes, base64url-encoded).
   - `INSERT INTO sessions (...) VALUES (new_uuid, user_id, bcrypt(new_token), ...)`
5. Commit transaction.
6. Issue new JWT (with new `sid`) and new refresh token to client.

**Token theft detection:** If a stolen old refresh token is used after rotation, the DELETE finds no row to delete (already deleted by the legitimate use). The refresh fails. The legitimate user's new session is unaffected — they only lose one refresh cycle, which they'll detect when their token is rejected and they need to re-login.

### Logout

- Input: `session_id` (from authenticated request — JWT `sid` claim).
- `UPDATE sessions SET revoked = TRUE WHERE id = <session_id>`
- No immediate invalidation of the access token (15 min TTL handles that).
- The session row remains in the DB with `revoked = TRUE` for the remainder of its natural expiry.

### LogoutAll

- Input: `user_id` (from authenticated request — JWT `sub` claim).
- `UPDATE sessions SET revoked = TRUE WHERE user_id = <user_id> AND revoked = FALSE`
- Same as Logout: access tokens expire on their own.
- No need to iterate or delete — a single bulk UPDATE.

### Repository interface (port)

```go
type SessionRepository interface {
    Create(ctx context.Context, session *Session) error
    FindByID(ctx context.Context, id uuid.UUID) (*Session, error)
    FindByUserID(ctx context.Context, userID uuid.UUID) ([]Session, error)
    Delete(ctx context.Context, id uuid.UUID) error
    Revoke(ctx context.Context, id uuid.UUID) error
    RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error

    // Rotate atomically deletes the old session and creates a new one.
    // Returns the new Session.
    Rotate(ctx context.Context, oldSessionID uuid.UUID, newSession *Session) (*Session, error)
}
```

## Constraints and risks

- **Bcrypt scanning for refresh:** Looking up a refresh token requires scanning the user's sessions and comparing bcrypt hashes. With a large number of sessions per user (>1000), this could be slow. Mitigation: add a cleanup routine to delete expired sessions, or introduce a `refresh_token_hash_prefix` index using the first 8 bytes of the hash as a discriminator. For v1, assumed acceptable.
- **LogoutAll is soft-revoke only.** There is no hard cutoff — actively held access tokens remain valid for up to 15 minutes. This is intentional per the spec.
- **No cron job for session cleanup.** Expired/revoked sessions accumulate. A periodic `DELETE FROM sessions WHERE expires_at < NOW() AND revoked = TRUE` should be added as a background goroutine or external cron, but is out of scope for v1.

## References

- GOBOX_SPEC.md §5.1 — "Session management" table and JWT claims
- GOBOX_SPEC.md §5.1 — Refresh token: "opaque random string, stored in Auth DB, TTL 30 days"
- RFC 6749 §10.4 — Refresh token rotation recommendation
