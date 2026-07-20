-- +goose Up
-- job_tech_tags.company_id was created (025_job_tech_tags.sql) without
-- ON DELETE CASCADE, unlike every other FK to companies(id) (jobs,
-- company_scan_settings, company_scan_log, company_scan_queue all cascade —
-- see 007_cascade_delete_company.sql). Any company with tagged jobs (e.g.
-- Booz Allen Hamilton, 1078 rows) fails admin deletion with a raw
-- foreign-key violation the repository layer surfaces as a generic
-- "failed to delete company" 500.

ALTER TABLE job_tech_tags
    DROP CONSTRAINT job_tech_tags_company_id_fkey,
    ADD  CONSTRAINT job_tech_tags_company_id_fkey
         FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE job_tech_tags
    DROP CONSTRAINT job_tech_tags_company_id_fkey,
    ADD  CONSTRAINT job_tech_tags_company_id_fkey
         FOREIGN KEY (company_id) REFERENCES companies(id);
