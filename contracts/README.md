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
| `naming-cases.json` | Phase 2: the title choice of spec §5: disc title `<T>` (`discTitle`: original Latin title, else English, else original) and chapter titles `<título cap.>` (`chapterTitle`: one title, several joined with ` + `, collapse of ` (1)`/`, Part 2`/final number, generic titles such as «Episode 3» count as no title). Phase 4: the sanitizing of spec §5, steps 1 to 6 (`sanitize`: NFC, `: `, `/` and `\`, quotes, signs and control characters, white space, trailing dots and spaces of the whole component), and the fallback name `Sin nombre` of an entry whose name is left empty (a phase 4 decision, not in §5). Phase 5 adds the subtitle language `<idioma>` (`subtitleLanguage`: ISO 639-1 with the regions Jellyfin recognizes, Latin American Spanish as `es-419`) and the library paths (`paths`: the folder and the file of a movie, version or part, chapter, several chapters, special, subtitle; flags `.forced` and `.sdh`; the tag `[tmdbid-ID]` on by default; the 255-byte cut; MOVIES_DIR and SERIES_DIR). A rejected input has `expected.error`, which names the field. The extension is written in lower case; the known extensions have no compound form, so the file's extension needs no vector of its own | 2 (titles), 4 (sanitizing), 5 (rest) | `metadata/domain` (titles, phase 2); `catalog/domain` (sanitizing, phase 4, `SanitizeName` and `EntryName` for the names of unassigned entries; `paths.go` and `language.go`, phase 5) | `modules/metadata/domain` (titles, phase 2); `modules/catalog/domain` (sanitizing, phase 4, `sanitizeName` and `entryName`; `paths.ts` and `language.ts`, phase 5, preview of the final names in Assign, RF-25) | created: titles (`discTitle`, `chapterTitle`), sanitizing (`sanitize`), subtitle language (`subtitleLanguage`), library paths (`paths`) |
| `filename-cases.json` | Filename analysis: movie or series, title, year, `SxxEyy`, `1x02`, several chapters per file, parts, subtitle language (RF-20, RF-22, RF-23) | 4 | `ingestion/domain` (`filename.go`) | `modules/ingestion/domain` (`filename.ts`, suggestions in the demo, RF-72) | created: movies and series, titles, years, `SxxEyy`, `1x02`, several chapters, parts, subtitle language and flags, folder fallback |
| `version-labels.json` | Version labels (RF-24, §5 `<versión>`) of the versions of one place: the resolution (`2160p`, `1080p`, with a 2 % reach so that a letterboxed 1920×800 is `1080p`), the codec when the resolutions coincide and the codecs differ (`1080p HEVC`), the audio when resolution and codec coincide and the audio differs (`Latino`, `Castellano`, `Inglés`, `Dual`), and ` (2)` for equal labels. A part that the rule cannot name and that tells the versions apart (a frame with no class, a codec without a short name, an audio track with no known language) is left out and its version is marked `incomplete`: the user completes it in the preview | 5 | `catalog/domain` (`version_label.go`) | `modules/catalog/domain` (`versionLabel.ts`, editable label before confirming, RF-24) | created: the rule above, with `incomplete` per version |

Changing a rule = change the vector first, then make both suites pass.

Vector files are JSON in UTF-8 with LF line endings, formatted by Prettier
(`task fmt`). The rules they pin are defined in [`docs/spec.md`](../docs/spec.md);
this README does not restate them.
