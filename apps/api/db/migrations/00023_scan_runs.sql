-- +goose Up
-- Phase 6 (spec RF-28): the time each scan of the library last finished, so that the times survive a
-- restart. One row per scan (unassigned: _unassigned/, library: Movies/ and TV Shows/). finished_at is a
-- UTC timestamp in the layout of the other catalog tables.
CREATE TABLE scan_runs (
    kind        TEXT PRIMARY KEY CHECK (kind IN ('unassigned', 'library')),
    finished_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE scan_runs;
