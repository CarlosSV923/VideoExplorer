-- name: CreateUploadJob :exec
INSERT INTO upload_jobs (id, batch_id, file_name, relative_path, size, status, error, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetUploadJob :one
SELECT id, batch_id, file_name, relative_path, size, status, error, created_at, updated_at
FROM upload_jobs WHERE id = ?;

-- name: SaveUploadJob :exec
UPDATE upload_jobs SET status = ?, error = ?, updated_at = ? WHERE id = ?;
