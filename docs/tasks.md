# VideoExplorer — Tareas

> Estados: `[ ]` pendiente · `[~]` en curso · `[x]` hecha. No se empieza una fase si su dependencia no está hecha.
> Requisitos: [`spec.md`](./spec.md) (los IDs RF/RNF se citan en cada fase). Los puntos abiertos (§9) se resuelven antes de la fase que los necesita.

## Fase 0 — Base del repositorio
Depende de: — · Base: GameExplorer
- [x] Repositorio git y remoto público `CarlosSV923/VideoExplorer` (confirmar con el usuario antes del primer push)
- [x] Monorepo copiado de la estructura de GameExplorer: Go (`apps/api`) + pnpm (`apps/web`), `Taskfile.yml`, `.editorconfig`, `.gitattributes`, `.gitignore`
- [x] API mínima (`/api/health`, apagado ordenado) y web mínima (React + Vite + TS estricto, Vitest, ESLint, Prettier, `VITE_DATA_SOURCE`)
- [x] `golangci-lint` v2, `task check`, CI en GitHub Actions
- [x] Esqueleto de `api/openapi.yaml` (con `/health` y su esquema, que hoy está escrito a mano) validado en `task check` y CI, y `contracts/` (README con los vectores previstos)
- [ ] README EN/ES, LICENSE (MIT), `.env.example`

## Fase 0.5 — Spike de nombres en Jellyfin y Plex
Depende de: 0 · Cubre: §5, RF-45, §9.1–9.3
- [ ] Biblioteca de prueba con archivos ficticios: película simple, con versiones, con partes; serie con especiales, varios capítulos por archivo, versiones y subtítulos (`.es.srt`, `.es-MX.srt`, `.en.forced.srt`, `.sdh`)
- [ ] Probar cada caso en Jellyfin y Plex reales, sin etiquetas de ID, con `{tmdb-}`, con `[tmdbid-]` y con ambas
- [ ] Probar títulos saneados (`Mission - Impossible`) y títulos ambiguos o remakes
- [ ] Registrar los resultados en `docs/spike-results.md` y ajustar §5

## Fase 1 — API base
Depende de: 0.5 · Cubre: RF-60, RNF-02, RNF-03
- [ ] Configuración (`caarlos0/env`, reemplaza `internal/platform/config`), `slog`, SQLite (modernc, WAL) + goose + sqlc; en CI, reactivar la caché de Go (ya habrá `go.sum`) y fijar la versión de `govulncheck`
- [ ] Contexto `identity` reutilizado de GameExplorer
- [ ] oapi-codegen + cliente TS (`task gen`, el handler de `/api/health` pasa a la interfaz generada, comprobación en CI de código generado al día; reactivar `security-defined` y `operation-4xx-response` en `api/redocly.yaml` con la sesión); SPA incrustada
- [ ] `/api/health` con diagnósticos (escritura en biblioteca y datos, `7zz`, `ffprobe`, TMDB). Antes, definir en la spec los estados de salud (¿`degraded` como en GameExplorer?) y qué ve el usuario; `api/openapi.yaml` ya lo anticipa en su descripción
- [ ] Imagen de desarrollo con `7zz` y `ffprobe` estático (versiones fijadas, SHA-256), tarea `dev:image` e instalación de ambos en CI

## Fase 2 — Contexto `metadata` (TMDB)
Depende de: 1 · Cubre: RF-21, RF-61, §6, §9.5
- [ ] Cliente TMDB (v3 key o v4 token, límites, reintentos): búsqueda de películas y series, detalle, temporadas, capítulos, colecciones, géneros, logos
- [ ] Regla de título de disco (original latino o inglés) y de títulos de capítulo (§5): crea `contracts/naming-cases.json` con esas secciones (la fase 5 añade carpetas, archivos y saneamiento) y actualiza `contracts/README.md`
- [ ] Caché de respuestas en SQLite y de imágenes en `/data/cache/images`; `GET /api/images/...`
- [ ] Catálogo fijo para la demo, sin imágenes de TMDB (RF-71)

## Fase 3 — Contexto `ingestion` I: subidas
Depende de: 1 · Cubre: RF-01 a RF-03, RF-09, RF-10, §9.6
- [ ] tusd en `/api/uploads/` (staging en `.videoexplorer/staging`), espacio libre, sesión
- [ ] Agregado `Batch`/`UploadJob` con relación (RF-03) y máquina de estados persistida
- [ ] `/api/jobs` y SSE de progreso; purga de 24 h

