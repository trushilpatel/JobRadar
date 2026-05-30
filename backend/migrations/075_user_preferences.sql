-- Migration 075: user_preferences table
-- Stores per-user application configuration as a JSONB blob. Future preference
-- fields are added by the application layer (new Go struct fields) — no
-- further schema migrations are needed.

-- +goose Up
CREATE TABLE user_preferences (
    user_id    UUID        NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    data       JSONB       NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS user_preferences;
