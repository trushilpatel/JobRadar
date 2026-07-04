-- +goose Up
CREATE TABLE resumes (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label             TEXT        NOT NULL,
    original_filename TEXT        NOT NULL,
    storage_key       TEXT        NOT NULL,
    content_type      TEXT        NOT NULL CHECK (content_type IN (
                          'application/pdf',
                          'application/vnd.openxmlformats-officedocument.wordprocessingml.document'
                      )),
    size_bytes        BIGINT      NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 2097152),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_resumes_storage_key ON resumes(storage_key);
CREATE INDEX idx_resumes_user_id_created_at ON resumes(user_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS resumes;
