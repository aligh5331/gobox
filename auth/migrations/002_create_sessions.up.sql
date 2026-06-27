CREATE TABLE IF NOT EXISTS sessions (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token  TEXT        NOT NULL,
    user_agent     TEXT        NOT NULL DEFAULT '',
    ip             VARCHAR(45) NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at     TIMESTAMPTZ NOT NULL,
    revoked        BOOLEAN     NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
