# VideoExplorer — Guía para asistentes de IA

Self-hosted web app (TrueNAS SCALE) to upload, extract, classify (movies and series via TMDB), rename and organize video files so Jellyfin recognizes them (file names and ID tags follow the Jellyfin convention; Plex is out of scope for this version, spec §8), with a Netflix / Prime Video style UI. It never plays content. Public portfolio project with a backend-less demo on Vercel. Sister project of GameExplorer (`../GameExplorer`), whose architecture, tooling and conventions it reuses.

## Source of truth
- Requirements: [`docs/spec.md`](docs/spec.md). Do not assume requirements that are not documented there; ask the user (one question at a time, with options and a recommendation) and document first. Open points live in spec §9.
- Work status and order: [`docs/tasks.md`](docs/tasks.md). Only work on the current phase; never start a phase whose dependency is not done.
- HTTP contract: `api/openapi.yaml` (spec-first; generated code is never edited by hand).
- Rules shared by Go and TS: `contracts/` golden vectors (naming, sanitizing, filename parsing, version labels). Change the vector first, then both implementations.
- Before UI work use the design plugin skills (`design:design-critique`, `design:accessibility-review`, `design:design-handoff`).

## Workflow: orchestrator + builder
- The **main session is the orchestrator**: it owns `docs/spec.md`, `docs/tasks.md`, `CLAUDE.md` and `.claude/`, asks the user every undefined point (one question at a time, with options and a recommendation) and designs the UI (mockups, design reviews, handoff).
- The **builder** writes all code, tests and tooling. It gets **one task per order**, commits locally (only its files, never pushes) and reports back. Two builders exist:
  - **Codex** (default since 2026-10-16): `codex exec -m gpt-6-luna -c model_reasoning_effort=high -c approval_policy=never -s danger-full-access -o <report.md> "<task>"`, run in the background from the repo root; it follows `AGENTS.md` (same rules and report format as the Claude builder). The sandboxed modes cannot reach Docker or write `.git`. No hook protects the orchestrator's files: after every run, check that the diff touches nothing under `docs/`, `CLAUDE.md`, `AGENTS.md` or `.claude/`, and revert it if it does. Follow-ups: run `codex exec` again with the context in the prompt (`codex exec resume --last` also works).
  - **`video-builder` subagent** (`.claude/agents/video-builder.md`, Haiku, effort high) as fallback. A PreToolUse hook (`.claude/hooks/protect-docs.mjs`) blocks it from writing the orchestrator's files.
- If the builder hits something undefined it stops and reports; the orchestrator asks the user, documents the answer in the spec, and resumes the builder with its context.
- Before marking a task `[x]`, the orchestrator reviews the builder's diff against the spec and runs `task check`. Pushes to GitHub only with the user's confirmation.
- Before launching the builder on a phase, every point it needs must be defined in the spec.

## Layout (same as GameExplorer)
| Path | What |
|---|---|
| `apps/api` | Go module. `cmd/videoexplorer` is the composition root; `internal/<context>/{domain,application,infrastructure,interfaces}` |
| `apps/web` | React + Vite + TS. `src/app`, `src/modules/<context>/{domain,application,infrastructure/{http,demo},ui}`, `src/shared` |
| `api/` | OpenAPI contract |
| `contracts/` | Golden test vectors |
| `docs/` | Spec, tasks, design handoff |

Bounded contexts: `catalog` (library, unassigned section and trash; every library change goes through the journaled operation engine), `ingestion` (uploads, extraction, filtering, filename analysis, probing), `metadata` (TMDB anti-corruption layer), `mediaservers` (Jellyfin refresh), `identity`, `platform` (shared kernel).

## Rules
- DDD: `domain` packages are pure (no I/O, no framework imports). Cross-context calls go through application ports.
- Huge files (tens of GB): never buffer file contents in memory. Stream, `rename`, delegate to `7zz` and `ffprobe` child processes, serve with `http.ServeContent`.
- Paths are built server-side from IDs; never trust client paths. TMDB and Jellyfin secrets never reach the browser.
- Nothing is assigned to the library without explicit user confirmation.
- The frontend never knows which adapter set is active; `VITE_DATA_SOURCE` is read only in `src/app`.
- Mouse, keyboard, touch and gamepad must all work; gamepad glyphs only when a gamepad is detected.
- UI copy: Spanish default, English available, via i18n keys (ESLint `i18next/no-literal-string`).

## Commands
Same Taskfile as GameExplorer: `task check` runs everything CI runs except the generated-code drift check, which is CI-only (it compares against git). Go tooling runs in Docker (Windows Application Control blocks freshly compiled Go binaries on the dev machine). After editing `api/openapi.yaml` or SQL queries run `task gen`.

## Conventions
- Go: gofumpt + goimports, golangci-lint v2, table tests, `t.Parallel()`.
- TS: strict + `exactOptionalPropertyTypes`, ESLint `strictTypeChecked`, Prettier (no semicolons, single quotes).
- Commits: small and focused; every change keeps `task check` green.
- Docs: `docs/` in Spanish; `README.md` (EN) and `README.es.md` (ES) stay in sync.
