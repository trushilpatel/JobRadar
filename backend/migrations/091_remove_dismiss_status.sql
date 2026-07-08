-- +goose Up
-- Removes the ability to write new 'dismissed' rows to user_job_state — the
-- dismiss-job feature is being removed from the UI and API entirely.
--
-- Existing 'dismissed' rows are INTENTIONALLY left untouched, and
-- user_job_counters.dismissed_count keeps being maintained internally (it
-- feeds the "New" tab badge math: New = total - sum(workflow buckets), and
-- dismissed is one of those buckets). Migration 084 backfilled
-- status='dismissed' for every (user x non-USA on-site job) pair to keep
-- those jobs out of the "New" feed (the feed's exclusion logic is simply
-- "status IS NULL" in application code — see JobImplService.go). If this
-- migration deleted/reassigned those rows or stopped counting them, every
-- previously-hidden non-USA job — and every user's own past dismissals —
-- would resurface in the New feed or inflate its badge count.
--
-- The CHECK constraint below is added NOT VALID specifically so existing
-- rows are never re-validated (and thus never rejected), while new
-- INSERT/UPDATE statements can no longer write 'dismissed'.

-- +goose StatementBegin

ALTER TABLE user_job_state
    DROP CONSTRAINT IF EXISTS user_job_state_status_check;

ALTER TABLE user_job_state
    ADD CONSTRAINT user_job_state_status_check
    CHECK (status IS NULL
        OR status IN ('applied', 'interviewing', 'offer', 'rejected'))
    NOT VALID;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE user_job_state
    DROP CONSTRAINT IF EXISTS user_job_state_status_check;

ALTER TABLE user_job_state
    ADD CONSTRAINT user_job_state_status_check
    CHECK (status IS NULL
        OR status IN ('applied', 'dismissed', 'interviewing', 'offer', 'rejected'));

-- +goose StatementEnd