## Fase 4 — Contexto `ingestion` II: extracción, filtrado y análisis
Depende de: 3 · Cubre: RF-04 a RF-08, RF-20, RF-23
- [ ] Extractor 7zz (de GameExplorer): multiparte, contraseña, CRC32, zip-slip; comprimido original a la papelera
- [ ] Filtrado por extensión y detección de *samples*; descartes a la papelera
- [ ] Puerto `Prober` + adaptador ffprobe (resolución, códecs, pistas, duración) con caché
- [ ] Analizador de nombres de archivo (película/serie, título, año, `SxxEyy`, `1x02`, varios capítulos, partes, idioma de subtítulos) + vectores `filename-cases.json`
- [ ] Detección de idioma de subtítulos *offline*
- [ ] Hash parcial para «Copia exacta»
- [ ] Resultado a `_unassigned/<lote>/` (RF-08)

## Fase 5 — Contexto `catalog`: asignar
Depende de: 2, 4 · Cubre: RF-21 a RF-26, §5
- [ ] Nombres y saneamiento (§5) + vectores `naming-cases.json`
- [ ] Motor de operaciones con journal (de GameExplorer) y papelera mínima
- [ ] Duplicados (Reemplazar / Conservar ambos con etiqueta de versión / Omitir) y «Copia exacta»
- [ ] `plan` (vista previa) y `commit` de una entrada de No asignados

## Fase 6 — No asignados, escaneo e importación
Depende de: 5 · Cubre: RF-27 a RF-29, §9.4
- [ ] Lectura en vivo de `_unassigned/` al abrir la sección, estado «Copiando…»
- [ ] Comprimidos en `_unassigned/` procesados como subidas (RF-27a)
- [ ] Escaneo periódico (dos intervalos) y «Escanear ahora»
- [ ] Importación de `Movies/` y `TV Shows/` existentes; lo que no sigue el formato va a No asignados

## Fase 7 — Edición, papelera, etiquetas de ID y descargas
Depende de: 6 · Cubre: RF-30, RF-31, RF-43 a RF-45
- [ ] Editar/reasignar con vista previa y renombrado reversible
- [ ] Etiquetas de ID por título (RF-45)
- [ ] Papelera completa: listar, restaurar a su origen con conflicto, borrar, vaciar, purga a 30 días
- [ ] Descargas: archivo con Range y zip *store* (de GameExplorer)
- [ ] Info técnica (ffprobe)

## Fase 8 — Integración Jellyfin / Plex
Depende de: 7 · Cubre: RF-50
- [ ] Puerto `MediaServerNotifier` + adaptadores Jellyfin y Plex (refresco por carpeta, agrupado en 30 s)
- [ ] Estado en Ajustes y en `/api/health`

## Fase 9 — Diseño → código
Depende de: 0 · Cubre: RF-40 a RF-43, RNF-05 a RNF-07, §9.7, §9.8
- [ ] **Mockups primero** (estética Netflix / Prime Video): Inicio con hero y filas, Películas, Series, ficha de película, ficha de serie con temporadas, colección, No asignados, Asignar (con duplicados), Subidas, Papelera, Ajustes, login, estados vacíos y de error, móvil y escritorio
- [ ] `design:design-critique` y `design:accessibility-review` → `docs/design-review.md`
- [ ] `design:design-handoff` → `docs/design-handoff.md`
- [ ] Tokens, `shared/ui`, i18n ES/EN y capa de entrada (de GameExplorer, adaptadas)

## Fase 10 — Frontend con adaptadores HTTP
Depende de: 8, 9 · Cubre: RF-01 a RF-62 (UI)
- [ ] Puertos y adaptadores `http/` por módulo; router y TanStack Query
- [ ] Pantallas completas; subidas con `tus-js-client` y pregunta de relación
- [ ] Prueba real contra la API con archivos reales

## Fase 11 — Demo
Depende de: 10 · Cubre: RF-70 a RF-72
- [ ] Adaptadores `demo/`, biblioteca sembrada, archivos de ejemplo, placeholders generados
- [ ] Despliegue en Vercel; Playwright E2E sobre la demo; en CI, build en modo demo y job E2E

## Fase 12 — Producción en TrueNAS
Depende de: 10 · Cubre: RNF-01, RNF-03, RNF-04
- [ ] Dockerfile de producción (node → golang → debian-slim con `7zz` y `ffprobe`), publicación en GHCR con tags `v*`
- [ ] `deploy/truenas/compose.yaml` y guía en README
- [ ] Prueba en el NAS con contenido real: subida, extracción, asignación, importación de la biblioteca existente, Jellyfin y Plex reconociéndolo todo
