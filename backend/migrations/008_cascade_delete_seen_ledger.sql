-- +goose Up
ALTER TABLE job_seen_ledger
    DROP CONSTRAINT job_seen_ledger_company_id_fkey,
    ADD  CONSTRAINT job_seen_ledger_company_id_fkey
         FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE job_seen_ledger
    DROP CONSTRAINT job_seen_ledger_company_id_fkey,
    ADD  CONSTRAINT job_seen_ledger_company_id_fkey
         FOREIGN KEY (company_id) REFERENCES companies(id);
