-- name: CreateBatch :exec
INSERT INTO upload_batches (id, name, relation, origin, created_at) VALUES (?, ?, ?, ?, ?);

-- name: GetBatch :one
SELECT id, name, relation, origin, created_at FROM upload_batches WHERE id = ?;

-- name: DeleteBatch :exec
DELETE FROM upload_batches WHERE id = ?;
