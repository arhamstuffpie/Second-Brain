-- +goose Up
ALTER TABLE analysis_stage_jobs
    ADD COLUMN depends_on TEXT[] NOT NULL DEFAULT '{}';

ALTER TABLE analysis_stage_jobs
    ADD CONSTRAINT analysis_stage_jobs_no_self_dependency
    CHECK (NOT stage = ANY(depends_on));

-- +goose Down
ALTER TABLE analysis_stage_jobs
    DROP CONSTRAINT analysis_stage_jobs_no_self_dependency,
    DROP COLUMN depends_on;
