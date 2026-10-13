-- +goose Up
-- Phase 5 (spec RF-21, RF-22, RF-24, RF-45): the registry of the library. A title is a movie or a
-- series whose folder lives in MOVIES_DIR or SERIES_DIR; its files are the videos and the subtitles
-- kept in that folder. The registry is written by the commit of each change (RNF-04); the disk
-- stays the source of the names. A file in the trash keeps its row, with trash_entry_id set, so
-- that restoring it (RF-31) can put it back where it was.
CREATE TABLE library_titles (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    -- A movie (Movies/) or a series (TV Shows/).
    kind       TEXT    NOT NULL CHECK (kind IN ('movie', 'series')),
    -- The title as the user sees it: the TMDB title, or the free title (RF-21).
    title      TEXT    NOT NULL CHECK (title <> ''),
    -- <T> of spec §5, the disc title chosen for the folder and its files.
    disc_title TEXT    NOT NULL CHECK (disc_title <> ''),
    -- <A> of spec §5; 0 when unknown.
    year       INTEGER NOT NULL DEFAULT 0 CHECK (year BETWEEN 0 AND 9999),
    -- The TMDB id; NULL for a free title (spec §6).
    tmdb_id    INTEGER CHECK (tmdb_id IS NULL OR tmdb_id > 0),
    -- The tag [tmdbid-ID] is on (1) or was removed (0) (RF-45). A free title has no tag.
    id_tag     INTEGER NOT NULL DEFAULT 0 CHECK (id_tag IN (0, 1)),
    -- The title's folder inside the library, with "/" separators: unique in any letter case.
    folder     TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL,
    CHECK (tmdb_id IS NOT NULL OR id_tag = 0)
);

CREATE TABLE library_files (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    title_id       INTEGER NOT NULL REFERENCES library_titles (id) ON DELETE CASCADE,
    -- The file's path inside the library, with "/" separators; built by the server (RNF-02).
    rel            TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    kind           TEXT    NOT NULL CHECK (kind IN ('video', 'subtitle')),
    -- Series chapters: the season (0 holds the specials) and the first and last chapter.
    -- NULL for movies and for subtitles (a subtitle takes its video's place).
    season         INTEGER CHECK (season IS NULL OR season >= 0),
    first_chapter  INTEGER CHECK (first_chapter IS NULL OR first_chapter >= 1),
    last_chapter   INTEGER CHECK (last_chapter IS NULL OR last_chapter >= first_chapter),
    -- Movies: the part number (RF-22); 0 for the whole movie. Always 0 for series.
    part           INTEGER NOT NULL DEFAULT 0 CHECK (part >= 0),
    -- <versión> of RF-24: "" for the only file. A part and a version are exclusive (RF-22).
    version        TEXT    NOT NULL DEFAULT '' CHECK (version NOT LIKE '%/%'),
    -- Subtitles: <idioma> of spec §5 and the .forced and .sdh flags (RF-23).
    language       TEXT    NOT NULL DEFAULT '',
    forced         INTEGER NOT NULL DEFAULT 0 CHECK (forced IN (0, 1)),
    sdh            INTEGER NOT NULL DEFAULT 0 CHECK (sdh IN (0, 1)),
    -- The video a subtitle goes with (RF-23); NULL for a video.
    video_id       INTEGER REFERENCES library_files (id) ON DELETE CASCADE,
    size           INTEGER NOT NULL CHECK (size >= 0),
    -- The partial hash of a video (RF-24), "" until it is known.
    partial_hash   TEXT    NOT NULL DEFAULT '',
    -- The trash entry the file is in; NULL while it is in the library (RF-30).
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
CREATE INDEX library_files_title ON library_files (title_id);
CREATE INDEX library_files_video ON library_files (video_id);
CREATE INDEX library_files_trash ON library_files (trash_entry_id);

-- +goose Down
DROP INDEX library_files_trash;
DROP INDEX library_files_video;
DROP INDEX library_files_title;
DROP TABLE library_files;
DROP TABLE library_titles;
