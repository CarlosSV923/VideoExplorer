-- +goose Up
-- RF-31a: a video row can disappear during a library scan while its subtitle rows remain in
-- the trash. Keep video_id as a logical registry link rather than a cascading SQLite FK; the
-- saved video_rel lets restoration reconnect subtitles if the video row was removed.
CREATE TABLE library_files_new (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    -- Keep the title row while any file is in the trash. It contains the TMDB link and
    -- naming data needed to restore a file after the title leaves the live library.
    title_id       INTEGER NOT NULL REFERENCES library_titles (id),
    rel            TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    kind           TEXT    NOT NULL CHECK (kind IN ('video', 'subtitle')),
    season         INTEGER CHECK (season IS NULL OR season >= 0),
    first_chapter  INTEGER CHECK (first_chapter IS NULL OR first_chapter >= 1),
    last_chapter   INTEGER CHECK (last_chapter IS NULL OR last_chapter >= first_chapter),
    part           INTEGER NOT NULL DEFAULT 0 CHECK (part >= 0),
    version        TEXT    NOT NULL DEFAULT '' CHECK (version NOT LIKE '%/%'),
    language       TEXT    NOT NULL DEFAULT '',
    forced         INTEGER NOT NULL DEFAULT 0 CHECK (forced IN (0, 1)),
    sdh            INTEGER NOT NULL DEFAULT 0 CHECK (sdh IN (0, 1)),
    -- Logical id of the video. It intentionally has no FK: a trashed subtitle must outlive a
    -- video that an import removes. Non-reuse is guaranteed by AUTOINCREMENT.
    video_id       INTEGER,
    video_rel      TEXT    NOT NULL DEFAULT '',
    trash_rel      TEXT    NOT NULL DEFAULT '',
    size           INTEGER NOT NULL CHECK (size >= 0),
    partial_hash   TEXT    NOT NULL DEFAULT '',
    trash_entry_id INTEGER REFERENCES trash_entries (id),
    created_at     TEXT    NOT NULL,
    CHECK (part = 0 OR version = ''),
    CHECK (part = 0 OR season IS NULL),
    CHECK ((kind = 'video') = (video_id IS NULL)),
    CHECK (kind = 'video' OR language <> ''),
    CHECK (kind = 'video' OR (season IS NULL AND first_chapter IS NULL AND last_chapter IS NULL AND part = 0 AND version = '')),
    CHECK (partial_hash = '' OR partial_hash LIKE 'ph1:%'),
    CHECK ((season IS NULL) = (first_chapter IS NULL)),
    CHECK ((first_chapter IS NULL) = (last_chapter IS NULL))
);

INSERT INTO library_files_new (id, title_id, rel, kind, season, first_chapter, last_chapter, part, version,
                               language, forced, sdh, video_id, video_rel, trash_rel, size, partial_hash, trash_entry_id, created_at)
SELECT f.id, f.title_id, CASE WHEN f.trash_entry_id IS NULL THEN f.rel ELSE 'trash:' || f.id END,
       f.kind, f.season, f.first_chapter, f.last_chapter, f.part, f.version,
       f.language, f.forced, f.sdh, f.video_id, coalesce(v.rel, ''),
       CASE WHEN f.trash_entry_id IS NULL THEN '' ELSE f.rel END,
       f.size, f.partial_hash, f.trash_entry_id, f.created_at
FROM library_files f LEFT JOIN library_files v ON v.id = f.video_id;

DROP INDEX library_files_title;
DROP INDEX library_files_video;
DROP INDEX library_files_trash;
DROP TABLE library_files;
ALTER TABLE library_files_new RENAME TO library_files;
CREATE INDEX library_files_title ON library_files (title_id);
CREATE INDEX library_files_video ON library_files (video_id);
CREATE INDEX library_files_trash ON library_files (trash_entry_id);

-- +goose Down
-- A cascading FK cannot be restored without risking loss of subtitle trash metadata. Keep
-- the non-cascading logical relations on rollback as well.
