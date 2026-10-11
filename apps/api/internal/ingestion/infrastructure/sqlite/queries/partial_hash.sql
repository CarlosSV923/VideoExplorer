-- name: GetPartialHash :one
SELECT size, mtime_ns, hash FROM partial_hashes WHERE device = ? AND inode = ?;

-- name: UpsertPartialHash :exec
INSERT INTO partial_hashes (device, inode, size, mtime_ns, hash) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (device, inode) DO UPDATE SET size = excluded.size, mtime_ns = excluded.mtime_ns, hash = excluded.hash;
