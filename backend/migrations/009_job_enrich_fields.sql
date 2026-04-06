-- +goose Up
ALTER TABLE job
    DROP COLUMN  years_exp_min,
    DROP COLUMN  years_exp_max,
    DROP COLUMN  other_criteria,
    ADD COLUMN   experience_years   INTEGER,
    ADD COLUMN   sponsorship        BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN   requires_citizen   BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN   security_clearance BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN   role_brief         TEXT,
    ADD COLUMN   responsibilities   TEXT;

-- +goose Down
ALTER TABLE job
    DROP COLUMN  experience_years,
    DROP COLUMN  sponsorship,
    DROP COLUMN  requires_citizen,
    DROP COLUMN  security_clearance,
    DROP COLUMN  role_brief,
    DROP COLUMN  responsibilities,
    ADD COLUMN   years_exp_min  INTEGER,
    ADD COLUMN   years_exp_max  INTEGER,
    ADD COLUMN   other_criteria JSONB;
