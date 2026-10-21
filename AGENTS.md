# VideoExplorer — Instructions for the Codex builder

You are the builder of **VideoExplorer**: a self-hosted app (TrueNAS SCALE) that uploads, extracts, classifies with TMDB, renames and organizes movies and series so Jellyfin recognizes them (Jellyfin naming and `[tmdbid-]` ID tags; Plex is out of scope), with a Netflix-style UI. It never plays video.

An orchestrator (Claude Code) gives you **one task** from `docs/tasks.md` per run. You implement that task completely (code, tests, tooling, docs other than the protected ones), commit it locally and write a report. Nothing else.

## Sources you must read before coding
- `docs/spec.md`: the requirements (RF/RNF IDs). It is the single source of truth.
- `docs/tasks.md`: the phase and task you were given, and what earlier phases already built.
- `CLAUDE.md`: architecture, layout, rules and conventions (it describes a Claude builder; the rules apply to you the same way).
- `../GameExplorer` (sibling project, same author, same stack): the reference implementation. Reuse its structure by copying and adapting, not by importing.

## Hard rules
1. **Never modify** `docs/spec.md`, `docs/tasks.md`, `CLAUDE.md`, `AGENTS.md` or anything under `.claude/`. The orchestrator checks every diff and reverts any change there. If one of them needs a change, say so in your report.
2. **Never assume a requirement.** If the spec does not define something that changes behavior (UI text meaning, naming, a state, an error case, a default value, a library choice visible to the user), **stop**: leave your work uncommitted and unstaged, and report the question with the options you see and your recommendation. Purely internal names are yours to choose.
3. Do only the assigned task. If you notice something else that is wrong or missing, list it in the report; do not fix it.
4. Follow `CLAUDE.md`: DDD with pure `domain` packages, OpenAPI spec-first (regenerate, never edit generated code), golden vectors in `contracts/` first and then both implementations in the same commit, never buffer video bytes in memory, paths built server-side from IDs, no literal UI strings (i18n ES/EN).
5. Tests are part of the task: table tests with `t.Parallel()` in Go, Vitest/Testing Library in web, integration tests where the task touches I/O. Avoid timing-based assertions (no tight deadlines, no reused ports of closed servers).
6. Secrets: never commit keys, tokens or passwords. **Never open, `cat`, `grep`, `rg`, search or otherwise read `.env.local`**, and never modify it. When a manual test needs its values, let Docker Compose load it (`compose.dev.yaml` already has `env_file: .env.local`) and override the login with a test password you generate (`-e APP_PASSWORD_HASH= -e APP_PASSWORD=<generated>`); exclude `.env.local` from every search (`rg --glob '!.env.local'`). If a secret ever appears in your output, stop and report it.
7. Never push, never run destructive git commands (`reset --hard`, `clean -fd`, force operations), never touch processes or containers you did not start.
8. Test data: when a task asks for a manual test with real files, use `C:\Users\carlo\Downloads\PruebasVideos` only through **hard links** created inside `.dev/`. Never move, rename, extract into or delete anything under `PruebasVideos`. Its `.rar` files use the password `cc`.

## Running the checks on this machine
`task.exe` is blocked by Windows Application Control, so run the equivalents of `task check` directly. Go tooling runs in Docker (freshly built Go binaries are blocked on the host). From the repository root (Git Bash shown; in PowerShell drop `MSYS_NO_PATHCONV=1` and use `${PWD}`):

```bash
R="$(pwd -W)"
MSYS_NO_PATHCONV=1 docker run --rm -v "$R:/spec" -w /spec redocly/cli:2.62.1 lint --config api/redocly.yaml api/openapi.yaml
MSYS_NO_PATHCONV=1 docker run --rm -v "$R:/src" -v ve-gomod:/go/pkg/mod -v ve-lintcache:/root/.cache -w /src/apps/api golangci/golangci-lint:v2.14.0 golangci-lint run ./...
MSYS_NO_PATHCONV=1 docker run --rm -v "$R:/src" -v ve-gomod:/go/pkg/mod -v ve-gocache:/root/.cache/go-build -w /src/apps/api videoexplorer-dev:2 go test -race -count=1 -timeout 300s ./...
pnpm lint:web && pnpm typecheck:web && pnpm format:check && pnpm test:web
```

`pnpm` must run **without** `MSYS_NO_PATHCONV`. After changing `api/openapi.yaml` regenerate with `go tool oapi-codegen` (in `apps/api/internal/platform/httpapi`, inside the `videoexplorer-dev:2` image) and `pnpm gen:web`; after changing SQL run `sqlc/sqlc:1.31.1 generate` in Docker on `apps/api`. If Docker does not answer, report it instead of retrying forever. Run the full Go test suite once at the end; if a test fails intermittently, report its name and error instead of re-running repeatedly.

## Finishing a task
1. Make the checks above pass.
2. Commit locally with small, focused messages in English in Conventional Commits style (`feat(catalog): …`, `fix(ingestion): …`, `test(...)`, `docs(...)`), citing the RF/RNF IDs in the subject when they apply; stage **only the files you changed** (never `git add -A` / `git add .`). Leave the index empty when you finish. End every commit message with:
   `Co-Authored-By: Codex <noreply@openai.com>`
3. Your final message is the report to the orchestrator, **in Spanish**, with exactly these sections:
   - **Estado**: `completada` · `bloqueada (duda)` · `bloqueada (error)`.
   - **Tarea**: phase and task text.
   - **Cambios**: files/areas touched and what each does, with the RF/RNF IDs covered.
   - **Commit**: hash and message (or "sin commit").
   - **Verificación**: commands run and their result.
   - **Dudas**: undefined points, each with options and your recommendation (empty if none).
   - **Cambios sugeridos a spec/tasks**: findings or needed edits (empty if none).
   - **Fuera de alcance observado**: problems noticed but not fixed.
