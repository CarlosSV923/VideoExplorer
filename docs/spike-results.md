# Fase 0.5 — Resultados del spike de nombres en Jellyfin

Fecha: 2026-10-10. Jellyfin **12.2** (`jellyfin/jellyfin:12.2.20261005-225228`, la misma versión que el NAS del usuario), en local con Docker y la biblioteca de prueba montada en solo lectura (`scripts/spike/`). Bibliotecas «Películas» y «Series» con TMDB, idioma de metadatos Español (México). Los datos se leyeron de la API de Jellyfin con la sesión del usuario.

## Resumen

| Caso (§5) | Resultado |
|---|---|
| Película simple `Iron Man (2008)` | ✅ TMDB 1726 |
| Título original latino con acento y apóstrofo `Le Fabuleux Destin d'Amélie Poulain (2001)` | ✅ TMDB 194 («Amélie») |
| Título saneado `Mission - Impossible (1996)` | ✅ TMDB 954 |
| Remakes por año `The Lion King (1994)` / `(2019)` | ✅ 8587 / 420818, dos películas distintas |
| Título original no latino guardado en inglés `Spirited Away (2001)` | ❌ **TMDB 535075**, un especial de TV del mismo año (debía ser 129) |
| Ídem con el título original `千と千尋の神隠し (2001)` | ✅ TMDB 129 |
| Ídem con etiqueta `Spirited Away (2001) [tmdbid-129]` | ✅ TMDB 129 |
| Versiones `Dune (2021) - 2160p` / `- 1080p HEVC` | ✅ una película con dos versiones etiquetadas «2160p» y «1080p HEVC» |
| Versiones con etiqueta de ID también en los archivos (`Blade Runner 2049 (2017) [tmdbid-335984] - 2160p`) | ✅ una película, versiones «2160p» y «1080p» |
| Partes `Gone with the Wind (1939) - part1/part2` | ✅ una película con `PartCount` 2; el subtítulo de `part1` queda asociado |
| Especial `Season 00` / `S00E01` | ✅ temporada 0, capítulo 1 |
| Varios capítulos `S01E03-E04` y `S01E01-E02` | ✅ un elemento con capítulos 3–4 y 1–2; Jellyfin muestra los dos títulos de TMDB |
| Versiones de capítulo `S01E02 - … - 2160p` / `- 1080p` | ✅ un capítulo con dos versiones |
| Serie ambigua `The Office (2005) [tmdbid-2316]` / `The Office (2001)` | ✅ 2316 / 2996 |
| Serie de título no latino guardada en inglés `Squid Game (2021)` | ✅ TMDB 93405 |
| Serie saneada `What If... (2021)` | ✅ TMDB 91363 |
| Nombres con `...`, `'`, `+`, `,` en títulos de capítulo | ✅ se reconocen; Jellyfin usa el título de TMDB, no el del archivo |

## Subtítulos

| Sufijo | Jellyfin 12.2 |
|---|---|
| `.es` / `.spa` | ✅ Español |
| `.en.forced` | ✅ Inglés, forzado |
| `.en.sdh` | ✅ Inglés, discapacidad auditiva |
| `.pt-BR`, `.pt-PT`, `.fr-CA`, `.zh-TW`, `.es-419` | ✅ idioma y región reconocidos |
| `.es-MX`, `.es-ES`, `.en-US`, `.en-GB` | ❌ idioma «Sin definir»; muestra el código como título |

Jellyfin solo reconoce las regiones que tiene en su lista de culturas: `es-MX` y `es-ES` no están, `es-419` (Latinoamérica) sí.

## Hallazgos que cambian la spec

1. **Coincidencia por nombre poco fiable con títulos traducidos**: con el título en inglés y el año, Jellyfin eligió otra ficha de TMDB del mismo año. Con el título original o con `[tmdbid-]` acertó. **Decisión (2026-10-10)**: la etiqueta `[tmdbid-]` va activada por defecto en todo título vinculado a TMDB (RF-45).
2. **Regiones de subtítulos**: varias regiones de §5 (`es-MX`, `es-ES`) no las reconoce Jellyfin. **Decisión (2026-10-10)**: solo se escriben regiones que Jellyfin reconoce; `es-MX` y variantes latinoamericanas se guardan como `es-419`, `es-ES` como `es`, `en-US`/`en-GB` como `en` (§5).

3. **Importación de la biblioteca existente**: con la etiqueta activada por defecto, ¿se renombran las carpetas importadas? **Decisión (2026-10-10)**: no; lo importado conserva su nombre y la etiqueta se añade desde la ficha (RF-29, RF-45).
4. **Regla 6 de saneamiento**: los puntos finales se recortan por componente de la ruta, no por título (§5), como ya implicaba el ejemplo `What If...?` → `What If...`.

## Otras observaciones

- Algunos capítulos sin traducción al español en TMDB aparecen como «Episodio 1» (`S00E01` de Breaking Bad, `S01E01` de The Office UK): es la metadata de Jellyfin, no el nombre del archivo, que se reconoció bien.
- Los puntos dentro de un título (`What If... Captain Carter…`, `Cat's in the Bag... - 2160p`) no causan problemas.
- Nombres con acentos, apóstrofos y caracteres japoneses se crean bien en NTFS y Jellyfin los lee sin problema.
