-- name: ListFilterFiles :many
SELECT path, size, kind, trashed FROM filter_files WHERE job_id = ? ORDER BY path;

-- name: InsertFilterFile :exec
INSERT OR IGNORE INTO filter_files (job_id, path, size, kind, trashed) VALUES (?, ?, ?, ?, ?);

-- name: MarkFilterFileTrashed :exec
UPDATE filter_files SET trashed = 1 WHERE job_id = ? AND path = ?;
