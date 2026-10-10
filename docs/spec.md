# VideoExplorer — Especificación del MVP

> Fuente de verdad de los requisitos. Si una decisión no está aquí, se consulta y se documenta antes de implementarla.
> Estado de trabajo: [`tasks.md`](./tasks.md). Proyecto hermano y base técnica: GameExplorer.
> Las decisiones todavía sin cerrar están en §9 **Puntos abiertos**; ninguna fase que dependa de uno empieza sin resolverlo.

## 1. Problema y objetivo

Las películas y series se guardan en un TrueNAS SCALE compartido por SMB y se reproducen con Jellyfin y Plex. Agregar contenido exige descargar, descomprimir, buscar el nombre correcto, crear carpetas por temporada y renombrar cada capítulo con el formato que esos servidores reconocen; además se olvida qué hay guardado y qué falta.

VideoExplorer es una app web autoalojada, con estética de Netflix / Prime Video, que desde cualquier navegador (escritorio, móvil o tablet táctil y gamepad) permite **subir, descomprimir, clasificar, renombrar y organizar** películas y series con datos de TMDB, y luego explorarlas, editarlas y descargarlas. **No reproduce contenido.** La subida no pide nombre: los archivos se suben y descomprimen directamente y el usuario los asigna al terminar. Es también un proyecto público de portfolio con una demo sin backend.

## 2. Glosario

| Término | Significado |
|---|---|
| Título | Una película o una serie de la biblioteca, enlazada a TMDB |
| Película | Título de tipo película: `Movies/<Película (Año)>/` |
| Serie | Título de tipo serie, con temporadas y capítulos: `TV Shows/<Serie (Año)>/Season NN/` |
| Capítulo | Un episodio de una serie (temporada + número). Un archivo puede contener varios capítulos consecutivos (§5) |
| Especial | Capítulo de la temporada 0 (`Season 00`), como en TMDB, Plex y Jellyfin |
| Versión | Otro archivo del mismo destino (película o capítulo) que el usuario decidió conservar, distinguido por una etiqueta (`- 2160p`) |
| Parte | Un trozo de una película repartida en varios archivos (`- part1`, `- part2`) |
| Subtítulo externo | `.srt`, `.ass`, `.ssa`, `.sub`/`.idx`, `.vtt` que acompaña a un video y se renombra con él |
| Etiquetas de ID | Sufijos opcionales con el id de TMDB en el nombre de la carpeta: `{tmdb-ID}` (Plex) y `[tmdbid-ID]` (Jellyfin) |
| No asignados | Carpeta `_unassigned/` con lo que todavía no pertenece a ningún título: subidas recién procesadas y lo copiado por Samba |
| Entrada | Elemento de No asignados: una carpeta de primer nivel o un archivo suelto de `_unassigned/` |
| Lote | Conjunto de archivos soltados juntos en el navegador, con la relación que eligió el usuario (RF-03) |
| Staging | Zona temporal (`.videoexplorer/staging`) donde se sube y descomprime |

## 3. Requisitos funcionales

### Subida
- **RF-01** Subir uno o varios archivos de video, subtítulos o comprimidos arrastrándolos a cualquier parte de la ventana o con el botón «Subir», que abre el selector nativo (solo archivos).
- **RF-01a** **Carpetas** (solo al arrastrar): se recorre su contenido y se suben sus archivos conservando la ruta relativa. Cada carpeta soltada es un lote (aplica la pregunta de RF-03 si contiene varios archivos) y su nombre es el nombre del lote (RF-08). Después se filtra igual que un comprimido (RF-07).
- **RF-02** Las subidas son reanudables (protocolo tus) y no tienen límite práctico de tamaño. Se suben 2 archivos a la vez; el resto espera en cola; reintento automático y botón Reintentar; si se recarga la pestaña, la subida queda **Interrumpida** y se continúa eligiendo de nuevo el mismo archivo (mismo nombre y tamaño). Mientras haya subidas en curso, el navegador pide confirmar antes de cerrar.
- **RF-03** **Sin formulario de nombre.** Con **un solo archivo**, la subida empieza al soltarlo. Con **varios**, aparece una única pregunta de un clic y la subida empieza al responder:
  - **Partes de una misma película o serie** (p. ej. `cd1`/`cd2`, o varios archivos de una misma obra).
  - **Capítulos de una misma serie**.
  - **Sin relación**.
  Los comprimidos multiparte (`.partN.rar`, `.7z.001`, `.zip.001`…) se reconocen y reúnen solos por nombre, como en GameExplorer, sin importar la respuesta; se descomprimen desde el primer volumen cuando están todas las partes. El formato antiguo `.r00` no se admite.
