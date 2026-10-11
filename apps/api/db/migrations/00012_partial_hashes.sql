-- +goose Up
-- Phase 4 (spec RF-24): the partial hash of a video is cached per file, in its own table
-- rather than as columns of media_probes. The two have their own lifecycles: the hash is
-- computed for every kept video, even one that ffprobe refuses, and it changes with its own
-- algorithm version (domain.PartialHashVersion), while a probe is only stored when ffprobe
-- answers. A file is named by its device and inode, which a rename inside one file system
-- keeps; size and mtime say whether the content is still the one that was hashed.
-- hash is the domain's FormatPartialHash, version prefix included.
CREATE TABLE partial_hashes (
    device   INTEGER NOT NULL,
    inode    INTEGER NOT NULL,
    size     INTEGER NOT NULL CHECK (size >= 0),
    mtime_ns INTEGER NOT NULL,
    hash     TEXT    NOT NULL,
    PRIMARY KEY (device, inode)
);

-- +goose Down
DROP TABLE partial_hashes;
