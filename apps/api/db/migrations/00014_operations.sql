-- +goose Up
-- Phase 5 (spec RNF-04, RF-25, RF-44): the journal of the library changes. A row exists
-- from before the first step of a change until its catalog records are committed, and the
-- commit deletes the row in the same transaction. A row that a restart finds is undone
-- (catalog application engine). steps is JSON [{kind, from, to}] in order, with absolute
-- paths; started counts the steps begun.
CREATE TABLE library_operations (
    id         TEXT    PRIMARY KEY,
    kind       TEXT    NOT NULL,
    source     TEXT    NOT NULL,
    steps      TEXT    NOT NULL,
    started    INTEGER NOT NULL DEFAULT 0 CHECK (started >= 0),
    created_at TEXT    NOT NULL
);

-- The files of No asignados are recorded once they are in place, by the commit of their
-- move: the flag that marked a file as moved before the journal is no longer needed.
ALTER TABLE unassigned_files DROP COLUMN moved;

-- +goose Down
ALTER TABLE unassigned_files ADD COLUMN moved INTEGER NOT NULL DEFAULT 1 CHECK (moved IN (0, 1));
DROP TABLE library_operations;
