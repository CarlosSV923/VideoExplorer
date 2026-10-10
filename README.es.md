# VideoExplorer

[English](README.md) · **Estado: fase 1 en curso (API base)**

[![CI](https://github.com/CarlosSV923/VideoExplorer/actions/workflows/ci.yml/badge.svg)](https://github.com/CarlosSV923/VideoExplorer/actions/workflows/ci.yml)

App web autoalojada para un NAS doméstico (TrueNAS SCALE) que permite **subir, descomprimir, clasificar, renombrar y organizar películas y series** para que Jellyfin las reconozca. La interfaz sigue la estética de Netflix y Prime Video y funciona desde cualquier navegador: escritorio, tablet, pantalla táctil o gamepad.

> **VideoExplorer nunca reproduce video.** Gestiona los archivos; los ves con Jellyfin.

## Qué hace

Los requisitos completos están en [`docs/spec.md`](docs/spec.md). En resumen:

- **Sube** archivos de video, subtítulos y comprimidos arrastrándolos a cualquier parte de la ventana. Las subidas son reanudables y no tienen un límite de tamaño práctico.
- **Descomprime** zip, 7z y rar con el 7-Zip oficial, los verifica contra su CRC y conserva solo los videos y los subtítulos externos. Lo descartado va a la papelera, no se borra.
- **Asigna** cada archivo a una película o una serie, con datos de TMDB o con un nombre libre. Nada entra en la biblioteca sin tu confirmación.
- **Nombra** todo con el formato que espera Jellyfin. Los nombres de carpetas y archivos y la etiqueta de ID (`[tmdbid-ID]`, opcional) siguen la convención de Jellyfin. Los duplicados se pueden reemplazar, conservar como versiones u omitir; hay una vista previa antes de guardar y una operación fallida se deshace.
- **Recoge** los archivos que copies por SMB en la sección No asignados, e importa la biblioteca que ya está en disco.
- **Explora** películas, series y colecciones, busca, edita, manda a la papelera con restauración y descarga archivos o títulos completos.
- **Refresca** Jellyfin en la carpeta afectada, cuando está configurado.
- Español e inglés, con ratón, teclado, táctil y gamepad.

## Qué no hace

- No reproduce contenido.
- No descarga contenido de internet (torrents, Usenet).
- No incluye ninguna película ni serie.
- Plex no se soporta en esta primera versión (nombres, etiquetas de ID y refresco). Podría añadirse en el futuro.
- Quedan fuera del MVP: anime con numeración absoluta, episodios repartidos en varios archivos, bibliotecas adicionales (anime, documentales, conciertos), usuarios y roles, y extras como tráileres. La lista completa está en el §8 de la spec.

## Estado

El repositorio contiene por ahora la base de la API: configuración, SQLite, el contrato OpenAPI con su código generado, el acceso con contraseña y sesiones, los diagnósticos de arranque de `GET /api/health` (las carpetas de biblioteca y datos deben poder escribirse, y `7zz` y `ffprobe` deben arrancar), la aplicación web incrustada, las herramientas y la CI. Las funciones de arriba se construyen fase a fase; el avance está en [`docs/tasks.md`](docs/tasks.md).

Próximamente: una demo sin backend, la instalación como Custom App de TrueNAS SCALE y capturas de pantalla.

## Stack

| | |
|---|---|
| Backend | Go, `net/http`, `log/slog`. SQLite (modernc, goose, sqlc). 7-Zip y ffprobe se ejecutan como procesos hijos |
| Frontend | React, Vite, TypeScript (estricto), i18next |
| Contrato | OpenAPI 3, spec-first ([`api/openapi.yaml`](api/openapi.yaml)). Go y TypeScript comparten vectores dorados de prueba en [`contracts/`](contracts/README.md) |
| Calidad | Tests de Go con el detector de carreras, golangci-lint v2, Vitest y Testing Library, ESLint, Prettier, Redocly y govulncheck en la CI |

## Arquitectura

Backend y frontend siguen Domain-Driven Design: cada contexto acotado tiene paquetes `domain` puros y se comunica con el exterior mediante puertos y adaptadores.

| Contexto | Responsabilidad |
|---|---|
| `catalog` | Biblioteca, sección No asignados y papelera. Todo cambio en la biblioteca pasa por un motor de operaciones con journal |
| `ingestion` | Subidas, descompresión, filtrado, análisis de nombres de archivo y sondeo |
| `metadata` | Cliente de TMDB, detrás de una capa anticorrupción |
| `mediaservers` | Refresco de Jellyfin |
| `identity` | Acceso con contraseña única y sesiones |
| `platform` | Núcleo compartido: configuración, servidor HTTP, registro de eventos |

Algunos principios se cumplen en todo el código: los archivos se transmiten en streaming y nunca se cargan en memoria, las rutas se construyen en el servidor a partir de IDs, y los secretos de TMDB y Jellyfin nunca llegan al navegador. La estructura es `apps/api` (Go), `apps/web` (React), `api/` (OpenAPI) y `contracts/` (vectores dorados). El diseño detallado está en la spec.

## Requisitos

- **Docker**. Las herramientas de Go corren en contenedores, así que no hace falta tener Go instalado.
- **Node.js 24** o posterior, y **pnpm** (la versión está fijada en `package.json`; `corepack enable` lo instala).
- **[Task](https://taskfile.dev)** 3.

## Desarrollo

```bash
task install       # dependencias de JS y módulos de Go
task dev:api       # API de Go en Docker, http://localhost:8080
task dev:web       # servidor de Vite, http://localhost:5173 (hace de proxy de /api)
task test          # tests de Go y del frontend
task lint          # lint de OpenAPI, Go y frontend
task check         # todo lo que ejecuta la CI
```

Otras tareas: `task gen` (regenera el código desde `api/openapi.yaml` y las consultas SQL; la CI falla si está desactualizado), `task fmt` (formateadores), `task build` (la SPA incrustada en el binario de Linux; `task build:api` y `task build:web` construyen cada parte), `task tidy` (`go mod tidy`), `task demo-catalog` (regenera el catálogo de la demo desde TMDB: solo estructura, con las sinopsis del proyecto en `apps/api/cmd/demo-catalog/synopses.json`; necesita `TMDB_READ_TOKEN` o `TMDB_API_KEY` en `.env.local`). `task --list` las muestra todas. Para comprobar la API: `curl http://localhost:8080/api/health`.

`task dev:image` construye la imagen de desarrollo (`deploy/dev/Dockerfile`): el toolchain de Go más el 7-Zip oficial (con RAR) y ffprobe, ambos en versiones fijadas y con SHA-256 comprobado. Todas las tareas de Go dependen de ella, así que la primera que se ejecuta la construye. La CI instala los mismos dos binarios.

`task dev:api` pasa al contenedor todas las variables de `.env.local` (el archivo es opcional, pero sin `SESSION_SECRET` y una contraseña la API no arranca). `LIBRARY_PATH` y `DATA_PATH` valen siempre `./.dev/library` y `./.dev/data` en desarrollo, digan lo que digan `.env.local`; `task dev:api` crea las dos carpetas si faltan.

## Configuración

Todas las variables de entorno están en [`.env.example`](.env.example), con su valor por defecto cuando lo tienen. Cópialo a `.env.local` para sobrescribir valores en local, y nunca subas valores reales.

El acceso usa una única contraseña. Define `APP_PASSWORD_HASH` con un hash argon2id, que imprime `echo 'tu contraseña' | task hash-password` (o `APP_PASSWORD` en texto plano), y `SESSION_SECRET` (al menos 32 bytes) para firmar la cookie de sesión. En `.env.local`, pon el hash entre comillas simples (`APP_PASSWORD_HASH='$argon2id$…'`): Compose lee un `$` sin comillas como variable y perdería parte del hash. Toda operación de la API salvo `GET /api/health` y `POST /api/auth/login` exige sesión; sin ella la API responde `401` como problema RFC 9457.

## Atribución y aviso legal

Este producto usa la API de TMDB pero no está avalado ni certificado por TMDB.

VideoExplorer no incluye, descarga ni distribuye películas ni series. Úsalo solo con archivos que tengas derecho a conservar.

Licencia [MIT](LICENSE).