- **RF-04** Los comprimidos (zip, 7z, rar/rar5, reconocidos por bytes mágicos) se descomprimen al terminar la subida con el `7zz` oficial. Si está cifrado, el trabajo queda **«Necesita contraseña»** y se pide en la UI; la contraseña no se guarda.
- **RF-05** Antes de descomprimir se comprueba el espacio libre (margen de 1 GiB). Protección zip-slip y rechazo de symlinks. Tras descomprimir se verifica cada archivo contra el índice del comprimido (tamaño + CRC32); los errores *solo de atributos* de 7zz se toleran como advertencia (hallazgo de GameExplorer).
- **RF-06** Tras una extracción correcta, el **comprimido original va a la papelera** (RF-30), restaurable durante la retención.
- **RF-07** **Filtrado del contenido.** Se conservan los videos (extensiones de `VIDEO_EXTENSIONS`, por defecto `.mkv .mp4 .avi .m4v .ts .m2ts .mov .wmv .webm .mpg .mpeg`) y los subtítulos externos (`SUBTITLE_EXTENSIONS`, por defecto `.srt .ass .ssa .sub .idx .vtt`). Se descartan los *samples* (menos de 150 MB **y** «sample» en el nombre o en su carpeta), `.nfo`, `.txt`, imágenes, `.url`, `.exe` y todo lo demás. **Lo descartado va a la papelera**, no se borra.
- **RF-08** Al terminar, el contenido conservado del lote se mueve a **No asignados** como una entrada (`_unassigned/<nombre del lote>/`, o un archivo suelto si es un único video sin subtítulos). El nombre del lote es el del comprimido o del primer archivo sin extensión, saneado; si existe, se agrega el sufijo ` (2)`, ` (3)`… La relación elegida en RF-03 queda guardada en la entrada y precarga Asignar (RF-27a). Con «Sin relación», cada archivo es su propia entrada.
- **RF-09** Las subidas abandonadas y el staging huérfano se purgan tras 24 h.
- **RF-10** El progreso (subida, unión de partes, descompresión, filtrado) se ve en vivo por SSE en el panel **Subidas** y en un indicador en la cabecera mientras haya trabajos activos o esperando al usuario.

### Asignar (No asignados → biblioteca)
- **RF-20** **Sugerencias automáticas.** Para cada video de una entrada se analiza el nombre del archivo y de sus carpetas (`Breaking.Bad.S01E02.1080p.WEB.mkv`, `Iron.Man.2008.BluRay.mkv`, `1x02`, `S01E01E02`, `S00E03`…) y se propone tipo, título, año, temporada y capítulo(s), con la coincidencia de TMDB ya buscada. **Nunca se asigna nada sin confirmación del usuario.** Un botón «Confirmar todo» acepta las sugerencias de una entrada completa.
- **RF-21** **Elegir título.** El buscador de TMDB muestra primero los títulos que ya están **en tu biblioteca** («Asignar a existente») y luego los resultados de TMDB (póster, año, tipo). Asignar a un título existente agrega los archivos a su carpeta. Siempre se puede escribir un **nombre libre** (título y año) en lugar de elegir de TMDB (§6).
- **RF-22** **Datos por video:**
  - **Película**: solo el título. Si la entrada trae varios videos marcados como partes, cada uno pide su número de parte (sugerido por nombre: `cd1`, `part2`, `disc1`…). **Partes y versiones son excluyentes**: una película en partes no tiene versiones; si llega un archivo entero para una película en partes (o partes para una con archivo entero), el duplicado (RF-24) solo ofrece Reemplazar u Omitir.
  - **Capítulo**: serie, temporada (0 = especiales) y capítulo; opcionalmente **capítulo final** para archivos con varios capítulos consecutivos (`S01E01-E02`). El título del capítulo se toma de TMDB. Se avisa si TMDB no conoce esa temporada o capítulo, pero se permite guardar.
  - **No guardar**: el video se queda en No asignados.
  Con la relación «Capítulos de una misma serie», la serie se elige una vez para toda la entrada y cada video solo pide temporada y capítulo (sugeridos).
