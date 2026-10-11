-- +goose Up
-- Phase 4 (spec RF-07, RF-30): the trash keeps where each file came from, the batch
-- it was dropped with and its relative path in that drop, and it has a reason for the
-- files the filter discards. SQLite cannot change a CHECK constraint, so both trash
-- tables are rebuilt and their rows (and ids) copied. Entries recorded before this
-- migration have no batch and no relative path.
CREATE TABLE trash_entries_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    -- Where the files came from: an upload.
    origin        TEXT    NOT NULL CHECK (origin IN ('upload')),
    -- Why they went to the trash: the archive was extracted (RF-06), or the filter discarded them (RF-07).
    reason        TEXT    NOT NULL CHECK (reason IN ('extracted', 'filtered')),
    -- The folder of the entry inside the trash, a random name.
    dir           TEXT    NOT NULL UNIQUE,
    trashed_at    TEXT    NOT NULL,
    -- The batch the file was dropped with (RF-03), and its path in that drop (RF-01a).
    batch_id      TEXT    NOT NULL DEFAULT '',
    relative_path TEXT    NOT NULL DEFAULT ''
);

INSERT INTO trash_entries_new (id, origin, reason, dir, trashed_at)
    SELECT id, origin, reason, dir, trashed_at FROM trash_entries;

CREATE TABLE trash_files_new (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    entry_id INTEGER NOT NULL REFERENCES trash_entries_new (id) ON DELETE CASCADE,
    -- The file's name inside the entry's folder, as it was called before.
    name     TEXT    NOT NULL,
    size     INTEGER NOT NULL CHECK (size >= 0)
);

INSERT INTO trash_files_new (id, entry_id, name, size)
    SELECT id, entry_id, name, size FROM trash_files;

DROP INDEX trash_files_entry;
DROP TABLE trash_files;
DROP TABLE trash_entries;

ALTER TABLE trash_entries_new RENAME TO trash_entries;
ALTER TABLE trash_files_new RENAME TO trash_files;
CREATE INDEX trash_files_entry ON trash_files (entry_id);

-- +goose Down
-- Entries the filter discarded have no place in the old schema: they are removed with their files.
DELETE FROM trash_entries WHERE reason = 'filtered';

CREATE TABLE trash_entries_old (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    origin     TEXT    NOT NULL CHECK (origin IN ('upload')),
    reason     TEXT    NOT NULL CHECK (reason IN ('extracted')),
    dir        TEXT    NOT NULL UNIQUE,
    trashed_at TEXT    NOT NULL
);

INSERT INTO trash_entries_old (id, origin, reason, dir, trashed_at)
    SELECT id, origin, reason, dir, trashed_at FROM trash_entries;

CREATE TABLE trash_files_old (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    entry_id INTEGER NOT NULL REFERENCES trash_entries_old (id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    size     INTEGER NOT NULL CHECK (size >= 0)
);

INSERT INTO trash_files_old (id, entry_id, name, size)
    SELECT id, entry_id, name, size FROM trash_files;

DROP INDEX trash_files_entry;
DROP TABLE trash_files;
DROP TABLE trash_entries;

ALTER TABLE trash_entries_old RENAME TO trash_entries;
ALTER TABLE trash_files_old RENAME TO trash_files;
CREATE INDEX trash_files_entry ON trash_files (entry_id);
