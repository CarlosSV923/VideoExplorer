-- +goose Up
-- Phase 6 (spec RF-27a): a batch, and the jobs in it, can come from a compressed file that Samba
-- copied into No asignados instead of a browser upload. Such a batch has origin samba. Its jobs
-- have no tus upload id: the server makes them, and their staged file is the job's own. The job
-- keeps the origin of its batch so that the purge and the reconciliation can tell them apart.
ALTER TABLE upload_batches ADD COLUMN origin TEXT NOT NULL DEFAULT 'upload' CHECK (origin IN ('upload', 'samba'));
ALTER TABLE upload_jobs ADD COLUMN origin TEXT NOT NULL DEFAULT 'upload' CHECK (origin IN ('upload', 'samba'));
CREATE INDEX upload_jobs_origin_status ON upload_jobs (origin, status);

-- +goose Down
DROP INDEX upload_jobs_origin_status;
ALTER TABLE upload_jobs DROP COLUMN origin;
ALTER TABLE upload_batches DROP COLUMN origin;