- **RF-23** **Subtítulos.** Cada subtítulo se asocia al video con el mismo nombre base (o, si no hay coincidencia, el usuario lo elige). Su idioma se propone en este orden: (1) código o nombre en el archivo (`.spa.`, `.es.`, `.eng.`, `Spanish`, `English`…); (2) detección por el texto con una librería Go *offline*. El usuario confirma o cambia el idioma y las marcas **forzado** (`.forced`) y **SDH** (`.sdh`). Se nombran según §5.
- **RF-24** **Duplicados.** Si el destino (la película, o ese capítulo de esa serie) ya tiene un archivo, se pregunta:
  - **Reemplazar**: el anterior va a la papelera.
  - **Conservar ambos**: los dos quedan como **versiones** con una etiqueta automática leída con ffprobe (resolución y, si coinciden, códec: `2160p`, `1080p`, `1080p HEVC`), **editable** antes de confirmar. El archivo existente se renombra con su etiqueta en la misma operación revertible.
  - **Omitir**: el nuevo se queda en No asignados.
  Si el contenido parece **idéntico** (mismo tamaño y mismo hash parcial: primeros, centrales y últimos 4 MiB), se avisa «Copia exacta» y se propone Omitir. Los archivos en la papelera no cuentan como duplicado.
- **RF-25** **Confirmar.** Siempre hay una vista previa con las rutas finales, los duplicados y las decisiones, y el botón Guardar. Al guardar, los movimientos pasan por el **motor de operaciones con journal** (como GameExplorer): si algo falla o la app se reinicia a mitad, se deshace todo y la entrada vuelve a No asignados. Nunca se sobrescribe un archivo existente.
- **RF-26** La entrada queda **«Asignándose»** (sin otras acciones) mientras el formulario está abierto; cancelar la libera. Lo no guardado se queda en la entrada; la entrada desaparece cuando queda vacía.

### No asignados y Samba
- **RF-27** **Sección No asignados** (`_unassigned/`): lista sus entradas con nombre, archivos (de cualquier subcarpeta), tamaño total y estado. **Al abrir la sección se lee el directorio en ese momento**: todo lo copiado por Samba aparece sin esperar al escaneo. Un archivo cuyo tamaño o fecha cambió desde la lectura anterior, o modificado hace menos de 1 min, se muestra **«Copiando…»** y no admite acciones hasta estabilizarse. Por entrada: **Asignar** (RF-20 a RF-26), **Descargar** (archivo o zip *store*), **Mandar a la papelera**, **Borrar definitivamente**; lo mismo por archivo dentro de la entrada.
- **RF-27a** **Comprimidos copiados por Samba.** Un comprimido estable en `_unassigned/` se procesa igual que una subida (RF-04 a RF-08): se descomprime (pidiendo contraseña si hace falta), se filtra, el original va a la papelera y el resultado queda como entrada. Mientras tanto la entrada muestra el progreso del trabajo.
- **RF-28** **Escaneo de la biblioteca**: `_unassigned/` cada `UNASSIGNED_SCAN_INTERVAL` (5 min por defecto) y la biblioteca completa cada `LIBRARY_SCAN_INTERVAL` (1 h por defecto), además de al arrancar y a pedido («Escanear ahora», con la hora de la última revisión). Un archivo solo se procesa o mueve si no cambió (tamaño y fecha) desde el escaneo anterior. Se ignoran `.videoexplorer/`, dotfiles, nombres que empiezan por `@`, `#` o `$`, `Thumbs.db`, `desktop.ini` y `.DS_Store`.
- **RF-29** **Importar lo existente.** Al escanear `Movies/` y `TV Shows/`:
  - lo que **sigue el formato de §5** se registra solo, buscando en TMDB por título y año (o por id si la carpeta trae etiqueta de ID) para traer póster y datos;
  - lo que **no lo sigue** se mueve a `_unassigned/` conservando su ruta relativa, para clasificarlo;
  - un archivo **borrado** por Samba desaparece de la biblioteca; si era el último de su título, el título también.
  - lo que sigue el formato pero **no tiene una coincidencia única** en TMDB (o no hay clave de TMDB) se registra **en su sitio** como título libre «Sin vincular», sin mover nada; aparece en Pendientes (RF-40) como «N sin vincular» con la acción **Vincular**, que muestra los candidatos de TMDB.

