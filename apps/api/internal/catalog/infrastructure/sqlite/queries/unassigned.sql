-- name: InsertUnassignedEntry :one
INSERT INTO unassigned_entries (source, name, loose, batch_id, relation, origin, created_at) VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertUnassignedFile :exec
INSERT INTO unassigned_files (entry_id, source, rel, size, moved) VALUES (?, ?, ?, ?, ?);

-- name: GetUnassignedEntryBySource :one
SELECT id, source, name, loose, batch_id, relation, origin, created_at FROM unassigned_entries WHERE source = ?;

-- name: ListUnassignedFiles :many
SELECT source, rel, size, moved FROM unassigned_files WHERE entry_id = ? ORDER BY rowid;

-- name: ListUnassignedNames :many
SELECT name FROM unassigned_entries ORDER BY id;

-- name: MarkUnassignedFileMoved :exec
UPDATE unassigned_files SET moved = 1 WHERE entry_id = ? AND source = ?;
