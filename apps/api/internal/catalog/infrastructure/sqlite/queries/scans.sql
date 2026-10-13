-- name: ListScanRuns :many
SELECT kind, finished_at FROM scan_runs ORDER BY kind;

-- name: UpsertScanRun :exec
INSERT INTO scan_runs (kind, finished_at) VALUES (?, ?) ON CONFLICT (kind) DO UPDATE SET finished_at = excluded.finished_at;
