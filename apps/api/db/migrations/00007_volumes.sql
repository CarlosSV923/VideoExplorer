-- +goose Up
-- Phase 4 (spec RF-03, RF-10a): the volumes of a multi-volume archive. partsTotal
-- is how many volumes the browser counted for the set; the set is the volume's
-- name without its number and the batch it was dropped with names it. Once the
-- set is complete it is claimed under volume_group, the id of its first volume.
-- Zero and empty for every other file.
ALTER TABLE upload_jobs ADD COLUMN parts_total INTEGER NOT NULL DEFAULT 0 CHECK (parts_total BETWEEN 0 AND 999);
ALTER TABLE upload_jobs ADD COLUMN volume_set TEXT NOT NULL DEFAULT '';
ALTER TABLE upload_jobs ADD COLUMN volume_index INTEGER NOT NULL DEFAULT 0 CHECK (volume_index >= 0);
ALTER TABLE upload_jobs ADD COLUMN volume_group TEXT NOT NULL DEFAULT '';
CREATE INDEX upload_jobs_volume ON upload_jobs (batch_id, volume_set);
CREATE INDEX upload_jobs_volume_group ON upload_jobs (volume_group);

-- +goose Down
DROP INDEX upload_jobs_volume_group;
DROP INDEX upload_jobs_volume;
ALTER TABLE upload_jobs DROP COLUMN volume_group;
ALTER TABLE upload_jobs DROP COLUMN volume_index;
ALTER TABLE upload_jobs DROP COLUMN volume_set;
ALTER TABLE upload_jobs DROP COLUMN parts_total;
