# Golden vectors

Business rules that exist in **both** the Go backend (authoritative) and the
TypeScript frontend (live previews and demo mode) are pinned here as data.
Both test suites load these files and must produce exactly the expected output,
so the two implementations cannot drift apart (spec RNF-08).

No vector file exists yet. Each one is created in the phase that implements
its rule, after the Jellyfin and Plex spike (phase 0.5) has confirmed the
naming rules they depend on. The table lists the planned files.

| File (planned) | Rule | Phase | Go consumer | TS consumer | Status |
|---|---|---|---|---|---|
| `naming-cases.json` | Folder and file naming for each kind (spec §5), sanitizing (§5, steps 1 to 6), rejected inputs (`expected.error` names the field) and a file's extension: the longest known one (`extensionCases`) | 5 | `catalog/domain` | `modules/catalog/domain` (preview of the final names in Assign, RF-25) | pending |
| `filename-cases.json` | Filename analysis: movie or series, title, year, `SxxEyy`, `1x02`, several chapters per file, parts, subtitle language (RF-20, RF-22, RF-23) | 4 | `ingestion/domain` | `modules/ingestion/domain` (suggestions in the demo, RF-72) | pending |
| `version-labels.json` | Version labels read from resolution and codec (`2160p`, `1080p HEVC`) and their sanitizing (RF-24, §5 `<versión>`) | 5 | `catalog/domain` | `modules/catalog/domain` (editable label before confirming, RF-24) | pending |

Changing a rule = change the vector first, then make both suites pass.

Vector files are JSON in UTF-8 with LF line endings, formatted by Prettier
(`task fmt`). The rules they pin are defined in [`docs/spec.md`](../docs/spec.md);
this README does not restate them.
