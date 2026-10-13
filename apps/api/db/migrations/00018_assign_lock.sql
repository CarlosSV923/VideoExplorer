-- +goose Up
-- Phase 5 (spec RF-26): the lock of an open assignment. An entry of No asignados is locked by
-- the tab that opened Asignar: lock_token identifies that tab and lock_until (Unix milliseconds)
-- is when the lock expires without activity. An empty token is a free entry.
ALTER TABLE unassigned_entries ADD COLUMN lock_token TEXT    NOT NULL DEFAULT '';
ALTER TABLE unassigned_entries ADD COLUMN lock_until INTEGER NOT NULL DEFAULT 0 CHECK (lock_until >= 0);

-- +goose Down
ALTER TABLE unassigned_entries DROP COLUMN lock_until;
ALTER TABLE unassigned_entries DROP COLUMN lock_token;
