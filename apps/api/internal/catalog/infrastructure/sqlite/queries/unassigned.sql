-- name: InsertUnassignedEntry :one
INSERT INTO unassigned_entries (source, name, loose, batch_id, relation, origin, created_at) VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertUnassignedFile :exec
INSERT INTO unassigned_files (entry_id, source, rel, size) VALUES (?, ?, ?, ?);

-- name: GetUnassignedEntryBySource :one
SELECT id, source, name, loose, batch_id, relation, origin, created_at FROM unassigned_entries WHERE source = ?;

-- name: ListUnassignedFiles :many
SELECT source, rel, size FROM unassigned_files WHERE entry_id = ? ORDER BY rowid;

-- name: ListUnassignedNames :many
SELECT name FROM unassigned_entries ORDER BY id;
