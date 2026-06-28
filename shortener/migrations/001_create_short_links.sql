-- 001_create_short_links.sql
-- Creates the short_links table for the Shortener service.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS short_links (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id     UUID NOT NULL,
    user_id     UUID NOT NULL,
    slug        VARCHAR(6)  NOT NULL,
    target_url  TEXT NOT NULL DEFAULT '',
    hit_count   BIGINT NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_short_links_slug ON short_links (slug);
CREATE INDEX idx_short_links_user_id ON short_links (user_id);
CREATE INDEX idx_short_links_expires_at ON short_links (expires_at);
