-- +goose Up
-- Phase 6 (spec RF-27a): whether a copy that Samba made into _unassigned/ is a compressed archive, read
-- from its first bytes. The reading is kept by path with the size and modification time it had, so that a
-- settled copy is not opened again at every reading of the section, and a file that changes is read again.
CREATE TABLE archive_probes (
    path     TEXT    PRIMARY KEY,
    size     INTEGER NOT NULL CHECK (size >= 0),
    mtime_ms INTEGER NOT NULL,
    archive  INTEGER NOT NULL CHECK (archive IN (0, 1))
);

-- +goose Down
DROP TABLE archive_probes;
