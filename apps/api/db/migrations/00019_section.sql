-- +goose Up
-- Phase 6 (spec RF-27, RF-30): the section reads _unassigned/ as it is now. Its entries can come from
-- Samba (origin samba), and each file keeps the size and modification time of its last reading
-- (mtime_ms, NULL until a reading): a file still being copied changes between two readings, and the
-- readings must survive a restart. The trash keeps what went there from the section (origin samba,
-- reason removed).
--
-- SQLite cannot change a CHECK constraint, so the tables are rebuilt and their rows and ids copied.
-- The files of the section are copied aside and their table is dropped before the entries' one
-- (foreign keys are on, and dropping a parent with children would delete them); the trash links of
-- the library are cleared while its tables are rebuilt, as in migration 00017.
CREATE TEMP TABLE unassigned_files_keep AS
    SELECT entry_id, source, rel, size FROM unassigned_files;
DROP TABLE unassigned_files;

CREATE TABLE unassigned_entries_new (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    source     TEXT    NOT NULL UNIQUE,
    name       TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    loose      INTEGER NOT NULL CHECK (loose IN (0, 1)),
    batch_id   TEXT    NOT NULL,
    relation   TEXT    NOT NULL CHECK (relation IN ('parts', 'episodes', 'none')),
    -- Where the entry came from: an upload (RF-08), or a copy by Samba (RF-27).
    origin     TEXT    NOT NULL CHECK (origin IN ('upload', 'samba')),
    created_at TEXT    NOT NULL,
    lock_token TEXT    NOT NULL DEFAULT '',
    lock_until INTEGER NOT NULL DEFAULT 0 CHECK (lock_until >= 0)
);

INSERT INTO unassigned_entries_new (id, source, name, loose, batch_id, relation, origin, created_at, lock_token, lock_until)
    SELECT id, source, name, loose, batch_id, relation, origin, created_at, lock_token, lock_until FROM unassigned_entries;

-- The ids keep counting from where the old table stopped, so a deleted entry's id is not reused.
UPDATE sqlite_sequence SET seq = MAX(seq, COALESCE(
        (SELECT seq FROM sqlite_sequence WHERE name = 'unassigned_entries'), 0))
    WHERE name = 'unassigned_entries_new';

DROP TABLE unassigned_entries;
ALTER TABLE unassigned_entries_new RENAME TO unassigned_entries;

CREATE TABLE unassigned_files (
    entry_id INTEGER NOT NULL REFERENCES unassigned_entries (id) ON DELETE CASCADE,
    -- Where the file was when the entry was recorded: its staging path, or its place in _unassigned/
    -- for a file copied by Samba. The key of the file inside its entry.
    source   TEXT    NOT NULL,
    -- Its place inside the entry, with "/" separators; empty for a loose entry.
    rel      TEXT    NOT NULL COLLATE NOCASE,
    size     INTEGER NOT NULL CHECK (size >= 0),
    -- The modification time of the last reading, in Unix milliseconds; NULL when never read.
    mtime_ms INTEGER CHECK (mtime_ms IS NULL OR mtime_ms >= 0),
    PRIMARY KEY (entry_id, source),
    UNIQUE (entry_id, rel)
);

INSERT INTO unassigned_files (entry_id, source, rel, size)
    SELECT entry_id, source, rel, size FROM unassigned_files_keep;
DROP TABLE unassigned_files_keep;

-- The trash: origin samba (an entry copied by Samba) and reason removed (sent to the trash from the section).
CREATE TEMP TABLE library_trash_links AS
    SELECT id, trash_entry_id FROM library_files WHERE trash_entry_id IS NOT NULL;
UPDATE library_files SET trash_entry_id = NULL WHERE trash_entry_id IS NOT NULL;

CREATE TABLE trash_entries_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    origin        TEXT    NOT NULL CHECK (origin IN ('upload', 'library', 'samba')),
    reason        TEXT    NOT NULL CHECK (reason IN ('extracted', 'filtered', 'replaced', 'removed')),
    dir           TEXT    NOT NULL UNIQUE,
    trashed_at    TEXT    NOT NULL,
    batch_id      TEXT    NOT NULL DEFAULT '',
    relative_path TEXT    NOT NULL DEFAULT ''
);

INSERT INTO trash_entries_new (id, origin, reason, dir, trashed_at, batch_id, relative_path)
    SELECT id, origin, reason, dir, trashed_at, batch_id, relative_path FROM trash_entries;

CREATE TABLE trash_files_new (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    entry_id INTEGER NOT NULL REFERENCES trash_entries_new (id) ON DELETE CASCADE,
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
-- The entries of Samba and the files sent to the trash from the section leave first: the trash
-- keeps its other entries, and the library links are cleared while its table is rebuilt.
DELETE FROM unassigned_files WHERE entry_id IN (SELECT id FROM unassigned_entries WHERE origin = 'samba');
DELETE FROM unassigned_entries WHERE origin = 'samba';

CREATE TEMP TABLE library_trash_links AS
    SELECT id, trash_entry_id FROM library_files WHERE trash_entry_id IS NOT NULL;
UPDATE library_files SET trash_entry_id = NULL WHERE trash_entry_id IS NOT NULL;
DELETE FROM trash_entries WHERE origin = 'samba' OR reason = 'removed';

CREATE TABLE trash_entries_old (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    origin        TEXT    NOT NULL CHECK (origin IN ('upload', 'library')),
    reason        TEXT    NOT NULL CHECK (reason IN ('extracted', 'filtered', 'replaced')),
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

CREATE TEMP TABLE unassigned_files_keep AS
    SELECT entry_id, source, rel, size FROM unassigned_files;
DROP TABLE unassigned_files;

CREATE TABLE unassigned_entries_old (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    source     TEXT    NOT NULL UNIQUE,
    name       TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    loose      INTEGER NOT NULL CHECK (loose IN (0, 1)),
    batch_id   TEXT    NOT NULL,
    relation   TEXT    NOT NULL CHECK (relation IN ('parts', 'episodes', 'none')),
    origin     TEXT    NOT NULL CHECK (origin IN ('upload')),
    created_at TEXT    NOT NULL,
    lock_token TEXT    NOT NULL DEFAULT '',
    lock_until INTEGER NOT NULL DEFAULT 0 CHECK (lock_until >= 0)
);

INSERT INTO unassigned_entries_old (id, source, name, loose, batch_id, relation, origin, created_at, lock_token, lock_until)
    SELECT id, source, name, loose, batch_id, relation, origin, created_at, lock_token, lock_until FROM unassigned_entries;

UPDATE sqlite_sequence SET seq = MAX(seq, COALESCE(
        (SELECT seq FROM sqlite_sequence WHERE name = 'unassigned_entries'), 0))
    WHERE name = 'unassigned_entries_old';

DROP TABLE unassigned_entries;
ALTER TABLE unassigned_entries_old RENAME TO unassigned_entries;

CREATE TABLE unassigned_files (
    entry_id INTEGER NOT NULL REFERENCES unassigned_entries (id) ON DELETE CASCADE,
    source   TEXT    NOT NULL,
    rel      TEXT    NOT NULL COLLATE NOCASE,
    size     INTEGER NOT NULL CHECK (size >= 0),
    PRIMARY KEY (entry_id, source),
    UNIQUE (entry_id, rel)
);

INSERT INTO unassigned_files (entry_id, source, rel, size)
    SELECT entry_id, source, rel, size FROM unassigned_files_keep;
DROP TABLE unassigned_files_keep;
