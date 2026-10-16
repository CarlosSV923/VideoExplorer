-- +goose Up
-- Phase 7 (spec RF-40, RF-43a): the last successful count of missing episodes in normal seasons.
-- NULL means that a linked series has not been calculated yet; a count is refreshed by the title page or
-- the background library scan. Movies and free titles leave this value NULL.
ALTER TABLE library_titles ADD COLUMN missing_episodes INTEGER CHECK (missing_episodes IS NULL OR missing_episodes >= 0);

-- +goose Down
ALTER TABLE library_titles DROP COLUMN missing_episodes;
