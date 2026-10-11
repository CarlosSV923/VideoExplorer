# Golden vectors

Business rules that exist in **both** the Go backend (authoritative) and the
TypeScript frontend (live previews and demo mode) are pinned here as data.
Both test suites load these files and must produce exactly the expected output,
so the two implementations cannot drift apart (spec RNF-08).

Each file is created in the phase that implements its rule, after the Jellyfin
spike (phase 0.5) has confirmed the naming rules they depend on. The table lists
the files and their state.

| File | Rule | Phase | Go consumer | TS consumer | Status |
|---|---|---|---|---|---|
| `naming-cases.json` | Phase 2: the title choice of spec §5: disc title `<T>` (`discTitle`: original Latin title, else English, else original) and chapter titles `<título cap.>` (`chapterTitle`: one title, several joined with ` + `, collapse of ` (1)`/`, Part 2`/final number, generic titles such as «Episode 3» count as no title). Phase 5 adds folders, file names, sanitizing (§5, steps 1 to 6), rejected inputs (`expected.error` names the field) and a file's extension: the longest known one (`extensionCases`) | 2 (titles), 5 (rest) | `metadata/domain` (titles, phase 2); `catalog/domain` (names, phase 5) | `modules/metadata/domain` (titles, phase 2); `modules/catalog/domain` (names, phase 5, preview of the final names in Assign, RF-25) | created: titles (`discTitle`, `chapterTitle`) |
| `filename-cases.json` | Filename analysis: movie or series, title, year, `SxxEyy`, `1x02`, several chapters per file, parts, subtitle language (RF-20, RF-22, RF-23) | 4 | `ingestion/domain` (`filename.go`) | `modules/ingestion/domain` (`filename.ts`, suggestions in the demo, RF-72) | created: movies and series, titles, years, `SxxEyy`, `1x02`, several chapters, parts, subtitle language and flags, folder fallback |
| `version-labels.json` | Version labels read from resolution and codec (`2160p`, `1080p HEVC`) and their sanitizing (RF-24, §5 `<versión>`) | 5 | `catalog/domain` | `modules/catalog/domain` (editable label before confirming, RF-24) | pending |

Changing a rule = change the vector first, then make both suites pass.

Vector files are JSON in UTF-8 with LF line endings, formatted by Prettier
(`task fmt`). The rules they pin are defined in [`docs/spec.md`](../docs/spec.md);
this README does not restate them.
