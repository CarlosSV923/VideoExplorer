# VideoExplorer

[Español](README.es.md) · **Status: phases 0–9 complete; phase 10 in progress**

[![CI](https://github.com/CarlosSV923/VideoExplorer/actions/workflows/ci.yml/badge.svg)](https://github.com/CarlosSV923/VideoExplorer/actions/workflows/ci.yml)

VideoExplorer is a self-hosted web app for a home NAS (TrueNAS SCALE). It organizes movies and TV series so Jellyfin recognizes them, with a Netflix-style library UI. It manages files; it never plays video.

## What it does today

- Browses a Netflix-style library of movies, series, seasons, episodes and collections.
- Uploads one or more files with resumable [tus](https://tus.io/) uploads, including zip, 7z and rar archives.
- Extracts archives with 7-Zip, filters videos and external subtitles, and sends discarded content to the trash instead of deleting it.
- Moves uploaded or SMB-copied files through **Unassigned** and **Assign**, with title suggestions from TMDB or a free title.
- Names folders, videos and subtitles using the Jellyfin convention, including optional `[tmdbid-ID]` tags, duplicate decisions, versions, parts and episode numbering.
- Edits and reassigns library files, downloads files or titles, restores items from the trash, and permanently purges trash entries.
- Provides settings for language, scans, TMDB and Jellyfin, including a notice to refresh Jellyfin after library changes when Jellyfin is configured.
- Supports Spanish and English with mouse, keyboard, touch and gamepad input.

The detailed requirements and current work are in [`docs/spec.md`](docs/spec.md) and [`docs/tasks.md`](docs/tasks.md). The design system and responsive behavior are in [`docs/design-handoff.md`](docs/design-handoff.md). The naming and classification vectors live in [`contracts/`](contracts/README.md).

## Online demo

**Live demo:** link pending publication. The demo runs without a backend; its in-memory changes reset when the page reloads. TMDB images load from `image.tmdb.org`.

## Project status

Phases 0 through 9 are complete. Phase 10 (the frontend with HTTP adapters) is in progress: the library, upload, Unassigned, Assign, edit, trash, settings and responsive UI flows are implemented, while final real-browser validation and its follow-up fixes are tracked in [`docs/tasks.md`](docs/tasks.md). Phase 11 is the backend-less demo, followed by the TrueNAS production packaging in phase 12.

## Development

Requirements: Docker, Node.js 24 or later, pnpm 12.9.1, and Task 3.

Install dependencies and start the services:

```bash
task install
task dev:api       # Go API in Docker at http://localhost:8080
task dev:web       # Vite frontend at http://localhost:5173
```

The API development service is defined in [`compose.dev.yaml`](compose.dev.yaml). It uses `./.dev/library` and `./.dev/data` as local development data and passes optional local configuration from `.env.local`. Do not commit secrets.

Useful checks:

```bash
task check         # OpenAPI, Go, web lint/tests, vulnerability scan and builds
task gen           # Regenerate OpenAPI and SQL code
task build:web     # Build the frontend
task build:demo    # Build with the demo adapters
task e2e           # Run Playwright E2E in Docker (desktop and touch mobile)
```

See [`Taskfile.yml`](Taskfile.yml) for all available tasks. Go tooling runs in Docker so development matches CI and production Linux paths.

## Architecture

The backend and frontend use Domain-Driven Design with pure domain packages and ports/adapters. The main areas are:

| Area | Responsibility |
|---|---|
| `apps/api` | Go API, SQLite, operations, extraction and file organization |
| `apps/web` | React, Vite, TypeScript, i18n and responsive UI |
| `api` | OpenAPI source of truth |
| `contracts` | Shared golden vectors |
| `docs` | Requirements, task status and design handoff |

Files are streamed and are not buffered as video data in memory. Paths are built server-side from IDs, and TMDB and Jellyfin secrets never reach the browser. Plex is out of scope for this version.

## Legal

VideoExplorer does not include, download or distribute movies or series. Use it only with files you have the right to keep. TMDB is used for metadata; VideoExplorer is not endorsed or certified by TMDB.

Licensed under the [MIT License](LICENSE).