### Biblioteca (estética Netflix / Prime Video)
- **RF-40** **Inicio**:
  - **Hero** con backdrop, logo (o título), año, géneros, sinopsis y botones «Ver detalles» y «Editar». **Rota cada 8 s entre los 5 últimos títulos agregados que tienen backdrop**, con indicadores y flechas; se pausa con hover o foco y no rota con `prefers-reduced-motion`. Sin títulos con backdrop, no hay hero.
  - **Filas horizontales**: «Agregados recientemente», «Películas», «Series» y una fila por **género** de TMDB con contenido.
  - **Filas de colecciones** de TMDB (sagas) y una **ficha de colección** con sus películas (las que faltan en la biblioteca se muestran atenuadas).
  - **Pendientes**: aviso o fila con «N en No asignados», «Series incompletas» (capítulos ya emitidos según TMDB que no están en la biblioteca) y «Sin vincular» (RF-29).
  Si la biblioteca está vacía, Inicio muestra un estado vacío con «Subir».
- **RF-41** **Explorar**: pantallas «Películas» y «Series» en cuadrícula de pósters con orden (agregado, título, año) y filtro por género.
- **RF-42** **Búsqueda global**: cada palabra debe aparecer en el título (original, mostrado o en disco), sin mayúsculas ni acentos.
- **RF-43** **Ficha de película**: backdrop, póster, título en español y original, año, duración, géneros, sinopsis, colección; archivos (versiones y partes) con sus subtítulos. **Ficha de serie**: lo mismo más selector de temporada y lista de capítulos (presentes y faltantes según TMDB, con miniatura, título y sinopsis).
- **RF-44** **Acciones** en la ficha (todo reversible si falla a mitad):
  - **Editar y reasignar**: cambiar la coincidencia de TMDB, el tipo, la temporada/capítulo de un archivo, la etiqueta de versión, el número de parte o el idioma de un subtítulo; renombra en disco. Si el destino choca, aplica RF-24.
  - **Etiquetas de ID** (RF-45).
  - **Mover a No asignados**: el título completo, una temporada o un archivo.
  - **Mandar a la papelera**: el título completo, una temporada, un capítulo o una versión concreta.
  - **Descargar**: un archivo (HTTP Range) o una temporada/serie/película completa como zip *store* en streaming con tamaño exacto anunciado.
  - **Info técnica** (ffprobe, cacheada): resolución, códecs de video y audio, pistas de audio y de subtítulos internos con idioma, duración, tamaño y ruta en el NAS.
- **RF-45** **Etiquetas de ID.** Por defecto las carpetas **no** llevan id: `Iron Man (2008)`. Desde la ficha se puede activar la etiqueta de **Plex**, la de **Jellyfin** o **ambas** para ese título, lo que renombra su carpeta (y, en películas, sus archivos): `Iron Man (2008) {tmdb-1726} [tmdbid-1726]`. Se puede volver a quitar.

### Papelera
- **RF-30** Lo eliminado (títulos, temporadas, archivos, entradas de No asignados, comprimidos originales y descartes del filtrado) se mueve a `.videoexplorer/trash` con su metadata y se purga tras `TRASH_RETENTION_DAYS` (**30** por defecto). Se puede borrar una entrada para siempre o **vaciar la papelera** (con confirmación).
- **RF-31** **Restaurar** devuelve cada cosa a su **ubicación original** con su vínculo de TMDB; lo que venía de No asignados, de una subida o del filtrado vuelve a No asignados. Si el lugar está ocupado, aplica RF-24 (Reemplazar / Conservar ambos / Cancelar).

### Integración con servidores multimedia
- **RF-50** Si se configuran `JELLYFIN_URL` + `JELLYFIN_API_KEY` y/o `PLEX_URL` + `PLEX_TOKEN`, cada cambio en la biblioteca (asignar, renombrar, papelera, restaurar) pide a ese servidor refrescar **solo la carpeta afectada**, agrupando los cambios en ventanas de 30 s. Sin variables, no hace nada. Un fallo del servidor multimedia se registra y se muestra en Ajustes, pero nunca bloquea la operación.

