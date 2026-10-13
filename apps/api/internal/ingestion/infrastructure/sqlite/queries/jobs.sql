-- name: CreateUploadJob :exec
INSERT INTO upload_jobs (id, batch_id, file_name, relative_path, size, status, error, parts_total, volume_set, volume_index, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetUploadJob :one
SELECT * FROM upload_jobs WHERE id = ?;

-- name: SaveUploadJob :exec
UPDATE upload_jobs SET status = ?, error = ?, received = ?, progress = ?, volume_group = ?, filter_done = ?, updated_at = ? WHERE id = ?;

-- name: ListRecentUploadJobs :many
SELECT j.id, j.batch_id, j.file_name, j.relative_path, j.size, j.received, j.progress, j.status, j.error,
       j.parts_total, j.volume_set, j.volume_index, j.volume_group, j.filter_done, j.created_at, j.updated_at,
       b.name AS batch_name, b.relation AS batch_relation, b.created_at AS batch_created_at
FROM upload_jobs j
JOIN upload_batches b ON b.id = j.batch_id
ORDER BY j.created_at DESC, j.rowid DESC
LIMIT ?;

-- name: ListUploadJobsByStatus :many
SELECT * FROM upload_jobs WHERE status = ? ORDER BY created_at, rowid;

-- name: ListStaleUploadJobs :many
SELECT * FROM upload_jobs WHERE status = ? AND updated_at < ? ORDER BY updated_at, rowid;

-- name: ListUploadJobsByVolumeGroup :many
SELECT * FROM upload_jobs WHERE volume_group = ? ORDER BY volume_index, rowid;

-- name: UploadJobExists :one
SELECT EXISTS(SELECT 1 FROM upload_jobs WHERE id = ?);

-- name: ListUploadJobsByBatch :many
SELECT * FROM upload_jobs WHERE batch_id = ? ORDER BY created_at, rowid;

-- name: BatchHasOpenUploadJobs :one
SELECT EXISTS(SELECT 1 FROM upload_jobs WHERE batch_id = ? AND status IN ('uploading', 'uploaded', 'waiting_parts', 'extracting', 'needs_password'));

-- name: BatchHasFilteringUploadJobs :one
SELECT EXISTS(SELECT 1 FROM upload_jobs WHERE batch_id = ? AND status = 'filtering' AND filter_done = 0);
