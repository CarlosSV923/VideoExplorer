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
- [x] Robustez de la ingesta: asentar los lotes en segundo plano tras arrancar (hoy un lote de 3000 archivos retrasa el arranque ~2 min 40 s); nunca borrar un comprimido que siga en staging porque falló su traslado a la papelera (el trabajo falla con motivo y Reintentar)
- [x] Rendimiento del asentamiento: coalescer las peticiones de `settleAll` (un solo pase pendiente) para evitar el coste O(n²) con lotes grandes (hoy filtrar y asentar 3000 archivos tarda ~26 min) y que un DELETE de subida no espere minutos al cerrojo
- [x] Duplicados (Reemplazar / Conservar ambos con etiqueta de versión / Omitir) y «Copia exacta» (registro de la biblioteca, detección, copia exacta, enlaces, etiqueta de versión, planes por el motor); etiqueta automática con resolución, códec e idioma del audio (RF-24) + vectores `contracts/version-labels.json`; el hash bajo demanda de archivos de la biblioteca rechaza enlaces simbólicos (RNF-02)
- [x] API de Asignar sobre una entrada de No asignados: bloqueo «Asignándose» (RF-26), sugerencias por video (RF-20: análisis del nombre + mejor coincidencia de TMDB), búsqueda de títulos con los de la biblioteca primero (RF-21), `plan` (vista previa con rutas finales, duplicados, copia exacta y decisiones, RF-24/RF-25) y `commit` por el motor (crea o amplía el título en el registro, RF-25); etiquetas escritas por el usuario para versiones existentes sin etiqueta legible (RF-24)

## Fase 6 — No asignados, escaneo e importación
Depende de: 5 · Cubre: RF-27 a RF-29, §9.4
- [x] Lectura en vivo de `_unassigned/` al abrir la sección (`GET` de la sección con entradas de subida y de Samba, «Copiando…», mandar a la papelera y borrar definitivamente por entrada y por archivo), estado «Copiando…»; archivos basura del sistema (`Thumbs.db`, `desktop.ini`, `.DS_Store`) no impiden vaciar una entrada al asignar
- [x] Comprimidos en `_unassigned/` procesados como subidas (RF-27a): detección al leer la sección, traslado a staging, trabajo de un lote `samba`, conjuntos multiparte por carpeta, progreso en la sección
- [x] Escaneo periódico (dos intervalos) y «Escanear ahora»; el escaneo de `_unassigned/` llama a `ingestion.Service.AdoptSamba` (RF-27a); valorar cachear la detección de comprimidos por (ruta, tamaño, mtime)
- [x] Importación de `Movies/` y `TV Shows/` existentes (RF-29, criterio «lo que Jellyfin reconoce»): registro en su sitio sin renombrar, etiquetas `[tmdbid-]`, `{tmdb-}` e `[imdbid-]`, «Sin vincular», solo lo no interpretable a No asignados, bajas de lo borrado; pruebas con enlaces duros de `~/Downloads/PruebasVideos` (nunca modificar el original)
- [x] Zips creados en Windows con la barra invertida como separador (p. ej. `Compress-Archive` de PowerShell 5.1) se extraen con su estructura en vez de fallar como `archive_corrupt`; podar `archive_probes` de copias que ya no están en `_unassigned/`; en el análisis de nombres, si el título del archivo coincide con el de la carpeta (normalizado), sugerir el de la carpeta tal cual (con su « - »)

