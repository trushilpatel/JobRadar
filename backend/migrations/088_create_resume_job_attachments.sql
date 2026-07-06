-- +goose Up
-- One resume may be attached per (user, job) at a time — attaching a
-- different resume overwrites the row via upsert (ON CONFLICT (user_id,
-- job_id) DO UPDATE), it does not create a second row. Deleting the user,
-- the job, or the resume cascades the attachment away.
CREATE TABLE resume_job_attachments (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_id     TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    resume_id  UUID NOT NULL REFERENCES resumes(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, job_id)
);

-- Backs the ON DELETE CASCADE lookup from resumes and any future
-- "which jobs use this resume" query.
CREATE INDEX idx_resume_job_attachments_resume_id ON resume_job_attachments(resume_id);

-- +goose Down
DROP TABLE IF EXISTS resume_job_attachments;
