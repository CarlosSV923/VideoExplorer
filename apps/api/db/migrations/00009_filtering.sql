-- +goose Up
-- Phase 4 (spec RF-07, RF-10a): the filter of a job's content survives restarts.
-- filter_done marks a job whose content the filter went through: what it keeps waits
-- in filter_files, and the job stays filtering until the step that moves it to No asignados.
ALTER TABLE upload_jobs ADD COLUMN filter_done INTEGER NOT NULL DEFAULT 0 CHECK (filter_done IN (0, 1));

-- What the filter found in a work unit, keyed by the job that leads it: a single
-- file, or the first volume of a set. A discarded file is trashed once (trashed = 1),
-- so resuming after a restart never sends it to the trash twice.
CREATE TABLE filter_files (
    job_id  TEXT    NOT NULL REFERENCES upload_jobs (id) ON DELETE CASCADE,
    -- The file's place inside the unit, with "/" separators.
    path    TEXT    NOT NULL,
    size    INTEGER NOT NULL CHECK (size >= 0),
    kind    TEXT    NOT NULL CHECK (kind IN ('video', 'subtitle', 'discard')),
    trashed INTEGER NOT NULL DEFAULT 0 CHECK (trashed IN (0, 1)),
    PRIMARY KEY (job_id, path)
);

-- +goose Down
DROP TABLE filter_files;
ALTER TABLE upload_jobs DROP COLUMN filter_done;
