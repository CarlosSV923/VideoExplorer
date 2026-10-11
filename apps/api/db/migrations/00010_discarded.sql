-- +goose Up
-- Phase 4 (spec RF-07, RF-10a): the state discarded («Descartado»), for a job the filter
-- emptied while its batch keeps something. SQLite cannot change a CHECK constraint, so
-- upload_jobs is rebuilt with the same columns, in the same order, and its rows are
-- copied. filter_files refers to upload_jobs, so it is copied aside and written back.
CREATE TABLE upload_jobs_new (
    id            TEXT    NOT NULL PRIMARY KEY,
    -- Every upload belongs to the batch it was dropped with (RF-09a).
    batch_id      TEXT    NOT NULL REFERENCES upload_batches (id),
    file_name     TEXT    NOT NULL,
    -- Path inside the folder dropped in the browser (RF-01a); empty for a file on its own.
    relative_path TEXT    NOT NULL DEFAULT '',
    size          INTEGER NOT NULL CHECK (size > 0),
    status        TEXT    NOT NULL CHECK (status IN (
                      'uploading', 'uploaded', 'waiting_parts', 'extracting', 'needs_password',
                      'filtering', 'done', 'discarded', 'failed', 'cancelled')),
    error         TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL,
    updated_at    TEXT    NOT NULL,
    received      INTEGER NOT NULL DEFAULT 0 CHECK (received >= 0),
    progress      INTEGER NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    parts_total   INTEGER NOT NULL DEFAULT 0 CHECK (parts_total BETWEEN 0 AND 999),
    volume_set    TEXT    NOT NULL DEFAULT '',
    volume_index  INTEGER NOT NULL DEFAULT 0 CHECK (volume_index >= 0),
    volume_group  TEXT    NOT NULL DEFAULT '',
    filter_done   INTEGER NOT NULL DEFAULT 0 CHECK (filter_done IN (0, 1))
);

INSERT INTO upload_jobs_new (id, batch_id, file_name, relative_path, size, status, error, created_at, updated_at,
                             received, progress, parts_total, volume_set, volume_index, volume_group, filter_done)
    SELECT id, batch_id, file_name, relative_path, size, status, error, created_at, updated_at,
           received, progress, parts_total, volume_set, volume_index, volume_group, filter_done
    FROM upload_jobs;

CREATE TABLE filter_files_kept AS SELECT job_id, path, size, kind, trashed FROM filter_files;
DROP TABLE filter_files;
DROP TABLE upload_jobs;

ALTER TABLE upload_jobs_new RENAME TO upload_jobs;
CREATE INDEX upload_jobs_batch ON upload_jobs (batch_id);
CREATE INDEX upload_jobs_volume ON upload_jobs (batch_id, volume_set);
CREATE INDEX upload_jobs_volume_group ON upload_jobs (volume_group);

CREATE TABLE filter_files (
    job_id  TEXT    NOT NULL REFERENCES upload_jobs (id) ON DELETE CASCADE,
    -- The file's place inside the unit, with "/" separators.
    path    TEXT    NOT NULL,
    size    INTEGER NOT NULL CHECK (size >= 0),
    kind    TEXT    NOT NULL CHECK (kind IN ('video', 'subtitle', 'discard')),
    trashed INTEGER NOT NULL DEFAULT 0 CHECK (trashed IN (0, 1)),
    PRIMARY KEY (job_id, path)
);
INSERT INTO filter_files (job_id, path, size, kind, trashed)
    SELECT job_id, path, size, kind, trashed FROM filter_files_kept;
DROP TABLE filter_files_kept;

-- +goose Down
-- A discarded job goes back to the nearest state the schema knows: failed, with the
-- reason no_video that the filter gives such a job.
CREATE TABLE upload_jobs_old (
    id            TEXT    NOT NULL PRIMARY KEY,
    batch_id      TEXT    NOT NULL REFERENCES upload_batches (id),
    file_name     TEXT    NOT NULL,
    relative_path TEXT    NOT NULL DEFAULT '',
    size          INTEGER NOT NULL CHECK (size > 0),
    status        TEXT    NOT NULL CHECK (status IN (
                      'uploading', 'uploaded', 'waiting_parts', 'extracting', 'needs_password',
                      'filtering', 'done', 'failed', 'cancelled')),
    error         TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL,
    updated_at    TEXT    NOT NULL,
    received      INTEGER NOT NULL DEFAULT 0 CHECK (received >= 0),
    progress      INTEGER NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    parts_total   INTEGER NOT NULL DEFAULT 0 CHECK (parts_total BETWEEN 0 AND 999),
    volume_set    TEXT    NOT NULL DEFAULT '',
    volume_index  INTEGER NOT NULL DEFAULT 0 CHECK (volume_index >= 0),
    volume_group  TEXT    NOT NULL DEFAULT '',
    filter_done   INTEGER NOT NULL DEFAULT 0 CHECK (filter_done IN (0, 1))
);

INSERT INTO upload_jobs_old (id, batch_id, file_name, relative_path, size, status, error, created_at, updated_at,
                             received, progress, parts_total, volume_set, volume_index, volume_group, filter_done)
    SELECT id, batch_id, file_name, relative_path, size,
           CASE WHEN status = 'discarded' THEN 'failed' ELSE status END,
           CASE WHEN status = 'discarded' THEN 'no_video' ELSE error END,
           created_at, updated_at,
           received, progress, parts_total, volume_set, volume_index, volume_group, filter_done
    FROM upload_jobs;

CREATE TABLE filter_files_kept AS SELECT job_id, path, size, kind, trashed FROM filter_files;
DROP TABLE filter_files;
DROP TABLE upload_jobs;

ALTER TABLE upload_jobs_old RENAME TO upload_jobs;
CREATE INDEX upload_jobs_batch ON upload_jobs (batch_id);
CREATE INDEX upload_jobs_volume ON upload_jobs (batch_id, volume_set);
CREATE INDEX upload_jobs_volume_group ON upload_jobs (volume_group);

CREATE TABLE filter_files (
    job_id  TEXT    NOT NULL REFERENCES upload_jobs (id) ON DELETE CASCADE,
    path    TEXT    NOT NULL,
    size    INTEGER NOT NULL CHECK (size >= 0),
    kind    TEXT    NOT NULL CHECK (kind IN ('video', 'subtitle', 'discard')),
    trashed INTEGER NOT NULL DEFAULT 0 CHECK (trashed IN (0, 1)),
    PRIMARY KEY (job_id, path)
);
INSERT INTO filter_files (job_id, path, size, kind, trashed)
    SELECT job_id, path, size, kind, trashed FROM filter_files_kept;
DROP TABLE filter_files_kept;
