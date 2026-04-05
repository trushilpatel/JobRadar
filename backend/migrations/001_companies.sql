-- +goose Up
CREATE TABLE companies (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    url               TEXT NOT NULL,
    adapter_type      TEXT NOT NULL
                          CHECK(adapter_type IN (
                              'greenhouse', 'lever', 'ashby',
                              'workday', 'workable', 'chromedp'
                          )),
    active            BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_scan_at      TIMESTAMPTZ,
    last_scan_status  TEXT
                          CHECK(last_scan_status IN ('ok', 'error', 'pending'))
);

-- +goose Down
DROP TABLE companies;