### Acceso y preferencias
- **RF-60** Contraseña única (`APP_PASSWORD_HASH` argon2id o `APP_PASSWORD`), sesión en cookie httpOnly firmada `SameSite=Strict`, rate-limit en el login; si la sesión vence, se vuelve al login y luego a donde se estaba. Reutiliza el contexto `identity` de GameExplorer.
- **RF-61** Interfaz en español (por defecto) e inglés; la elección se recuerda en el navegador. Los datos de TMDB se muestran en `TMDB_LANGUAGE` (por defecto `es-MX`) con respaldo en inglés si falta la traducción.
- **RF-62** **Ajustes**: General (idioma, escaneo con última revisión y «Escanear ahora», estado de Jellyfin/Plex, cerrar sesión, atribución a TMDB) y Papelera.

### Modo demo
- **RF-70** Con `VITE_DATA_SOURCE=demo` el frontend funciona sin backend con adaptadores en memoria (estado de la sesión). Biblioteca sembrada con películas, series (con temporadas incompletas), colecciones y entradas en No asignados.
- **RF-71** **Imágenes de la demo**: placeholders generados (degradados con el título) o de dominio público; **no** se embeben pósters de TMDB.
- **RF-72** Subida, unión de partes, descompresión, contraseña, filtrado, sugerencias, duplicados y guardado se simulan con la misma máquina de estados que el backend. Acepta archivos propios usando solo su nombre y tamaño (nunca salen del navegador). Botón «Probar con archivos de ejemplo». Banner fijo de modo demo, sin login.

## 4. Requisitos no funcionales

- **RNF-01 Archivos grandes**: los bytes de los videos nunca se cargan en memoria; streams y `rename`; extracción con `7zz` en proceso hijo; ffprobe en proceso hijo; descargas con `http.ServeContent` o zip *store* en streaming. Objetivo: RAM estable < 300 MB saturando la red.
- **RNF-02 Seguridad**: rutas construidas en el servidor a partir de IDs, nunca del cliente; zip-slip; nombres saneados; ningún secreto versionado. Las claves de TMDB, Jellyfin y Plex nunca llegan al navegador.
- **RNF-03 Permisos**: igual que GameExplorer: `PUID/PGID`, ACL heredada del dataset (*Modify*, *Inherit*); si la app no puede escribir al arrancar lo muestra en la UI.
- **RNF-03a Binarios externos**: `7zz` oficial (versión fijada y SHA-256 verificado, con RAR) y `ffprobe` estático (versión fijada y SHA-256 verificado) en la imagen.
- **RNF-04 Atomicidad**: staging, papelera y `_unassigned/` viven en el mismo dataset que `Movies/` y `TV Shows/`, así que mover es `rename`; ante `EXDEV`, copiar y borrar. Todo cambio de la biblioteca pasa por el motor con journal.
- **RNF-05 Táctil y responsive**: objetivos de 44 px, nada que dependa solo del hover, swipe en las filas, layouts para móvil, tablet y escritorio.
- **RNF-06 Entrada**: ratón, teclado, táctil y gamepad (navegación espacial y acciones, como GameExplorer); glifos de gamepad solo con un control detectado. Atajos de teclado en Asignar.
- **RNF-07 Accesibilidad**: WCAG 2.1 AA.
- **RNF-08 Arquitectura**: la de GameExplorer: DDD back y front, OpenAPI spec-first, reglas compartidas Go/TS fijadas con vectores dorados en `contracts/` (nombres, saneamiento, análisis de nombres de archivo, etiquetas de versión).
- **RNF-09 Calidad**: lint, tipos estrictos, tests unitarios, de integración y E2E en CI; `task check` igual que CI.
- **RNF-10 Legal**: la app no distribuye contenido. Muestra la atribución exigida por TMDB («This product uses the TMDB API but is not endorsed or certified by TMDB.» y su logo). Las imágenes de TMDB se sirven desde una caché local: el navegador nunca contacta a terceros (salvo la demo, que no usa TMDB). Licencia MIT.

## 5. Nomenclatura (vectores: `contracts/naming-cases.json`)

Raíces dentro de `LIBRARY_PATH` (por defecto `Movies` y `TV Shows`, configurables con `MOVIES_DIR` y `SERIES_DIR`):

