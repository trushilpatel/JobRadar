-- +goose Up
-- Add ON DELETE CASCADE to every FK that blocks company deletion.
-- Delete chain: companies → job_scan_log
--                         → job → job_application
--                                → job_discarded

ALTER TABLE job_scan_log
    DROP CONSTRAINT job_scan_log_company_id_fkey,
    ADD  CONSTRAINT job_scan_log_company_id_fkey
         FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE;

ALTER TABLE job
    DROP CONSTRAINT job_company_id_fkey,
    ADD  CONSTRAINT job_company_id_fkey
         FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE;

ALTER TABLE job_application
    DROP CONSTRAINT job_application_job_result_id_fkey,
    ADD  CONSTRAINT job_application_job_result_id_fkey
         FOREIGN KEY (job_result_id) REFERENCES job(id) ON DELETE CASCADE;

ALTER TABLE job_discarded
    DROP CONSTRAINT job_discarded_job_result_id_fkey,
    ADD  CONSTRAINT job_discarded_job_result_id_fkey
         FOREIGN KEY (job_result_id) REFERENCES job(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE job_discarded
    DROP CONSTRAINT job_discarded_job_result_id_fkey,
    ADD  CONSTRAINT job_discarded_job_result_id_fkey
         FOREIGN KEY (job_result_id) REFERENCES job(id);

ALTER TABLE job_application
    DROP CONSTRAINT job_application_job_result_id_fkey,
    ADD  CONSTRAINT job_application_job_result_id_fkey
         FOREIGN KEY (job_result_id) REFERENCES job(id);

ALTER TABLE job
    DROP CONSTRAINT job_company_id_fkey,
    ADD  CONSTRAINT job_company_id_fkey
         FOREIGN KEY (company_id) REFERENCES companies(id);

ALTER TABLE job_scan_log
    DROP CONSTRAINT job_scan_log_company_id_fkey,
    ADD  CONSTRAINT job_scan_log_company_id_fkey
         FOREIGN KEY (company_id) REFERENCES companies(id);
