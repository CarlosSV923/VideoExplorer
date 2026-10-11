-- +goose Up
-- Phase 4 (spec RF-06, RF-30): the minimal trash of the catalog context. A trash
-- entry groups what went to the trash together; its files live in
-- .videoexplorer/trash/<dir>/, and these rows are their metadata. Restoring and
-- purging them come with the trash of phase 7.
CREATE TABLE trash_entries (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    -- Where the files came from: an upload (the original of an extracted archive).
    origin     TEXT    NOT NULL CHECK (origin IN ('upload')),
    -- Why they went to the trash: the archive was extracted (RF-06).
    reason     TEXT    NOT NULL CHECK (reason IN ('extracted')),
    -- The folder of the entry inside the trash, a random name.
    dir        TEXT    NOT NULL UNIQUE,
    trashed_at TEXT    NOT NULL
);

CREATE TABLE trash_files (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    entry_id INTEGER NOT NULL REFERENCES trash_entries (id) ON DELETE CASCADE,
    -- The file's name inside the entry's folder, as it was called before.
    name     TEXT    NOT NULL,
    size     INTEGER NOT NULL CHECK (size >= 0)
);
CREATE INDEX trash_files_entry ON trash_files (entry_id);

-- +goose Down
DROP INDEX trash_files_entry;
DROP TABLE trash_files;
DROP TABLE trash_entries;