## Fase 7 — Edición, papelera, etiquetas de ID y descargas
Depende de: 6 · Cubre: RF-30, RF-31, RF-43 a RF-45
- [x] Editar/reasignar con vista previa y renombrado reversible (RF-44): plan/commit por el motor, cambiar coincidencia de TMDB o título libre (incluido Vincular), tipo, temporada/capítulo, etiqueta de versión, parte e idioma de subtítulo; Mover a No asignados y Mandar a la papelera desde la ficha; endpoints de lectura de títulos y archivos que necesite la ficha
- [x] Etiqueta de ID `[tmdbid-]`: activa por defecto al asignar o vincular, quitar/poner desde la ficha (RF-45); previo: arreglar el test intermitente `TestProbeGivesUpAfterItsTimeout` (`ingestion/infrastructure/ffprobe`, «text file busy» al ejecutar el ffprobe falso escrito por el test)
- [x] Vista previa de edición y de etiqueta: listar los duplicados detectados con su propuesta antes de que el usuario decida (como Asignar); hoy solo aparecen tras responder
- [x] Papelera completa: listar, restaurar a su origen con conflicto, borrar, vaciar, purga a 30 días; revisar `library_files.video_id ON DELETE CASCADE` (hoy borrar un video borra la fila de sus subtítulos aunque estén en la papelera); la importación debe ignorar los títulos que están solo en la papelera (hoy `Library.Titles` los devuelve)
- [x] Descargas: archivo con Range y zip *store* (de GameExplorer), en la ficha y en No asignados (RF-44, RF-27)
- [x] Info técnica (ffprobe) de un archivo de la biblioteca (RF-44)
- [x] Ficha: lista de capítulos presentes y faltantes por temporada según TMDB, con miniatura, título y sinopsis (RF-43), y datos de TMDB de la ficha (backdrop, géneros, sinopsis, colección)
- [x] «Actualizar datos» de TMDB desde la ficha (RF-44, §6); previo: localizar y arreglar un fallo intermitente visto una vez en la batería completa de Go (probablemente `internal/metadata/infrastructure/sqlite`; no se reprodujo en 4 pasadas)

## Fase 8 — Integración con Jellyfin
Depende de: 7 · Cubre: RF-50
- [x] Puerto `MediaServerNotifier` + adaptador Jellyfin (con la detección de rutas de RF-50a) (refresco por carpeta, agrupado en 30 s); el puerto deja abierta la puerta a otros servidores en el futuro
- [x] Estado en Ajustes y en `/api/health` (RF-50a, RF-62); previo: la autenticación con Jellyfin 12.2 debe usar `Authorization: MediaBrowser Token="…"` (rechaza `X-Emby-Token`)

