# VideoExplorer

[English](README.md) · **Estado: fases 0–9 cerradas; fase 10 en curso**

[![CI](https://github.com/CarlosSV923/VideoExplorer/actions/workflows/ci.yml/badge.svg)](https://github.com/CarlosSV923/VideoExplorer/actions/workflows/ci.yml)

VideoExplorer es una app web autoalojada para un NAS doméstico (TrueNAS SCALE). Organiza películas y series para que Jellyfin las reconozca, con una interfaz de biblioteca estilo Netflix. Gestiona archivos; nunca reproduce video.

## Qué hace hoy

- Explora una biblioteca estilo Netflix de películas, series, temporadas, capítulos y colecciones.
- Sube uno o varios archivos con subidas reanudables mediante [tus](https://tus.io/), incluidos comprimidos zip, 7z y rar.
- Descomprime con 7-Zip, filtra videos y subtítulos externos, y manda lo descartado a la papelera en vez de borrarlo.
- Lleva los archivos subidos o copiados por SMB a **No asignados** y **Asignar**, con sugerencias de títulos de TMDB o un nombre libre.
- Nombra carpetas, videos y subtítulos con la convención de Jellyfin, incluida la etiqueta opcional `[tmdbid-ID]`, decisiones de duplicados, versiones, partes y numeración de capítulos.
- Edita y reasigna archivos de la biblioteca, descarga archivos o títulos, restaura elementos de la papelera y la vacía de forma permanente.
- Ofrece ajustes de idioma, escaneos, TMDB y Jellyfin, incluido un aviso para actualizar Jellyfin tras cambios en la biblioteca cuando está configurado.
- Funciona en español e inglés con ratón, teclado, táctil y gamepad.

Los requisitos detallados y el trabajo actual están en [`docs/spec.md`](docs/spec.md) y [`docs/tasks.md`](docs/tasks.md). El sistema de diseño y el comportamiento adaptable están en [`docs/design-handoff.md`](docs/design-handoff.md). Los vectores de nombres y clasificación están en [`contracts/`](contracts/README.md).

## Demo en línea

**Demo:** [video-explorer-sepia.vercel.app](https://video-explorer-sepia.vercel.app). Funciona sin backend; los cambios en memoria se pierden al recargar. Las imágenes de TMDB se cargan desde `image.tmdb.org`.

## Imagen de producción

La imagen para TrueNAS es `ghcr.io/carlossv923/videoexplorer`. Al crear un tag de versión `v*`, se publican el tag semver correspondiente y `latest` para `linux/amd64`; los pull requests solo construyen la imagen. Para construirla localmente, ejecuta `task image`.

La imagen incluye `ffprobe` de jellyfin-ffmpeg 8.1.3-1, bajo GPL-3.0. Su licencia y aviso están en `/usr/share/doc/videoexplorer/ffprobe/` dentro de la imagen. El [código fuente de la versión fijada](https://github.com/jellyfin/jellyfin-ffmpeg/tree/v8.1.3-1) está disponible en Jellyfin-FFmpeg. `ffprobe` se ejecuta como proceso independiente; VideoExplorer conserva su licencia MIT.

## Estado del proyecto

Las fases 0 a 11 están cerradas: la API, la interfaz web completa (biblioteca, subidas, No asignados, Asignar, edición, papelera, ajustes), probada con archivos reales, y la demo sin backend publicada en Vercel con pruebas de punta a punta. Sigue la fase 12, el empaquetado de producción para TrueNAS; el avance se sigue en [`docs/tasks.md`](docs/tasks.md).

## Desarrollo

Requisitos: Docker, Node.js 24 o posterior, pnpm 12.9.1 y Task 3.

Instala las dependencias y levanta los servicios:

```bash
task install
task dev:api       # API de Go en Docker, http://localhost:8080
task dev:web       # frontend de Vite, http://localhost:5173
```

El servicio de desarrollo de la API está definido en [`compose.dev.yaml`](compose.dev.yaml). Usa `./.dev/library` y `./.dev/data` como datos locales de desarrollo y pasa la configuración local opcional de `.env.local`. No subas secretos al repositorio.

Comprobaciones útiles:

```bash
task check         # OpenAPI, lint/tests de Go y web, vulnerabilidades y builds
task gen           # Regenera el código de OpenAPI y SQL
task build:web     # Construye el frontend
task build:demo    # Construye con los adaptadores de demo
task e2e           # Ejecuta Playwright en Docker (escritorio y móvil táctil)
```

Consulta [`Taskfile.yml`](Taskfile.yml) para ver todas las tareas. Las herramientas de Go corren en Docker para coincidir con la CI y con las rutas Linux de producción.

## Arquitectura

El backend y el frontend usan Domain-Driven Design, con paquetes de dominio puros y puertos/adaptadores. Las áreas principales son:

| Área | Responsabilidad |
|---|---|
| `apps/api` | API de Go, SQLite, operaciones, extracción y organización de archivos |
| `apps/web` | React, Vite, TypeScript, i18n e interfaz adaptable |
| `api` | Fuente de verdad de OpenAPI |
| `contracts` | Vectores dorados compartidos |
| `docs` | Requisitos, estado de tareas y handoff de diseño |

Los archivos se transmiten y los datos de video no se almacenan completos en memoria. Las rutas se construyen en el servidor a partir de IDs y los secretos de TMDB y Jellyfin nunca llegan al navegador. Plex queda fuera de esta versión.

## Legal

VideoExplorer no incluye, descarga ni distribuye películas o series. Úsalo solo con archivos que tengas derecho a conservar. TMDB se usa para metadatos; VideoExplorer no está avalado ni certificado por TMDB.

Licencia [MIT](LICENSE).
