-- name: InsertUnassignedEntry :one
INSERT INTO unassigned_entries (source, name, loose, batch_id, relation, origin, created_at) VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertUnassignedFile :exec
INSERT INTO unassigned_files (entry_id, source, rel, size) VALUES (?, ?, ?, ?);

-- name: GetUnassignedEntryBySource :one
SELECT id, source, name, loose, batch_id, relation, origin, created_at, lock_token, lock_until FROM unassigned_entries WHERE source = ?;

-- name: GetUnassignedEntryByID :one
SELECT id, source, name, loose, batch_id, relation, origin, created_at, lock_token, lock_until FROM unassigned_entries WHERE id = ?;

-- name: ListUnassignedFiles :many
SELECT source, rel, size FROM unassigned_files WHERE entry_id = ? ORDER BY rowid;

-- name: ListUnassignedNames :many
SELECT name FROM unassigned_entries ORDER BY id;

-- name: TakeUnassignedLock :execrows
UPDATE unassigned_entries SET lock_token = ?, lock_until = ? WHERE id = ? AND (lock_token = '' OR lock_until <= ? OR lock_token = ?);

-- name: ReleaseUnassignedLock :exec
UPDATE unassigned_entries SET lock_token = '', lock_until = 0 WHERE id = ? AND lock_token = ?;

-- name: ClearUnassignedLock :exec
UPDATE unassigned_entries SET lock_token = '', lock_until = 0 WHERE id = ?;

-- name: DeleteUnassignedFile :exec
DELETE FROM unassigned_files WHERE entry_id = ? AND source = ?;

-- name: DeleteUnassignedEntry :exec
DELETE FROM unassigned_entries WHERE id = ?;
