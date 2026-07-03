-- +goose Up
-- Fix 1: crawler retry hot-loop. Adds per-row back-off to the scan queue and a
-- system auto-pause flag (distinct from admin `active`) to scan settings.
--
-- Run order: BEFORE deploy is safe — the columns are additive with fast
-- defaults (nullable TIMESTAMPTZ and BOOLEAN DEFAULT false are metadata-only in
-- PG11+), and old binaries ignore them. The auto-pause SWEEP that POPULATES the
-- flag ships in the new scheduler binary, so there is NO separate post-deploy
-- cleanup migration — the sweep auto-pauses existing chronic failers on its
-- first tick once the new binary is live.
--
-- No CONCURRENTLY: goose runs each migration inside a transaction block where
-- CONCURRENTLY is forbidden (matches migration 076).

ALTER TABLE company_scan_queue
    ADD COLUMN next_attempt_at TIMESTAMPTZ; -- NULL = due now

-- Claim/enqueue eligibility index: fast lookup of due 'queued' rows. The claim
-- WHERE filters next_attempt_at with COALESCE(next_attempt_at,'-infinity') <=
-- NOW(); this partial index narrows the scan to queued rows first.
CREATE INDEX idx_csq_due
    ON company_scan_queue (adapter_type, priority DESC, queued_at ASC)
    WHERE status = 'queued';

ALTER TABLE company_scan_settings
    ADD COLUMN auto_paused BOOLEAN NOT NULL DEFAULT false;

-- Supports the auto-pause sweep scan (companies over the error threshold that
-- are not yet paused) and the eligibility filter in EnqueueDue/EnqueueAll.
CREATE INDEX idx_css_auto_pause_candidates
    ON company_scan_settings (consecutive_errors)
    WHERE auto_paused = false;

-- +goose Down
DROP INDEX IF EXISTS idx_css_auto_pause_candidates;
ALTER TABLE company_scan_settings DROP COLUMN auto_paused;
DROP INDEX IF EXISTS idx_csq_due;
ALTER TABLE company_scan_queue DROP COLUMN next_attempt_at;
