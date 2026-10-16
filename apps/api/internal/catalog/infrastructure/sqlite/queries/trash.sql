-- name: InsertTrashEntry :one
INSERT INTO trash_entries (origin, reason, dir, trashed_at, batch_id, relative_path) VALUES (?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertTrashFile :exec
INSERT INTO trash_files (entry_id, name, size) VALUES (?, ?, ?);

-- name: ListTrashEntries :many
SELECT id, origin, reason, dir, trashed_at, batch_id, relative_path FROM trash_entries ORDER BY trashed_at DESC, id DESC;

-- name: ListTrashFiles :many
SELECT entry_id, name, size FROM trash_files ORDER BY id;

-- name: DeleteLibraryFilesInTrashEntry :exec
DELETE FROM library_files WHERE trash_entry_id = ?;

-- name: DeleteTrashEntry :exec
DELETE FROM trash_entries WHERE id = ?;

-- name: DeleteEmptyLibraryTitles :exec
DELETE FROM library_titles WHERE NOT EXISTS (SELECT 1 FROM library_files WHERE title_id = library_titles.id);
