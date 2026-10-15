-- name: GetArchiveProbe :one
SELECT size, mtime_ms, archive FROM archive_probes WHERE path = ?;

-- name: UpsertArchiveProbe :exec
INSERT INTO archive_probes (path, size, mtime_ms, archive) VALUES (?, ?, ?, ?)
    ON CONFLICT (path) DO UPDATE SET size = excluded.size, mtime_ms = excluded.mtime_ms, archive = excluded.archive;

-- name: ListArchiveProbePaths :many
SELECT path FROM archive_probes;

-- name: DeleteArchiveProbe :exec
DELETE FROM archive_probes WHERE path = ?;
