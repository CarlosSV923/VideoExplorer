-- +goose Up
-- Responses of TMDB (metadata context, spec §6). The key holds the operation,
-- its parameters and the language; the payload is the JSON of the answer;
-- expires_at is a Unix time in seconds. Expired rows stay: they are the
-- fallback when TMDB does not answer.
CREATE TABLE metadata_cache (
    key        TEXT    NOT NULL PRIMARY KEY,
    payload    BLOB    NOT NULL,
    expires_at INTEGER NOT NULL
);

-- +goose Down
DROP TABLE metadata_cache;
