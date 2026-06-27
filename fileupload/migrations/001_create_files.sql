-- 001_create_files.sql
-- Creates the files table for the FileUpload service.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS files (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL,
    name        VARCHAR(255) NOT NULL,
    size        BIGINT NOT NULL DEFAULT 0,
    mime_type   VARCHAR(127) NOT NULL DEFAULT 'application/octet-stream',
    storage_key TEXT NOT NULL,
    status      VARCHAR(16) NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'ready', 'failed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX idx_files_user_id ON files (user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_files_status   ON files (status);
