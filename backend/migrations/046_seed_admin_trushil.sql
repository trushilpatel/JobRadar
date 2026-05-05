-- +goose Up
-- Grant admin role to trushilpatel9898@gmail.com.
-- Inserts a user_roles row only if the user already exists (lookup by email).
-- Safe to re-run: ON CONFLICT DO NOTHING.
INSERT INTO user_roles (user_id, role_id, granted_by)
SELECT u.id, 'admin', NULL
FROM   users u
WHERE  u.email = 'trushilpatel9898@gmail.com'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM user_roles
USING  users u
WHERE  user_roles.user_id = u.id
  AND  u.email = 'trushilpatel9898@gmail.com'
  AND  user_roles.role_id = 'admin';