```
/library
├── Movies/
│   └── Iron Man (2008)/
│       ├── Iron Man (2008).mkv
│       └── Iron Man (2008).es.srt
├── TV Shows/
│   └── Breaking Bad (2008)/
│       ├── Season 00/
│       │   └── Breaking Bad (2008) - S00E01 - Good Cop Bad Cop.mkv
│       └── Season 01/
│           ├── Breaking Bad (2008) - S01E01 - Pilot.mkv
│           └── Breaking Bad (2008) - S01E01 - Pilot.en.forced.srt
├── _unassigned/
└── .videoexplorer/        (staging, trash, ops)
```

| Caso | Carpeta | Archivo |
|---|---|---|
| Película | `Movies/<T> (<A>)<ids>/` | `<T> (<A>)<ids>.<ext>` |
| Película, versión | ídem | `<T> (<A>)<ids> - <versión>.<ext>` |
| Película, parte N | ídem | `<T> (<A>)<ids> - part<N>.<ext>` |
| Capítulo | `TV Shows/<T> (<A>)<ids>/Season <SS>/` | `<T> (<A>) - S<SS>E<EE> - <título cap.>.<ext>` |
| Varios capítulos | ídem | `<T> (<A>) - S<SS>E<EE>-E<FF> - <título cap.>.<ext>` |
| Capítulo, versión | ídem | `<T> (<A>) - S<SS>E<EE> - <título cap.> - <versión>.<ext>` |
| Subtítulo | junto a su video | `<nombre base del video>.<idioma>[.forced][.sdh].<ext>` |

- `<T>`: título del disco. Es el **título original** de TMDB si está en alfabeto latino; si no (japonés, coreano, ruso…), el **título en inglés** (`en-US`). `Amélie`, `The Godfather`, `Spirited Away`.
- `<A>`: año del estreno (película) o de la primera emisión (serie).
- `<ids>`: vacío por defecto; ` {tmdb-ID}`, ` [tmdbid-ID]` o ` {tmdb-ID} [tmdbid-ID]` según RF-45. En series solo va en la carpeta; en películas también en los archivos (Jellyfin exige que los archivos de versiones empiecen como la carpeta).
- `<SS>`, `<EE>`: dos dígitos como mínimo (`S01E05`, `S01E105`). Temporada 0 → `Season 00`.
- `<título cap.>`: de TMDB, en el idioma original de la serie si es latino, si no en inglés. Con varios capítulos, los títulos se unen con ` + ` (`Pilot + Cat's in the Bag`); si solo difieren en un número final o entre paréntesis (`The Finale (1)`, `The Finale, Part 2`), se colapsan en el título común (`The Finale`). Si TMDB no tiene título, se omite el segmento ` - <título cap.>`.
- `<versión>`: etiqueta de RF-24 (`2160p`, `1080p HEVC`, o texto libre del usuario, saneado).
- `<idioma>`: ISO 639-1 (`es`, `en`, `ja`…) con región opcional elegida por el usuario (`es-MX`, `es-ES`, `pt-BR`, `pt-PT`).
- `<ext>`: la del archivo, en minúsculas.

**Saneamiento** (títulos de disco, de capítulo y etiquetas), en este orden (convención Radarr/Sonarr):
1. Unicode NFC (se conservan acentos, `ñ` y símbolos válidos como `™`).
2. `: ` → ` - ` y `:` → `-` (`Mission: Impossible` → `Mission - Impossible`).
3. `/` y `\` → `-`.
4. `"` → `'`.
5. Se eliminan `? * < > |` y los caracteres de control (`What If...?` → `What If...`).
6. Bloques de espacios → uno; se recortan espacios iniciales y espacios y puntos finales.

Una ruta cuyo componente supere 255 bytes recorta primero el título del capítulo.

## 6. Datos de TMDB

