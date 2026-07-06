-- +goose Up
-- Replaces LLM-derived resumes.keywords with a plain-text column populated by
-- the (non-LLM) PDF/DOCX text extractor at upload time. Matching against a
-- job's keywords now happens client-side against this raw text — see
-- docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md.
-- No backfill for pre-existing resumes (explicit product decision) — they
-- get extracted_text = '' until re-uploaded.
ALTER TABLE resumes DROP COLUMN IF EXISTS keywords;
ALTER TABLE resumes ADD COLUMN extracted_text TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE resumes DROP COLUMN IF EXISTS extracted_text;
ALTER TABLE resumes ADD COLUMN keywords JSONB NOT NULL DEFAULT '[]'::jsonb;
