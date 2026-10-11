-- name: InsertTrashEntry :one
INSERT INTO trash_entries (origin, reason, dir, trashed_at) VALUES (?, ?, ?, ?) RETURNING id;

-- name: InsertTrashFile :exec
INSERT INTO trash_files (entry_id, name, size) VALUES (?, ?, ?);

-- name: ListTrashEntries :many
SELECT id, origin, reason, dir, trashed_at FROM trash_entries ORDER BY trashed_at, id;

-- name: ListTrashFiles :many
SELECT entry_id, name, size FROM trash_files ORDER BY id;
