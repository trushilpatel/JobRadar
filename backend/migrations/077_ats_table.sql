-- Deploy order: run this migration AFTER the new binary is live. The old
-- binary reads adapter_configs; running Up before rollout will crash the
-- running instance.

-- +goose Up

CREATE TABLE ats (
    name                   TEXT        PRIMARY KEY,
    display_name           TEXT        NOT NULL,
    base_url               TEXT        NOT NULL,
    is_active              BOOLEAN     NOT NULL DEFAULT true,
    is_admin_creatable     BOOLEAN     NOT NULL DEFAULT true,
    scan_interval_min      INTEGER     NOT NULL DEFAULT 1440 CHECK (scan_interval_min > 0),
    inter_company_delay_s  INTEGER     NOT NULL DEFAULT 3    CHECK (inter_company_delay_s >= 0),
    last_scheduled_at      TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO ats (name, display_name, base_url, is_active, is_admin_creatable, scan_interval_min, inter_company_delay_s) VALUES
    ('greenhouse', 'Greenhouse',  'https://boards-api.greenhouse.io/v1/boards/', true, true,  1440,  3),
    ('lever',      'Lever',       'https://jobs.lever.co/',                       true, true,  1440,  3),
    ('ashby',      'Ashby',       'https://jobs.ashbyhq.com/',                    true, true,  1440,  3),
    ('google',     'Google',      '',                                              true, true,  1440, 30),
    ('meta',       'Meta',        '',                                              true, true,  1440, 30),
    ('netflix',    'Netflix',     '',                                              true, true,  1440,  5),
    ('uber',       'Uber',        '',                                              true, true,  1440,  5),
    ('rippling',   'Rippling',    '',                                              true, true,  1440,  3),
    ('microsoft',  'Microsoft',   '',                                              true, true,  1440,  3),
    ('apple',      'Apple',       '',                                              true, true,  1440,  3),
    ('gem',        'Gem',         'https://jobs.gem.com/',                         true, true,  1440,  3),
    ('deel',       'Deel',        'https://jobs.deel.com/',                        true, true,  1440,  5),
    ('workday',    'Workday',     '',                                              true, true,  1440, 15),
    ('eightfold',  'Eightfold',   '',                                              true, false, 1440,  5)
ON CONFLICT DO NOTHING;

-- Re-point FKs: company_scan_settings → ats
ALTER TABLE company_scan_settings
    DROP CONSTRAINT IF EXISTS fk_css_adapter_type;

ALTER TABLE company_scan_settings
    ADD CONSTRAINT fk_css_adapter_type
    FOREIGN KEY (adapter_type) REFERENCES ats(name);

-- Re-point FKs: company_scan_queue → ats
ALTER TABLE company_scan_queue
    DROP CONSTRAINT IF EXISTS fk_scan_queue_adapter_type;

ALTER TABLE company_scan_queue
    ADD CONSTRAINT fk_scan_queue_adapter_type
    FOREIGN KEY (adapter_type) REFERENCES ats(name);

DROP TABLE adapter_configs;

-- +goose Down

CREATE TABLE adapter_configs (
    adapter_type            TEXT        PRIMARY KEY,
    scan_interval_minutes   INT         NOT NULL DEFAULT 1440 CHECK(scan_interval_minutes > 0),
    inter_company_delay_s   INT         NOT NULL DEFAULT 5    CHECK(inter_company_delay_s >= 0),
    last_scheduled_at       TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO adapter_configs (adapter_type, scan_interval_minutes, inter_company_delay_s)
SELECT name, scan_interval_min, inter_company_delay_s FROM ats;

-- Re-point FKs: company_scan_settings → adapter_configs
ALTER TABLE company_scan_settings
    DROP CONSTRAINT IF EXISTS fk_css_adapter_type;

ALTER TABLE company_scan_settings
    ADD CONSTRAINT fk_css_adapter_type
    FOREIGN KEY (adapter_type) REFERENCES adapter_configs(adapter_type);

-- Re-point FKs: company_scan_queue → adapter_configs
ALTER TABLE company_scan_queue
    DROP CONSTRAINT IF EXISTS fk_scan_queue_adapter_type;

ALTER TABLE company_scan_queue
    ADD CONSTRAINT fk_scan_queue_adapter_type
    FOREIGN KEY (adapter_type) REFERENCES adapter_configs(adapter_type);

DROP TABLE ats;
