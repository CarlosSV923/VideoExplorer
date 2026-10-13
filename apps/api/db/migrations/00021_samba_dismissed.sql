-- +goose Up
-- Phase 6 (spec RF-27a): "Quitar" takes a failed Samba copy off No asignados. The job is kept, so that
-- GET /jobs still shows it; only the section stops listing it.
ALTER TABLE upload_jobs ADD COLUMN dismissed INTEGER NOT NULL DEFAULT 0 CHECK (dismissed IN (0, 1));

-- +goose Down
ALTER TABLE upload_jobs DROP COLUMN dismissed;
