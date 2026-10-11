-- name: CreateBatch :exec
INSERT INTO upload_batches (id, name, relation, created_at) VALUES (?, ?, ?, ?);

-- name: GetBatch :one
SELECT id, name, relation, created_at FROM upload_batches WHERE id = ?;
