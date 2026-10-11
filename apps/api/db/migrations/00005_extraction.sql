-- +goose Up
-- Phase 4 (spec RF-10): the percentage of an archive being extracted, shown by
-- GET /api/jobs and by the event stream. It is written at most once a second per job.
ALTER TABLE upload_jobs ADD COLUMN progress INTEGER NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100);

-- +goose Down
ALTER TABLE upload_jobs DROP COLUMN progress;
