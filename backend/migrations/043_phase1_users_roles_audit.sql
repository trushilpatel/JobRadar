-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id           UUID         PRIMARY KEY,
    email        CITEXT       NOT NULL UNIQUE,
    display_name TEXT,
    status       TEXT         NOT NULL DEFAULT 'active'
                              CHECK (status IN ('active', 'suspended')),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE roles (
    id          TEXT         PRIMARY KEY,
    name        TEXT         NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE user_roles (
    user_id     UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id     TEXT         NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    granted_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    granted_by  UUID         REFERENCES users(id) ON DELETE SET NULL,
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_user_roles_role_id ON user_roles(role_id);

CREATE TABLE admin_audit_log (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id  UUID         REFERENCES users(id) ON DELETE SET NULL,
    action         TEXT         NOT NULL,
    target_type    TEXT,
    target_id      TEXT,
    payload        JSONB,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_admin_audit_log_actor_created
    ON admin_audit_log(actor_user_id, created_at DESC);
CREATE INDEX idx_admin_audit_log_action_created
    ON admin_audit_log(action, created_at DESC);

INSERT INTO roles (id, name, description) VALUES
    ('user',  'Regular User',  'Default role for every signed-up user'),
    ('admin', 'Administrator', 'Full access to infrastructure and settings');

-- +goose Down
DROP TABLE IF EXISTS admin_audit_log;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;
