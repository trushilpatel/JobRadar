-- +goose Up
CREATE TABLE job_application (
    id            BIGSERIAL PRIMARY KEY,
    job_result_id TEXT NOT NULL REFERENCES job(id),
    stage         TEXT NOT NULL
                      CHECK(stage IN (
                          'applied',
                          'phone_screen',
                          'technical',
                          'final_round',
                          'offer',
                          'rejected'
                      )),
    stage_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note          TEXT
);

CREATE INDEX idx_app_job   ON job_application(job_result_id);
CREATE INDEX idx_app_stage ON job_application(stage);
CREATE INDEX idx_app_time  ON job_application(stage_at DESC);

-- +goose Down
DROP INDEX idx_app_time;
DROP INDEX idx_app_stage;
DROP INDEX idx_app_job;
DROP TABLE job_application;
