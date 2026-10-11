-- name: CreateUploadJob :exec
INSERT INTO upload_jobs (id, batch_id, file_name, relative_path, size, status, error, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetUploadJob :one
SELECT * FROM upload_jobs WHERE id = ?;

-- name: SaveUploadJob :exec
UPDATE upload_jobs SET status = ?, error = ?, received = ?, updated_at = ? WHERE id = ?;

-- name: ListRecentUploadJobs :many
SELECT j.id, j.batch_id, j.file_name, j.relative_path, j.size, j.received, j.status, j.error,
       j.created_at, j.updated_at,
       b.name AS batch_name, b.relation AS batch_relation, b.created_at AS batch_created_at
FROM upload_jobs j
JOIN upload_batches b ON b.id = j.batch_id
ORDER BY j.created_at DESC, j.rowid DESC
LIMIT ?;

-- name: ListUploadJobsByStatus :many
SELECT * FROM upload_jobs WHERE status = ? ORDER BY created_at, rowid;

-- name: ListStaleUploadJobs :many
SELECT * FROM upload_jobs WHERE status = ? AND updated_at < ? ORDER BY updated_at, rowid;

-- name: UploadJobExists :one
SELECT EXISTS(SELECT 1 FROM upload_jobs WHERE id = ?);
