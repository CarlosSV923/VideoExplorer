-- +goose Up
-- Phase 4 (spec RF-08, RF-08a): the entries of No asignados that the ingestion moves
-- into _unassigned/. An entry keeps the batch it came from, its relation and its origin
-- (upload), so that Asignar can pre-fill them (RF-27a). Its files are recorded before the
-- first rename: a move that a failure or a restart interrupted goes on from these rows,
-- and a file already moved is never moved twice.
CREATE TABLE unassigned_entries (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    -- What the move is for, chosen by the caller: the batch, or the unit of a batch without relation.
    source     TEXT    NOT NULL UNIQUE,
    -- The entry's name in _unassigned/: a folder, or a loose file with its extension (RF-08).
    name       TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    -- A single video kept as it is, without a folder (RF-08).
    loose      INTEGER NOT NULL CHECK (loose IN (0, 1)),
    batch_id   TEXT    NOT NULL,
    relation   TEXT    NOT NULL CHECK (relation IN ('parts', 'episodes', 'none')),
    origin     TEXT    NOT NULL CHECK (origin IN ('upload')),
    created_at TEXT    NOT NULL
);

CREATE TABLE unassigned_files (
    entry_id INTEGER NOT NULL REFERENCES unassigned_entries (id) ON DELETE CASCADE,
    -- Where the file was when the entry was recorded (its staging path).
    source   TEXT    NOT NULL,
    -- Its place inside the entry, with "/" separators; empty for a loose entry.
    rel      TEXT    NOT NULL COLLATE NOCASE,
    size     INTEGER NOT NULL CHECK (size >= 0),
    moved    INTEGER NOT NULL DEFAULT 0 CHECK (moved IN (0, 1)),
    PRIMARY KEY (entry_id, source),
    UNIQUE (entry_id, rel)
);

-- +goose Down
DROP TABLE unassigned_files;
DROP TABLE unassigned_entries;
