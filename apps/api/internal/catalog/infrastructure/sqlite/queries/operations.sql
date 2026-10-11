-- name: InsertOperation :exec
INSERT INTO library_operations (id, kind, source, steps, started, created_at) VALUES (?, ?, ?, ?, ?, ?);

-- name: AdvanceOperation :exec
UPDATE library_operations SET started = ? WHERE id = ?;

-- name: ListOperations :many
SELECT id, kind, source, steps, started, created_at FROM library_operations ORDER BY created_at, id;

-- name: DeleteOperation :exec
DELETE FROM library_operations WHERE id = ?;
