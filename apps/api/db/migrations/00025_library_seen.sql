-- +goose Up
-- Phase 6 (spec RF-28, RF-29): the reading that the library scan made of each file it found and did not
-- register or move, because it was still being copied or had changed since the previous scan. The next scan
-- compares the file with this reading and processes it only when it did not change. The table holds only
-- the files still waiting: each scan replaces it.
CREATE TABLE library_seen (
    rel      TEXT    PRIMARY KEY COLLATE NOCASE,
    size     INTEGER NOT NULL CHECK (size >= 0),
    mtime_ms INTEGER NOT NULL
);

-- +goose Down
DROP TABLE library_seen;
