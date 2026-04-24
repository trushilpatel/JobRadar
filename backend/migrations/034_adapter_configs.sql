-- +goose Up
CREATE TABLE adapter_configs (
    adapter_type            TEXT PRIMARY KEY,
    scan_interval_minutes   INT  NOT NULL DEFAULT 1440 CHECK(scan_interval_minutes > 0),
    inter_company_delay_s   INT  NOT NULL DEFAULT 5    CHECK(inter_company_delay_s >= 0),
    last_scheduled_at       TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO adapter_configs (adapter_type, scan_interval_minutes, inter_company_delay_s) VALUES
    ('greenhouse', 1440, 3),
    ('lever',      1440, 3),
    ('ashby',      1440, 3),
    ('netflix',    1440, 5),
    ('uber',       1440, 5),
    ('microsoft',  1440, 3),
    ('rippling',   1440, 3),
    ('apple',      1440, 3),
    ('google',     1440, 30),
    ('meta',       1440, 30)
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS adapter_configs;
