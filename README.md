# VideoExplorer

[Español](README.es.md) · **Status: phase 1 in progress (API base)**

[![CI](https://github.com/CarlosSV923/VideoExplorer/actions/workflows/ci.yml/badge.svg)](https://github.com/CarlosSV923/VideoExplorer/actions/workflows/ci.yml)

Self-hosted web app for a home NAS (TrueNAS SCALE) to **upload, extract, classify, rename and organize movies and TV series** so that Jellyfin recognizes them. The interface follows the Netflix and Prime Video style and works from any browser: desktop, tablet, touch screen or gamepad.

> **VideoExplorer never plays video.** It manages the files; you watch them with Jellyfin.

## What it does

The full requirements are in [`docs/spec.md`](docs/spec.md). In short:

- **Upload** video, subtitle and archive files by dragging them anywhere in the window. Uploads are resumable and have no practical size limit.
- **Extract** zip, 7z and rar archives with the official 7-Zip, verify them against their CRC and keep only the videos and external subtitles. Discarded content goes to the trash, not deleted.
- **Assign** each file to a movie or a series, using TMDB data or a free name. Nothing enters the library without your confirmation.
- **Name** everything in the format Jellyfin expects. Folder and file names and the optional ID tag (`[tmdbid-ID]`) follow the Jellyfin convention. Duplicates can be replaced, kept as versions or skipped; there is a preview before saving, and a failed operation is rolled back.
- **Pick up** files copied over SMB into the Unassigned section, and import the library that is already on disk.
- **Browse** movies, series and collections, search, edit, move to the trash with restore, and download files or whole titles.
- **Refresh** Jellyfin for the affected folder, when it is configured.
- Spanish and English, with mouse, keyboard, touch and gamepad.

## What it does not do

- It does not play content.
- It does not download content from the internet (torrents, Usenet).
- It does not ship any movie or series.
- Plex is not supported in this first version (names, ID tags and refresh). It may be added in the future.
- Out of scope for the MVP: absolute-numbered anime, episodes spread over several files, extra libraries (anime, documentaries, concerts), users and roles, and extras such as trailers. The full list is in spec §8.

## Status

The repository currently contains the API base: configuration, SQLite, the OpenAPI contract with generated code, the password sign-in with sessions, the startup diagnostics of `GET /api/health` (the library and data folders must be writable, and `7zz` and `ffprobe` must start), the embedded web app, the tooling and the CI. The features above are built phase by phase; the progress is in [`docs/tasks.md`](docs/tasks.md).

Coming later: a demo without a backend, installation as a TrueNAS SCALE custom app, and screenshots.

## Stack

| | |
|---|---|
| Backend | Go, `net/http`, `log/slog`. SQLite (modernc, goose, sqlc). 7-Zip and ffprobe run as child processes |
| Frontend | React, Vite, TypeScript (strict), i18next |
| Contract | OpenAPI 3, spec-first ([`api/openapi.yaml`](api/openapi.yaml)). Go and TypeScript share golden test vectors in [`contracts/`](contracts/README.md) |
| Quality | Go tests with the race detector, golangci-lint v2, Vitest and Testing Library, ESLint, Prettier, Redocly, govulncheck in CI |

## Architecture

The backend and the frontend follow Domain-Driven Design: each bounded context has pure `domain` packages and talks to the outside world through ports and adapters.

| Context | Responsibility |
|---|---|
| `catalog` | Library, Unassigned section and trash. Every library change goes through a journaled operation engine |
| `ingestion` | Uploads, extraction, filtering, filename analysis and probing |
| `metadata` | TMDB client, behind an anti-corruption layer |
| `mediaservers` | Jellyfin refresh |
| `identity` | Single-password sign-in and sessions |
| `platform` | Shared kernel: configuration, HTTP server, logging |

Some principles hold across the code: files are streamed and never loaded into memory, paths are built on the server from IDs, and the TMDB and Jellyfin secrets never reach the browser. The layout is `apps/api` (Go), `apps/web` (React), `api/` (OpenAPI) and `contracts/` (golden vectors). The detailed design is in the spec.

## Requirements

- **Docker**. The Go tooling runs in containers, so Go does not need to be installed.
- **Node.js 24** or later, and **pnpm** (the version is pinned in `package.json`; `corepack enable` installs it).
- **[Task](https://taskfile.dev)** 3.

## Development

```bash
task install       # JS dependencies and Go modules
task dev:api       # Go API in Docker, http://localhost:8080
task dev:web       # Vite dev server, http://localhost:5173 (proxies /api)
task test          # Go and web tests
task lint          # OpenAPI, Go and web lint
task check         # everything CI runs
```

Other tasks: `task gen` (regenerates the code from `api/openapi.yaml` and the SQL queries; CI fails when it is stale), `task fmt` (formatters), `task build` (the SPA embedded in the Linux binary; `task build:api` and `task build:web` build each part), `task tidy` (`go mod tidy`). `task --list` shows them all. To check the API: `curl http://localhost:8080/api/health`.

`task dev:image` builds the development image (`deploy/dev/Dockerfile`): the Go toolchain plus the official 7-Zip (with RAR) and ffprobe, both at pinned versions with SHA-256 checks. Every Go task depends on it, so the first `task` that runs Go builds it. CI installs the same two binaries.

`task dev:api` passes every variable of `.env.local` into the container (it is optional, but without `SESSION_SECRET` and a password the API does not start). `LIBRARY_PATH` and `DATA_PATH` are always `./.dev/library` and `./.dev/data` in development, whatever `.env.local` says.

## Configuration

Every environment variable is listed in [`.env.example`](.env.example), with its default when it has one. Copy it to `.env.local` to override values locally, and never commit real values.

Access uses one shared password. Set `APP_PASSWORD_HASH` to an argon2id hash, which `echo 'your password' | task hash-password` prints (or set `APP_PASSWORD` in plain text instead), and `SESSION_SECRET` (at least 32 bytes) to sign the session cookie. Every API operation except `GET /api/health` and `POST /api/auth/login` requires a session; without one the API answers `401` as an RFC 9457 problem.

## Attribution and legal

This product uses the TMDB API but is not endorsed or certified by TMDB.

VideoExplorer does not include, download or distribute movies or series. Use it only with files you have the right to keep.

Licensed under the [MIT License](LICENSE).
