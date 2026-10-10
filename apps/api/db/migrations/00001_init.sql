-- +goose Up
-- Phase 1: the database is created and ready. The tables arrive with the
-- bounded contexts that own them (catalog, ingestion, metadata, identity).

-- +goose Down
-- Nothing to undo: the migration creates no tables.
