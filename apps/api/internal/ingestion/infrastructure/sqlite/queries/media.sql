-- name: GetMediaProbe :one
SELECT size, mtime_ns, info FROM media_probes WHERE device = ? AND inode = ?;

-- name: UpsertMediaProbe :exec
INSERT INTO media_probes (device, inode, size, mtime_ns, info) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (device, inode) DO UPDATE SET size = excluded.size, mtime_ns = excluded.mtime_ns, info = excluded.info;
