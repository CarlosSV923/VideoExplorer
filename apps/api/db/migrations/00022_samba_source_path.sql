-- +goose Up
-- Phase 6 (spec RF-27a): a Samba copy keeps its place in _unassigned/ (folders included). That place is the
-- origin of the copy in the trash, so that restoring it puts the copy back in its folder. Empty for an upload.
ALTER TABLE upload_jobs ADD COLUMN source_path TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE upload_jobs DROP COLUMN source_path;
