-- +goose Up
-- Phase 3 (spec RF-10b): bytes received so far, shown by GET /api/jobs and by the
-- event stream. The tus endpoint reports progress about once a second per upload,
-- so this column is written at most that often, never once per chunk.
ALTER TABLE upload_jobs ADD COLUMN received INTEGER NOT NULL DEFAULT 0 CHECK (received >= 0);

-- +goose Down
ALTER TABLE upload_jobs DROP COLUMN received;
