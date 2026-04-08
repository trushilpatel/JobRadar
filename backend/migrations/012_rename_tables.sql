-- +goose Up
ALTER TABLE job_seen_ledger     RENAME TO job_pipeline_ledger;
ALTER TABLE job_scan_log        RENAME TO company_scan_log;
ALTER TABLE job                 RENAME TO job_feed;
ALTER TABLE job_snapshot        RENAME TO job_analytics;
ALTER TABLE job_application     RENAME TO job_application_tracker;
ALTER TABLE job_discarded       RENAME TO job_dismissal_log;

-- +goose Down
ALTER TABLE job_pipeline_ledger     RENAME TO job_seen_ledger;
ALTER TABLE company_scan_log        RENAME TO job_scan_log;
ALTER TABLE job_feed                RENAME TO job;
ALTER TABLE job_analytics           RENAME TO job_snapshot;
ALTER TABLE job_application_tracker RENAME TO job_application;
ALTER TABLE job_dismissal_log       RENAME TO job_discarded;