## Fase 9 — Diseño → código
Depende de: 0 · Cubre: RF-40 a RF-43, RNF-05 a RNF-07, §9.7, §9.8
- [x] **Mockups primero** (estética Netflix / Prime Video; lienzo [`/design`](https://claude.ai/artifact/A2ENfFRCjVUMMcjMMiZCqb), aprobado por el usuario el 2026-10-16): Inicio con hero y filas, Películas, Series, ficha de película, ficha de serie con temporadas, colección, No asignados, Asignar (con duplicados), Subidas, Papelera, Ajustes, login, estados vacíos y de error, móvil y escritorio
- [x] `design:design-critique` y `design:accessibility-review` → [`design-review.md`](./design-review.md) (7 hallazgos de crítica y 12 de WCAG con su corrección)
- [x] Lienzo corregido: 22 pantallas; nuevas Series (filtro «Solo incompletas»), Editar y reasignar (coincidencia, tipo, etiqueta de ID, versiones, subtítulos y vista previa) y Asignar una película en partes; modo demo y barra de gamepad en Estados; contraste de la colección, nombres accesibles de los pósters, `h1` de Inicio, atajos de una letra solo con foco en un video, acciones secundarias de No asignados en «Más acciones»
- [x] `design:design-handoff` → [`design-handoff.md`](./design-handoff.md): tokens, componentes, estados, movimiento, entrada (atajos de Asignar), tamaños y rutas, casos límite, i18n y accesibilidad
- [x] Tokens, `shared/ui`, i18n ES/EN y capa de entrada (de GameExplorer, adaptadas): tokens como `@theme` de Tailwind v4, fuentes con `@fontsource`, primitivas de `design-handoff.md` §2 (incluida `Brand`), i18n con `Intl`, capa de entrada con gamepad, navegación espacial y atajos de una letra solo con foco; página de componentes (`app/UiKit.tsx`) revisada en el navegador a 1440 y 375 px (sin desbordes; foco del diálogo destructivo en Cancelar)

## Fase 10 — Frontend con adaptadores HTTP
Depende de: 8, 9 · Cubre: RF-01 a RF-62 (UI)
- [x] Puertos y adaptadores `http/` por módulo; router y TanStack Query: puertos y adaptadores de todo el contrato (descargas e imágenes como URL, SSE de trabajos), `AppError` por tipo sin mostrar el `detail`, composición en `src/app`, rutas de `design-handoff.md` §5 con marcadores, login mínimo y vuelta tras sesión vencida
- [x] Pantallas completas:
  - [x] Biblioteca: cabecera y navegación (con barra inferior en móvil), Inicio (hero, pendientes, filas, colecciones), Películas y Series, búsqueda, fichas de película y serie (versiones, partes, subtítulos, info técnica, descargas, temporadas y capítulos), colección, Editar y reasignar con etiqueta de ID y Vincular, Actualizar datos, mover a No asignados y papelera desde la ficha. Contrato ampliado según RF-42a (resumen con datos guardados, sin llamar a TMDB; `GET /library/collections/{id}`); una pantalla por archivo y cabecera común en `app/LibraryLayout.tsx`
  - [x] No asignados (estados, «Más acciones», descargas, papelera, borrar con confirmación; explicar la entrada que solo guarda contenido en carpetas ocultas o del sistema, RF-27), Asignar (capítulos y película, duplicados, subtítulos, vista previa, bloqueo y atajos) y elegir título
  - [x] Subidas con `tus-js-client` (zona de soltar, pregunta de relación, `partsTotal` por conjunto multiparte, RF-03; panel y hoja móvil; cancelar hasta descomprimir, RF-10a), Papelera, Ajustes (idioma, escaneo, Jellyfin, TMDB, sesión), banner de salud y botón de cerrar de `Dialog` con «Cerrar»; en Asignar faltan en la barra de atajos Enter (aceptar sugerencia), Ctrl+Enter (Guardar) y Esc (Cancelar) con los grupos «Con el foco en un video» / «Siempre», y la barra no debe verse junto a la de gamepad
- [x] Prueba real contra la API con archivos reales (2026-10-16, biblioteca de `.dev` con enlaces duros a PruebasVideos y Jellyfin desactivado): importación de Alien Romulus (`[imdbid-]`) y Lizzie Borden, cuatro rar de Samba con contraseña, asignación de capítulos de Dahmer a una serie nueva y a una existente con subtítulo `es-419`, Editar sin cambios conserva los nombres importados, papelera y Ajustes. Cuatro rondas de correcciones (ruta de Asignar, sondeo excesivo, bloqueo al navegar, sugerencias de TMDB en la biblioteca, Editar completa, nombres importados, accesibilidad y textos). Después: subida con tus probada por el usuario en su navegador (encontró el panel de Subidas duplicado y la falta de un botón para ver el progreso: decisión RF-10, botón «Subidas» siempre visible), restaurar con lugar ocupado y «Conservar ambos» (`- 720p` y `- 720p (2)`), etiqueta editable al restaurar; siete rondas de corrección en total
- [x] README EN/ES: estado del proyecto al día (fases cerradas)

Notas para la Fase 12 (resueltas el 2026-10-20 antes de `v0.1.0`, publicada ese día: `ghcr.io/carlossv923/videoexplorer:0.1.0` y `:latest`, descarga anónima comprobada; resuelto el 2026-10-21: las acciones de Docker del workflow de publicación se actualizaron a sus versiones con Node.js 24): estado «no encontrado» propio para recursos que no existen; carga diferida por pantalla (bloque inicial de 700 kB a 320 kB, sin aviso de 500 kB); transportes HTTP salientes propios en Jellyfin y TMDB (corrige un test intermitente).

## Fase 11 — Demo
Depende de: 10 · Cubre: RF-70 a RF-72
- [x] Adaptadores `demo/`, biblioteca sembrada (lee `apps/web/src/modules/metadata/infrastructure/demo/catalog.json`; decide qué títulos están en la biblioteca y qué capítulos tiene cada serie), archivos de ejemplo, placeholders generados, atribución de TMDB en Ajustes: adaptadores en memoria para todos los puertos, semilla según RF-71a, subidas simuladas con la máquina de estados del backend (multiparte junto, contraseña `demo` una vez por conjunto, comprimidos propios sin contraseña), ejemplos, banner; probado en el navegador. Después (decisión del usuario, RF-71): imágenes reales cargadas desde `image.tmdb.org` con el catálogo regenerado (solo rutas) y placeholder si fallan; pósters de ancho fijo en filas y cuadrículas
- [x] Despliegue en Vercel; Playwright E2E sobre la demo; en CI, build en modo demo y job E2E (repo listo: `apps/web/vercel.json` con reescritura de la SPA, Playwright en escritorio y móvil táctil con 12 pruebas, job E2E y build demo en CI; corregidos Asignar en móvil y el refresco de No asignados. desplegada por el usuario el 2026-10-20 en [video-explorer-sepia.vercel.app](https://video-explorer-sepia.vercel.app), comprobada con recarga directa de rutas profundas e imágenes de TMDB)

## Fase 12 — Producción en TrueNAS
Depende de: 10 · Cubre: RNF-01, RNF-03, RNF-04
- [x] Dockerfile de producción (node → golang → debian-slim con `7zz` y `ffprobe` vía los scripts de `scripts/`, aviso de licencia GPL de ffprobe con enlace a su código fuente), publicación en GHCR con tags `v*`: imagen de 84 MiB (debian 13 slim) con SHA-256 de `7zz` y `ffprobe` verificados, aviso GPL y licencia en `/usr/share/doc/videoexplorer/ffprobe/`, `HEALTHCHECK`, volúmenes `/library` y `/data`; workflow `release.yml` (PR solo construye, `v*` publica); probada en local
- [x] `deploy/truenas/compose.yaml` y guía en README (si el compose interpola variables, el hash argon2id necesita comillas o `$$`): compose fijado a `0.1.0` con `user:`, datasets `/library` y `/data`, variables de §7 (hash con `$$`), guía en `deploy/truenas/README.md` y en los README; `task compose:check` en CI; probado en local con la imagen construida (salud y login)
- [x] Inicio simplificado (RF-40, decisión 2026-10-20): hero, Pendientes y dos filas automáticas con las 10 últimas películas y las 10 últimas series, sin barra de desplazamiento; sin filas por género ni de colecciones; también en la demo; probado en la demo
- [x] Hero al azar (RF-40, decisión 2026-10-20): 5 títulos al azar por visita entre las 10 películas y 10 series recientes con backdrop; probado en la demo
- [x] Correcciones de la primera instalación en el NAS: ficha que falla con `genres: null` en títulos libres, etiqueta `[tmdbid-]` solo en el archivo no reconocida al importar, detección automática (RF-28a) y `_unassigned/` al arrancar; mensajes de configuración más claros. Hecho: arrays sin `null`, etiquetas de ID en el archivo (la película real quedó vinculada a TMDB 273477), intervalos 6 h / 30 min, mensajes que nombran la variable, vigilante inotify con estabilidad de 10 s y una pasada por carpeta, aviso a la web por SSE y `_unassigned/` al arrancar; probado con la imagen de producción sobre volúmenes Linux. Antes de publicar: Go 1.27.3 (GO-2026-6809, imagen de desarrollo `videoexplorer-dev:2`) y vigilante sin huecos al crear carpetas (100/100). Publicada `0.1.3` el 2026-10-20
- [x] Correcciones de la segunda ronda en el NAS: Vincular (el propio título aparecía como resultado y se elegía sin efecto), títulos libres con etiqueta vinculados en el escaneo, menús «Más acciones» lejos de su botón, sugerencia por `[imdbid-]` en Asignar, vista previa de Vincular en la demo. Hecho; el título libre de los zombis se vinculó solo a TMDB 273477 en el escaneo, sin renombrar; menús anclados comprobados en la demo
- [x] Lentitud en el NAS: la ficha (y otras pantallas) se queda varios segundos en el esqueleto de carga mientras se suben películas nuevas. Hecho: la ficha responde con lo guardado (3–7 ms aun con una extracción en curso) y TMDB se completa en segundo plano (0,6–1,7 s en frío, temporadas en paralelo); imágenes bajo demanda; invalidaciones SSE agrupadas en 500 ms y solo de lo afectado; la ficha no vuelve al esqueleto al recargar
- [x] Navegación sin recargas: todos los enlaces internos (`<a href>`) recargan la app entera; con subidas en curso eso las interrumpe y dispara el aviso de salir (RF-02), y la página se queda en el esqueleto o bloqueada. Hecho: `AppLink` sobre el router en todos los enlaces internos, regla de ESLint contra `<a href="/…">`, E2E que navega sin recargar con una subida en curso; comprobado en la demo (la página no se recarga)
- [x] Buscar por id de TMDB o IMDb en el selector de títulos (RF-21, decisión 2026-10-21): probado con la API real (`1396` → película y serie, enlace `/tv/1396` → serie, `tt0118583` → Alien: La resurrección, id de la biblioteca marcado como tal, id inexistente sin resultados)
- [x] Rutas en inglés (RF-64, decisión 2026-10-21): rutas y parámetros traducidos, sin redirección desde las antiguas; las antiguas muestran «no encontrado» (corregidos dos textos «incompletas» que el builder había escapado)
- [x] Selección múltiple en Películas y Series y mandar a la papelera (RF-41, decisión 2026-10-21): probado en la demo en escritorio y móvil (corregidos en la revisión: se mandaban títulos ocultos por el filtro, la marca no se veía, la barra tapaba la última fila y el aviso de error no se cerraba)
- [x] Ajustes de la 0.1.8 en las cuadrículas (RF-41, decisión 2026-10-21): casillas de selección alineadas con los pósters; filtro de géneros en un desplegable con varios géneros (al menos uno): probado en la demo (marcos alineados antes y durante la selección, casilla a 8 px en todos; Acción + Aventura → 8 títulos, URL `?genre=Aventura,Acción`; lista a lo ancho en móvil)
- [x] Pregunta de relación con tarjetas de opción apiladas (RF-03, ajuste 2026-10-21: con muchos comprimidos los botones se solapaban): título con el número de archivos, tarjetas con ayuda que hace wrap, hoja inferior en móvil
- [x] Elegir coincidencia como opciones claras con póster, 5 por sección y «Mostrar 5 más» (RF-21, ajuste 2026-10-21): probado en la demo (tarjetas de radio con póster, «Elegida», 5 por sección y «Mostrar N más»; los resultados de la biblioteca llevan su póster guardado)
- [x] Hero: título largo sin logo que se sale de su caja; quitar la etiqueta «Destacado» (RF-40, ajuste 2026-10-21): tamaño según longitud y corte a 3 líneas dentro de la caja; sin «Destacado»
- [x] Prueba en el NAS con contenido real: subida, extracción, asignación, importación de la biblioteca existente y Jellyfin reconociéndolo todo; confirmado por el usuario el 2026-10-21 con la 0.1.10 (comprimido con contraseña, asignación desde No asignados, importación de la biblioteca existente, Jellyfin mostrando lo asignado sin escanear a mano, papelera y restaurar)

## Fase 13 — Documentación visual (posterior a 1.0.0)
Depende de: 12 · Petición del usuario 2026-10-21: capturas, GIF y diagramas en el README como GameExplorer
- [x] Capturas y GIF automáticos desde la demo con Playwright (`task screenshots`, como en GameExplorer: build demo, `vite preview` y `apps/web/scripts/screenshots.mjs` en el contenedor de Playwright con ffmpeg), en inglés y en español, en `docs/screenshots/<en|es>/`, a 1440×900 (móvil 390×844, escala 2): `home.png` (Inicio con hero y filas), `movie.png` (ficha de película con versiones y colección), `series.png` (ficha de serie con temporadas y capítulos presentes y faltantes), `assign.png` (Asignar con las opciones de coincidencia con póster y «Así quedará»), `unassigned.png` (No asignados), `select.png` (Películas en modo selección con dos marcados y el filtro de géneros abierto o activo) y `phone.png` (Inicio en móvil). GIF `assign.gif` (escritorio, ~15–25 s, ≤ 6 MB, 12 fps, ancho 1200 px, paleta optimizada): subir con «Probar con archivos de ejemplo» → No asignados → Asignar → elegir coincidencia → «Así quedará» → guardar → ficha del título
- [x] Correcciones vistas en las capturas: en móvil, los botones del hero se solapan con los indicadores; en inglés, la demo muestra sinopsis, géneros, colecciones y capítulos en español (RF-71, decisión 2026-10-21: los textos del catálogo de la demo siguen el idioma de la interfaz): hecho (indicadores debajo de los botones con 12 px en móvil; datos de la demo según el idioma)
- [x] Vista previa de Asignar en la demo con rutas de Jellyfin: hoy el adaptador demo arma `<título>/<archivo original>` (`catalogDemo.ts`); debe usar las reglas de §5 (`modules/catalog/domain/paths.ts`, vectores de `contracts/`) igual que el servidor; hecho con las funciones de dominio y tests de tabla
- [x] Repetir capturas tras las correcciones, con fichas encuadradas desde la cabecera, Asignar con «Así quedará» completo sin aviso de duplicado y el filtro de géneros activo con el menú cerrado (decisión del usuario 2026-10-21: pósters reales en las capturas, con atribución a TMDB); hecho: todas a 1440×900 (móvil 390×844 ×2), fichas desde la cabecera, Asignar con rutas de Jellyfin, filtro activo con el menú cerrado; GIF ≤ 5 MB por idioma
- [x] README EN/ES para portfolio: GIF arriba, capturas en tabla, vista móvil, nota «capturas y GIF generados desde la demo con `task screenshots`», diagramas Mermaid (despliegue en TrueNAS con Jellyfin, TMDB, SMB e inotify; estados de un trabajo de subida RF-10a) y tabla de stack; hecho

## Fase 14 — Icono y diagramas interactivos (posterior a 1.0.1)
Depende de: 13 · Petición del usuario 2026-10-23
- [ ] Favicon «VE» con la paleta de la app (`docs/design-handoff.md`, componente `Favicon`): SVG, ICO, apple-touch-icon e iconos 192/512 en `apps/web/public/`, enlazados en `index.html` con `theme-color`; servidos por el binario Go (SPA embebida) y por Vercel sin caer en la reescritura de la SPA; también en la demo
- [ ] Diagramas interactivos con la skill [archify](https://github.com/tt-a1i/archify) v3.2.0 (instalada en `~/.agents/skills/archify`), equivalentes a los del README pero explorables: arquitectura respaldada por el código (`--repo-root`, con TrueNAS, binario Go + React, SQLite, dataset, 7-Zip/ffprobe, Jellyfin, TMDB, SMB/inotify y la demo en Vercel) y ciclo de vida de un trabajo de subida (RF-10a), en inglés y en español, con fuentes JSON versionadas en `docs/architecture/` y HTML generados en `docs/architecture/site/`; página `index.html` sencilla con la paleta de la app que enlaza los cuatro diagramas; workflow `.github/workflows/pages.yml` que publica `docs/architecture/site/` en GitHub Pages (`https://carlossv923.github.io/VideoExplorer/`) al cambiar esa carpeta en `main` y a mano; enlace en los README

