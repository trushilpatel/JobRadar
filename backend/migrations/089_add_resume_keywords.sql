-- +goose Up
-- Canonical (techtag.Clean-normalized) technology/skill keywords extracted
-- from the resume's plain text body — see service/llm/services.Scorer's
-- ExtractResumeKeywords and service/resume/textextract. Defaults to an empty
-- array: extraction is best-effort and non-fatal, so a resume uploaded before
-- this feature existed (or one whose extraction failed) simply has no
-- keywords until POST /resumes/{id}/keywords is retried.
ALTER TABLE resumes ADD COLUMN keywords JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE resumes DROP COLUMN IF EXISTS keywords;
