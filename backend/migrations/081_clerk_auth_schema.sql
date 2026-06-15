-- DESTRUCTIVE: truncates all user data. Only safe because we start fresh with Clerk.
-- +goose Up
-- +goose StatementBegin

-- Truncate all user-linked tables (CASCADE handles any remaining referential deps).
TRUNCATE TABLE job_application_tracker, user_job_state, user_job_counters, admin_audit_log, user_roles, users RESTART IDENTITY CASCADE;

-- Drop RBAC tables — roles are now carried in Clerk JWT claims (publicMetadata.role).
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS roles;

-- Drop FK constraints referencing users.id before changing its type.
-- PostgreSQL requires all referencing FK columns to match the referenced column type.
ALTER TABLE user_job_state          DROP CONSTRAINT IF EXISTS user_job_state_user_id_fkey;
ALTER TABLE user_job_counters       DROP CONSTRAINT IF EXISTS user_job_counters_user_id_fkey;
ALTER TABLE job_application_tracker DROP CONSTRAINT IF EXISTS job_application_tracker_user_id_fkey;
ALTER TABLE admin_audit_log         DROP CONSTRAINT IF EXISTS admin_audit_log_actor_user_id_fkey;
ALTER TABLE user_preferences        DROP CONSTRAINT IF EXISTS user_preferences_user_id_fkey;

-- Alter users.id from UUID to TEXT (Clerk user IDs are strings like "user_2abc...").
ALTER TABLE users ALTER COLUMN id TYPE TEXT USING id::TEXT;

-- Alter FK columns on user-linked tables to match the new TEXT type.
ALTER TABLE user_job_state          ALTER COLUMN user_id       TYPE TEXT USING user_id::TEXT;
ALTER TABLE user_job_counters       ALTER COLUMN user_id       TYPE TEXT USING user_id::TEXT;
ALTER TABLE job_application_tracker ALTER COLUMN user_id       TYPE TEXT USING user_id::TEXT;
ALTER TABLE admin_audit_log         ALTER COLUMN actor_user_id TYPE TEXT USING actor_user_id::TEXT;
ALTER TABLE user_preferences        ALTER COLUMN user_id       TYPE TEXT USING user_id::TEXT;

-- Re-add FK constraints with the original ON DELETE behaviour.
ALTER TABLE user_job_state          ADD CONSTRAINT user_job_state_user_id_fkey          FOREIGN KEY (user_id)       REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE user_job_counters       ADD CONSTRAINT user_job_counters_user_id_fkey       FOREIGN KEY (user_id)       REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE job_application_tracker ADD CONSTRAINT job_application_tracker_user_id_fkey FOREIGN KEY (user_id)       REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE admin_audit_log         ADD CONSTRAINT admin_audit_log_actor_user_id_fkey   FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE user_preferences        ADD CONSTRAINT user_preferences_user_id_fkey        FOREIGN KEY (user_id)       REFERENCES users(id) ON DELETE CASCADE;

-- +goose StatementEnd

-- +goose Down
-- Irreversible: user data was truncated and roles table was dropped.
SELECT 1;
