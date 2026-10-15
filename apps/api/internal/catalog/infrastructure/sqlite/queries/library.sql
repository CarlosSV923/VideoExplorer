-- name: InsertLibraryTitle :one
INSERT INTO library_titles (kind, title, disc_title, year, tmdb_id, id_tag, folder, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: GetLibraryTitleByFolder :one
SELECT id, kind, title, disc_title, year, tmdb_id, id_tag, folder, created_at, updated_at
FROM library_titles WHERE folder = ?;

-- name: InsertLibraryFile :one
INSERT INTO library_files (title_id, rel, kind, season, first_chapter, last_chapter, part, version,
                           language, forced, sdh, video_id, size, partial_hash, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: LibraryFileIDByRel :one
SELECT id FROM library_files WHERE rel = ?;

-- name: ListLibraryFiles :many
SELECT f.id, f.title_id, f.rel, f.kind, f.season, f.first_chapter, f.last_chapter, f.part, f.version,
       f.language, f.forced, f.sdh, v.rel AS video_rel, f.size, f.partial_hash, f.trash_entry_id, f.created_at
FROM library_files f
LEFT JOIN library_files v ON v.id = f.video_id
WHERE f.title_id = ?
ORDER BY f.id;

-- name: MoveLibraryFile :exec
UPDATE library_files SET rel = ?, version = ? WHERE id = ?;

-- name: TrashLibraryFile :exec
UPDATE library_files SET trash_entry_id = ? WHERE id = ?;

-- name: ListLibraryTitles :many
SELECT id, kind, title, disc_title, year, tmdb_id, id_tag, folder, created_at, updated_at
FROM library_titles ORDER BY id;

-- name: DeleteLibraryFile :exec
DELETE FROM library_files WHERE id = ?;

-- name: DeleteLibraryTitle :exec
DELETE FROM library_titles WHERE id = ?;

-- name: ListLibrarySeen :many
SELECT rel, size, mtime_ms FROM library_seen ORDER BY rel;

-- name: DeleteLibrarySeen :exec
DELETE FROM library_seen;

-- name: InsertLibrarySeen :exec
INSERT INTO library_seen (rel, size, mtime_ms) VALUES (?, ?, ?);

-- name: UpdateLibraryTitle :exec
UPDATE library_titles SET kind = ?, title = ?, disc_title = ?, year = ?, tmdb_id = ?, id_tag = ?, folder = ?, updated_at = ?
WHERE id = ?;

-- name: UpdateLibraryFile :exec
UPDATE library_files SET title_id = ?, rel = ?, season = ?, first_chapter = ?, last_chapter = ?, part = ?, version = ?,
       language = ?, forced = ?, sdh = ?
WHERE id = ?;

-- name: ReleaseLibraryFileRel :exec
UPDATE library_files SET rel = 'released:' || id WHERE id = ?;
