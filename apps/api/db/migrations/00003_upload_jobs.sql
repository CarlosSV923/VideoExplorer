-- +goose Up
-- Phase 3: the files dropped together form a batch (spec RF-03) and every uploaded
-- file is a job whose state survives restarts (RF-10a). The job id is the tus
-- upload id (RF-02), so the browser's upload and the server's job are one thing.
CREATE TABLE upload_batches (
    id         TEXT NOT NULL PRIMARY KEY,
    -- Provisional name (spec RF-08 sets the final one); empty when none was given.
    name       TEXT NOT NULL DEFAULT '',
    relation   TEXT NOT NULL CHECK (relation IN ('parts', 'episodes', 'none')),
    created_at TEXT NOT NULL
);

CREATE TABLE upload_jobs (
    id            TEXT    NOT NULL PRIMARY KEY,
    -- Every upload belongs to the batch it was dropped with (RF-09a).
    batch_id      TEXT    NOT NULL REFERENCES upload_batches (id),
    file_name     TEXT    NOT NULL,
    -- Path inside the folder dropped in the browser (RF-01a); empty for a file on its own.
    relative_path TEXT    NOT NULL DEFAULT '',
    size          INTEGER NOT NULL CHECK (size > 0),
    status        TEXT    NOT NULL CHECK (status IN (
                      'uploading', 'uploaded', 'waiting_parts', 'extracting', 'needs_password',
                      'filtering', 'done', 'failed', 'cancelled')),
    error         TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL,
    updated_at    TEXT    NOT NULL
);
CREATE INDEX upload_jobs_batch ON upload_jobs (batch_id);

-- +goose Down
DROP INDEX upload_jobs_batch;
DROP TABLE upload_jobs;
DROP TABLE upload_batches;
