-- +goose Up
CREATE TABLE crawler_nodes (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT        NOT NULL UNIQUE,
    token_prefix  CHAR(8)     NOT NULL,
    token_hash    TEXT,
    status        TEXT        NOT NULL DEFAULT 'active'
                              CHECK (status IN ('active', 'disabled', 'retired')),
    last_seen_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_crawler_nodes_token_prefix ON crawler_nodes (token_prefix);
CREATE INDEX idx_crawler_nodes_status       ON crawler_nodes (status);

-- +goose Down
DROP TABLE crawler_nodes;
