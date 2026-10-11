-- +goose Up
-- Phase 4 (spec RF-24, RF-44): the technical reading of a video (ffprobe) is cached per
-- file. A file is named by its device and inode, which a rename inside one file system
-- keeps; size and mtime say whether the content is still the one that was read. A changed
-- size or mtime replaces the row (the same device and inode), so no stale row is left behind.
-- info is the JSON of the domain's MediaInfo.
CREATE TABLE media_probes (
    device   INTEGER NOT NULL,
    inode    INTEGER NOT NULL,
    size     INTEGER NOT NULL CHECK (size >= 0),
    mtime_ns INTEGER NOT NULL,
    info     TEXT    NOT NULL,
    PRIMARY KEY (device, inode)
);

-- +goose Down
DROP TABLE media_probes;