- Clave: `TMDB_API_KEY` (v3) o `TMDB_READ_TOKEN` (v4). **Es opcional** (decisión 2026-10-09):
  - Sin clave, o si TMDB no responde, todo funciona con **nombre libre**: en Asignar se escriben a mano título y año (y, en capítulos, el título del capítulo, que puede quedar vacío). No hay sugerencias de TMDB, aunque el análisis del nombre de archivo (RF-20) sigue proponiendo título, año, temporada y capítulo.
  - Un título libre usa un póster genérico con su nombre, no tiene géneros, colección ni lista de capítulos faltantes, y no admite etiquetas de ID (RF-45). Aparece en «Agregados recientemente», «Películas»/«Series» y búsqueda, pero no en filas de género ni de colección.
  - Un título libre se puede **vincular a TMDB** más tarde desde Editar (RF-44), lo que puede renombrarlo.
  - La importación (RF-29) sin TMDB registra los títulos con su nombre y año de la carpeta como títulos libres.
  - La atribución a TMDB solo aparece si está configurado.
- Se usan: búsqueda de películas y series, detalle (títulos, año, géneros, sinopsis, colección, imágenes y logos), temporadas y capítulos, colecciones, idioma original.
- Caché local de imágenes en `/data/cache/images` y de respuestas en SQLite con caducidad; respeto de los límites de TMDB.

## 7. Ambientes

| | Local | Producción (TrueNAS SCALE) | Demo (Vercel) |
|---|---|---|---|
| Back | Go en Docker | Contenedor GHCR con la SPA incrustada, `7zz` y `ffprobe` | — |
| Front | Vite dev con proxy `/api` | Servido por Go | Estático, `VITE_DATA_SOURCE=demo` |
| Datos | `./.dev/` | Datasets `/library` (SMB) y `/data` (SQLite y caché) | Memoria de la sesión |
| TMDB | Clave de desarrollo | Clave de producción | Catálogo fijo sin imágenes de TMDB |

Variables: `PORT`, `LIBRARY_PATH`, `DATA_PATH`, `MOVIES_DIR`, `SERIES_DIR`, `APP_PASSWORD[_HASH]`, `SESSION_SECRET`, `SESSION_TTL`, `COOKIE_SECURE`, `TMDB_API_KEY`/`TMDB_READ_TOKEN`, `TMDB_LANGUAGE`, `JELLYFIN_URL`, `JELLYFIN_API_KEY`, `PLEX_URL`, `PLEX_TOKEN`, `PUID`, `PGID`, `TRASH_RETENTION_DAYS`, `EXTRACT_CONCURRENCY`, `UNASSIGNED_SCAN_INTERVAL`, `LIBRARY_SCAN_INTERVAL`, `VIDEO_EXTENSIONS`, `SUBTITLE_EXTENSIONS`, `LOG_LEVEL`. Ver `.env.example`.

## 8. Fuera de alcance del MVP

Reproducción · descarga de contenido desde internet (torrents, Usenet) · anime con numeración absoluta (se asigna a mano por temporada y capítulo) · capítulos repartidos en varias partes · bibliotecas adicionales (anime, documentales, conciertos) · usuarios y roles · TV con control remoto como objetivo de diseño · extras (trailers, detrás de cámaras) · edición de metadatos fuera de TMDB.

## 9. Puntos abiertos

Se resuelven con el usuario, uno a uno, antes de la fase que los necesita.

1. **Etiquetas de ID** (Fase 0.5): verificar en Jellyfin y Plex reales que `{tmdb-ID} [tmdbid-ID]` juntos funcionan en ambos.
2. ~~Versión + parte~~ **Resuelto**: son excluyentes (RF-22).
3. ~~Título con varios capítulos~~ **Resuelto**: unidos con ` + `, colapsando partes numeradas (§5).
4. ~~Importación ambigua~~ **Resuelto**: queda en su sitio como «Sin vincular» (RF-29).
5. ~~¿TMDB obligatorio?~~ **Resuelto**: opcional, con nombre libre (§6).
6. ~~Soltar carpetas~~ **Resuelto**: se permiten al arrastrar (RF-01a).
7. ~~Título destacado del hero~~ **Resuelto**: rotativo entre los 5 recientes (RF-40).
8. ~~Nombre de las carpetas raíz~~ **Resuelto**: `Movies/` y `TV Shows/` (§5).
9. ~~Idioma de los subtítulos~~ **Resuelto**: ISO 639-1 con región opcional (§5); verificar en el spike de la Fase 0.5.
10. ~~Repositorio e imagen~~ **Resuelto**: `github.com/CarlosSV923/VideoExplorer` público (MIT) e imagen `ghcr.io/carlossv923/videoexplorer`.
