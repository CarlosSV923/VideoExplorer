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
- [x] README EN/ES, LICENSE (MIT), `.env.example`
- [x] Quitar Plex de `README.md`, `README.es.md`, `.env.example` y `contracts/README.md`, y decir que los nombres y la etiqueta de ID siguen la convención de Jellyfin (decisión 2026-10-09, spec §1, §5, §8)

## Fase 0.5 — Spike de nombres en Jellyfin
Depende de: 0 · Cubre: §5, RF-45
- [x] Biblioteca de prueba con archivos ficticios: película simple, con versiones, con partes; serie con especiales, varios capítulos por archivo, versiones y subtítulos (`.es.srt`, `.es-MX.srt`, `.en.forced.srt`, `.sdh`)
- [x] Jellyfin local en Docker con la misma versión que el NAS del usuario (12.2, imagen `jellyfin/jellyfin:12.2.20261005-225228`; biblioteca montada en solo lectura; decisión 2026-10-10) y `CASES.md` adaptado a ese flujo
- [x] Probar cada caso en ese Jellyfin (incluida la etiqueta `[tmdbid-]` en archivos de versiones de película)
- [x] Probar títulos saneados (`Mission - Impossible`) y títulos ambiguos o remakes
- [x] Registrar los resultados en `docs/spike-results.md` y ajustar §5 (etiqueta de ID por defecto, regiones de subtítulos, importación sin renombrar, regla 6 de saneamiento)

## Fase 1 — API base
Depende de: 0.5 · Cubre: RF-60, RF-63, RNF-02, RNF-03, §7
- [x] Configuración (`caarlos0/env`, reemplaza `internal/platform/config`, defaults de §7 reflejados en `.env.example`, sin `PUID`/`PGID`), `slog`, SQLite (modernc, WAL) + goose + sqlc; `task dev:api` recibe `.env.local` (p. ej. `compose.dev.yaml` como GameExplorer); en CI, reactivar la caché de Go (ya habrá `go.sum`) y fijar la versión de `govulncheck`
- [x] oapi-codegen + cliente TS (`task gen`, el handler de `/api/health` pasa a la interfaz generada, comprobación en CI de código generado al día); SPA incrustada
- [x] Contexto `identity` reutilizado de GameExplorer (endpoints `/auth/*` en `api/openapi.yaml` sobre la interfaz generada; reactivar `security-defined` y `operation-4xx-response` en `api/redocly.yaml` con la sesión; middleware de sesión y cabeceras de seguridad de RNF-02; la página de «binario sin SPA» de `platform/web` pasa a inglés)
- [x] Imagen de desarrollo con `7zz` y `ffprobe` estático (versiones fijadas, SHA-256), tarea `dev:image` e instalación de ambos en CI; `dev:api`, `test:api` y `hash-password` la usan; README con estado actualizado
- [x] `/api/health` con diagnósticos según RF-63 (escritura en biblioteca y datos, `7zz`, `ffprobe`; `code` + datos + `message` en el contrato; TMDB fuera del estado)
- [x] Arranque: comprobar `DATA_PATH` antes de abrir la base y parar con error claro (RNF-03); en desarrollo, crear `.dev/library` y `.dev/data` si faltan; el hash argon2id va entre comillas simples en `.env.local` para que Compose no interpole sus `$`

## Fase 2 — Contexto `metadata` (TMDB)
Depende de: 1 · Cubre: RF-21, RF-61, §6, §9.5
- [x] Cliente TMDB (v3 key o v4 token, límites, reintentos): búsqueda de películas y series, detalle, temporadas, capítulos, colecciones, géneros, logos
- [x] Regla de título de disco (original latino o inglés) y de títulos de capítulo (§5): crea `contracts/naming-cases.json` con esas secciones (la fase 5 añade carpetas, archivos y saneamiento) y actualiza `contracts/README.md`
- [x] Caché de respuestas en SQLite y de imágenes en `/data/cache/images`; `GET /api/images/...`
- [x] Catálogo fijo para la demo (RF-70, RF-71): generador `cmd/demo-catalog` como GameExplorer que toma de TMDB solo la estructura (títulos, años, géneros, colecciones, temporadas, capítulos) y sinopsis propias en un archivo aparte; necesita la clave de TMDB
- [x] Logo oficial de atribución de TMDB en el repo (RNF-10; descargarlo requiere confirmación del usuario), para la UI de las fases 9 y 10

## Fase 3 — Contexto `ingestion` I: subidas
Depende de: 1 · Cubre: RF-01 a RF-03, RF-09, RF-10, §9.6
- [x] tusd en `/api/uploads/` (staging en `.videoexplorer/staging`), espacio libre, sesión
- [x] Agregado `Batch`/`UploadJob` con relación (RF-03) y máquina de estados persistida; sustituye el consumidor provisional `logCompleted`; reserva de espacio de las subidas en curso y rechazo de 0 bytes (RF-09a)
- [x] `/api/jobs` y SSE de progreso; purga de 24 h (subidas abandonadas y staging huérfano liberan su trabajo y su reserva de espacio; trabajos `uploading` sin archivo y subidas completas cuyo trabajo quedó en `uploading` se reconcilian al arrancar y al purgar)

