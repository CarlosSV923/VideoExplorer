-- name: GetCachedResponse :one
SELECT payload, expires_at FROM metadata_cache WHERE key = ?;

-- name: PutCachedResponse :exec
INSERT INTO metadata_cache (key, payload, expires_at) VALUES (?, ?, ?)
ON CONFLICT (key) DO UPDATE SET payload = excluded.payload, expires_at = excluded.expires_at;
