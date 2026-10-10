# Logotipos de atribución de TMDB

Logotipos oficiales de TMDB (The Movie Database) para la atribución que exige TMDB (RNF-10 de `docs/spec.md`). Son marcas de TMDB y se usan **solo** para esa atribución.

| Archivo | Nombre en la guía de TMDB | Origen | Descarga |
|---|---|---|---|
| `tmdb-primary-short-blue.svg` | Primary short (blue) | `https://www.themoviedb.org/assets/v4/logos/v2/blue_short-8e7b30f73a4020692ccca9c88bafe5dcb6f8a62a4c6bc55cd9ba82bb2cd95f6c.svg` | 2026-10-10 |
| `tmdb-primary-long-blue.svg` | Primary long (blue) | `https://www.themoviedb.org/assets/v4/logos/v2/blue_long_1-8ba2ac31f354005783fab473602c34c3f4fd207150182061e425d366e4f34596.svg` | 2026-10-10 |

Guía de atribución de TMDB: `https://www.themoviedb.org/about/logos-attribution`.

## Reglas

- **No se modifican.** Son copias byte a byte de los archivos oficiales: ni colores, ni proporciones, ni texto. El SHA-256 de cada archivo coincide con el que TMDB incluye en el nombre de su URL. Si hace falta actualizarlos, se vuelven a descargar de la guía y se comprueba ese hash.
- **Colores del archivo:** degradado `#00b3e5`, `#3cbec9` y `#90cea1`. Se usan tal cual.
- **Dónde aparece (RNF-10, §6, RF-62, RF-71):** la atribución (texto y logo) se muestra en Ajustes solo si TMDB está configurado (`TMDB_API_KEY` o `TMDB_READ_TOKEN`), y en la demo (`VITE_DATA_SOURCE=demo`), porque sus datos derivan de TMDB.
- **Texto:** la clave i18n `attribution.tmdb` (en `shared/i18n/en.json` y `es.json`). La frase es la exacta de RNF-10 en inglés y se muestra igual en las dos lenguas, también con la interfaz en español (decisión 2026-10-10): «This product uses the TMDB API but is not endorsed or certified by TMDB.».
- **Sin peticiones a terceros:** los logos se empaquetan con la app, así que el navegador no contacta a TMDB para mostrarlos (RNF-10).

La constante tipada que los exporta está en `index.ts`.
