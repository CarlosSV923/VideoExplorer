-- +goose Up
-- Phase 5 (spec RF-24, RF-30): replacing a file of the library sends it, with its subtitles, to the
-- trash. Its entry has the origin library (a file of the library, with no batch) and the reason
-- replaced. SQLite cannot change a CHECK constraint, so both trash tables are rebuilt and their rows
-- and ids copied. library_files refers to trash_entries: its links are saved, cleared while the
-- tables are rebuilt (a dropped parent with children still pointing at it fails the foreign key
-- check), and restored at the end, when the rebuilt table has the same ids.
CREATE TEMP TABLE library_trash_links AS
    SELECT id, trash_entry_id FROM library_files WHERE trash_entry_id IS NOT NULL;
UPDATE library_files SET trash_entry_id = NULL WHERE trash_entry_id IS NOT NULL;

CREATE TABLE trash_entries_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    -- Where the files came from: an upload, or the library (a replaced file).
    origin        TEXT    NOT NULL CHECK (origin IN ('upload', 'library')),
    -- Why they went to the trash: the archive was extracted (RF-06), the filter discarded them
    -- (RF-07), or a new file replaced them (RF-24).
    reason        TEXT    NOT NULL CHECK (reason IN ('extracted', 'filtered', 'replaced')),
    -- The folder of the entry inside the trash, a random name.
    dir           TEXT    NOT NULL UNIQUE,
    trashed_at    TEXT    NOT NULL,
    -- The batch the file was dropped with (RF-03), and its path in that drop (RF-01a); empty for a library file.
    batch_id      TEXT    NOT NULL DEFAULT '',
    relative_path TEXT    NOT NULL DEFAULT ''
);

INSERT INTO trash_entries_new (id, origin, reason, dir, trashed_at, batch_id, relative_path)
    SELECT id, origin, reason, dir, trashed_at, batch_id, relative_path FROM trash_entries;

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

UPDATE library_files SET trash_entry_id = (
        SELECT l.trash_entry_id FROM library_trash_links l WHERE l.id = library_files.id)
    WHERE id IN (SELECT id FROM library_trash_links);
DROP TABLE library_trash_links;

-- +goose Down
-- The replaced files leave the registry's trash links first; they stay on disk in the trash.
CREATE TEMP TABLE library_trash_links AS
    SELECT id, trash_entry_id FROM library_files WHERE trash_entry_id IS NOT NULL;
UPDATE library_files SET trash_entry_id = NULL WHERE trash_entry_id IS NOT NULL;
DELETE FROM trash_entries WHERE origin = 'library' OR reason = 'replaced';

CREATE TABLE trash_entries_old (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    origin        TEXT    NOT NULL CHECK (origin IN ('upload')),
    reason        TEXT    NOT NULL CHECK (reason IN ('extracted', 'filtered')),
    dir           TEXT    NOT NULL UNIQUE,
    trashed_at    TEXT    NOT NULL,
    batch_id      TEXT    NOT NULL DEFAULT '',
    relative_path TEXT    NOT NULL DEFAULT ''
);

INSERT INTO trash_entries_old (id, origin, reason, dir, trashed_at, batch_id, relative_path)
    SELECT id, origin, reason, dir, trashed_at, batch_id, relative_path FROM trash_entries;

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

UPDATE library_files SET trash_entry_id = (
        SELECT l.trash_entry_id FROM library_trash_links l WHERE l.id = library_files.id)
    WHERE id IN (SELECT id FROM library_trash_links WHERE trash_entry_id IN (SELECT id FROM trash_entries));
DROP TABLE library_trash_links;