## Fase 4 — Contexto `ingestion` II: extracción, filtrado y análisis
Depende de: 3 · Cubre: RF-04 a RF-08, RF-20, RF-23
- [x] Extractor 7zz (de GameExplorer): multiparte (`waiting_parts`), contraseña, espacio libre, CRC32, zip-slip, fallos y Reintentar de RF-10c; comprimido original a la papelera (adelanta la papelera mínima del contexto `catalog`, con el formato de GameExplorer, tras un puerto)
- [x] Filtrado por extensión y detección de *samples*; descartes a la papelera (parte de `filtering`, contenido en `staging/extract/<id>/`; previo: el `.rar` de un conjunto `.r00` en el mismo lote también da `legacy_volume`)
- [x] Puerto `Prober` + adaptador ffprobe (resolución, códecs, pistas, duración) con caché
- [x] Analizador de nombres de archivo (película/serie, título, año, `SxxEyy`, `1x02`, varios capítulos, partes, idioma de subtítulos) + vectores `filename-cases.json`
- [x] ~~Detección de idioma de subtítulos *offline*~~ descartada (decisión 2026-10-10, RF-23): el idioma sale solo del nombre o lo elige el usuario
- [x] Hash parcial para «Copia exacta»
- [x] Resultado a `_unassigned/<lote>/` (RF-08, RF-08a): una entrada por lote o por trabajo según la relación, nombre saneado (añade la sección de saneamiento a `contracts/naming-cases.json`), metadatos en SQLite, `done`, reencolar sondeo y hash, `filter_failed`/`move_failed` con Reintentar

## Fase 5 — Contexto `catalog`: asignar
Depende de: 2, 4 · Cubre: RF-21 a RF-26, §5
- [x] Nombres (§5; el saneamiento ya está en `naming-cases.json` desde la Fase 4), incluida la etiqueta de ID por defecto y la normalización de regiones de subtítulos): añade carpetas, archivos y saneamiento al mismo `contracts/naming-cases.json` (consumidor `catalog/domain`); el capítulo necesita sus títulos en el idioma original y en inglés (hoy `Episode` solo trae el localizado)
- [x] Motor de operaciones con journal (de GameExplorer); la papelera mínima ya existe desde la Fase 4; el paso a No asignados (`catalog.Unassigned.Put`) y la papelera pasan por el motor
- [ ] Robustez de la ingesta: asentar los lotes en segundo plano tras arrancar (hoy un lote de 3000 archivos retrasa el arranque ~2 min 40 s); nunca borrar un comprimido que siga en staging porque falló su traslado a la papelera (el trabajo falla con motivo y Reintentar)
- [ ] Duplicados (Reemplazar / Conservar ambos con etiqueta de versión / Omitir) y «Copia exacta»; etiqueta automática con resolución, códec e idioma del audio (RF-24) + vectores `contracts/version-labels.json`; el hash bajo demanda de archivos de la biblioteca rechaza enlaces simbólicos (RNF-02)
- [ ] `plan` (vista previa) y `commit` de una entrada de No asignados

## Fase 6 — No asignados, escaneo e importación
Depende de: 5 · Cubre: RF-27 a RF-29, §9.4
- [ ] Lectura en vivo de `_unassigned/` al abrir la sección, estado «Copiando…»
- [ ] Comprimidos en `_unassigned/` procesados como subidas (RF-27a)
- [ ] Escaneo periódico (dos intervalos) y «Escanear ahora»
- [ ] Importación de `Movies/` y `TV Shows/` existentes; lo que no sigue el formato va a No asignados (el análisis de nombres debe extraer `[tmdbid-N]` de la carpeta, RF-29)

## Fase 7 — Edición, papelera, etiquetas de ID y descargas
Depende de: 6 · Cubre: RF-30, RF-31, RF-43 a RF-45
- [ ] Editar/reasignar con vista previa y renombrado reversible
- [ ] Etiqueta de ID `[tmdbid-]`: activa por defecto al asignar o vincular, quitar/poner desde la ficha (RF-45)
- [ ] Papelera completa: listar, restaurar a su origen con conflicto, borrar, vaciar, purga a 30 días
- [ ] Descargas: archivo con Range y zip *store* (de GameExplorer)
- [ ] Info técnica (ffprobe)
- [ ] «Actualizar datos» de TMDB desde la ficha (RF-44, §6)

## Fase 8 — Integración con Jellyfin
Depende de: 7 · Cubre: RF-50
- [ ] Puerto `MediaServerNotifier` + adaptador Jellyfin (refresco por carpeta, agrupado en 30 s); el puerto deja abierta la puerta a otros servidores en el futuro
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
- [ ] Pantallas completas; subidas con `tus-js-client` y pregunta de relación (envía `partsTotal` por conjunto multiparte, RF-03); decidir cómo se cancela desde la UI un trabajo ya extraído (tus DELETE ya no encuentra el comprimido)
- [ ] Prueba real contra la API con archivos reales
- [ ] README EN/ES: estado del proyecto al día (fases cerradas)

## Fase 11 — Demo
Depende de: 10 · Cubre: RF-70 a RF-72
- [ ] Adaptadores `demo/`, biblioteca sembrada (lee `apps/web/src/modules/metadata/infrastructure/demo/catalog.json`; decide qué títulos están en la biblioteca y qué capítulos tiene cada serie), archivos de ejemplo, placeholders generados, atribución de TMDB en Ajustes
- [ ] Despliegue en Vercel; Playwright E2E sobre la demo; en CI, build en modo demo y job E2E

## Fase 12 — Producción en TrueNAS
Depende de: 10 · Cubre: RNF-01, RNF-03, RNF-04
- [ ] Dockerfile de producción (node → golang → debian-slim con `7zz` y `ffprobe` vía los scripts de `scripts/`, aviso de licencia GPL de ffprobe con enlace a su código fuente), publicación en GHCR con tags `v*`
- [ ] `deploy/truenas/compose.yaml` y guía en README (si el compose interpola variables, el hash argon2id necesita comillas o `$$`)
- [ ] Prueba en el NAS con contenido real: subida, extracción, asignación, importación de la biblioteca existente y Jellyfin reconociéndolo todo
