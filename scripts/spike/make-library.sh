#!/usr/bin/env bash
# Spike de nombres en Jellyfin (fase 0.5, spec §5 y §9.1).
#
# Desechable: no forma parte de la app. Genera una biblioteca ficticia en
# .dev/spike-library/ (Movies/, TV Shows/ y CASES.md). Se borra y se recrea en
# cada ejecución. Los videos son válidos (5 s, negro y en silencio) y se crean
# con ffmpeg de una imagen de Docker con versión y digest fijados.
#
# Uso (Git Bash, con Docker en marcha): task spike:library
set -euo pipefail

# Git Bash no debe reescribir las rutas que pasan a Docker (p. ej. /out).
export MSYS_NO_PATHCONV=1

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SPIKE_DIR="$ROOT/.dev"
LIBRARY="$SPIKE_DIR/spike-library"
TEMPLATES="$SPIKE_DIR/spike-templates"
# ffmpeg y ffprobe 8.1.2 (jrottenberg/ffmpeg, Ubuntu 24.04), fijados por digest.
FFMPEG_IMAGE='jrottenberg/ffmpeg:8.1.2-ubuntu2404@sha256:73b5e69d428ecd4b97c2a8085755ecbe2c56f244a38917ed710bb98597e65d8f'

# Ruta del host en el formato que entiende Docker (C:/... en Windows).
host_path() {
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -m "$1"
  else
    printf '%s' "$1"
  fi
}

# make_template <nombre> <ancho> <alto> <códec de vídeo> [opciones extra]
# Genera una plantilla de 5 s (vídeo negro, audio en silencio). Cada combinación
# se genera una vez; los casos copian la plantilla.
make_template() {
  local name=$1 width=$2 height=$3 vcodec=$4
  shift 4
  docker run --rm -v "$(host_path "$TEMPLATES"):/out" --entrypoint ffmpeg "$FFMPEG_IMAGE" \
    -hide_banner -loglevel error -y \
    -f lavfi -i "color=c=black:s=${width}x${height}:r=24:d=5" \
    -f lavfi -i "anullsrc=channel_layout=stereo:sample_rate=48000" \
    -t 5 -c:v "$vcodec" -preset veryfast -crf 40 -pix_fmt yuv420p "$@" \
    -c:a aac -b:a 64k "/out/$name.mkv" \
    2> >(grep -v 'set_mempolicy: Operation not permitted' >&2)
}

rm -rf "$TEMPLATES"
mkdir -p "$TEMPLATES"
make_template h264-1080 1920 1080 libx264
make_template h264-2160 3840 2160 libx264
make_template hevc-1080 1920 1080 libx265 -x265-params log-level=error

rm -rf "$LIBRARY"
mkdir -p "$LIBRARY"

cat > "$LIBRARY/CASES.md" <<'HEADER'
# Spike de nombres en Jellyfin: casos

Biblioteca ficticia para la fase 0.5 (spec §5, §9.1). Los videos son de 5 s,
negros y sin sonido; solo importa el nombre, la resolución y el códec.

Cómo usarla:

1. Copia la carpeta `spike-library` completa (con `Movies/` y `TV Shows/`) por SMB a
   una carpeta NUEVA del NAS, fuera de tu biblioteca real (p. ej. `videoexplorer-spike/`).
   No la mezcles con tus `Movies/` ni `TV Shows/` reales.
2. En Jellyfin, crea dos bibliotecas NUEVAS: Películas sobre `.../videoexplorer-spike/Movies`
   y Series sobre `.../videoexplorer-spike/TV Shows`. Deja activados los metadatos de TMDB
   y desactiva el guardado de `.nfo` e imágenes en la carpeta de medios, para no ensuciarla.
   Lanza el escaneo.
3. Marca cada fila en la columna «Resultado» (`[x]` si coincide con lo esperado)
   o anota lo que muestra Jellyfin.
4. Al terminar, borra en Jellyfin esas dos bibliotecas y la carpeta
   `videoexplorer-spike/` del NAS.

Resoluciones reales: `h264-1080` 1920x1080 H.264, `h264-2160` 3840x2160 H.264,
`hevc-1080` 1920x1080 HEVC. Las versiones etiquetadas `2160p` son `h264-2160` y
las `1080p HEVC` son `hevc-1080`; el resto es `h264-1080`.
HEADER

# Casos: ruta relativa a la biblioteca | tipo | qué comprueba | qué debería mostrar Jellyfin
# tipo: video:<plantilla> (h264-1080, h264-2160, hevc-1080) o srt:<texto de la línea>.
# Las líneas que empiezan por "## " son secciones de CASES.md.
n=0
while IFS='|' read -r rel kind check expect; do
  [[ -z "$rel" || "$rel" == \#\ * ]] && continue
  if [[ "$rel" == '## '* ]]; then
    printf '\n%s\n\n| # | Ruta | Qué comprueba | Qué debería mostrar Jellyfin | Resultado |\n|---|---|---|---|---|\n' "$rel" >> "$LIBRARY/CASES.md"
    continue
  fi
  n=$((n + 1))
  mkdir -p "$(dirname "$LIBRARY/$rel")"
  case "$kind" in
    video:*) cp "$TEMPLATES/${kind#video:}.mkv" "$LIBRARY/$rel" ;;
    srt:*) printf '1\n00:00:00,000 --> 00:00:04,000\n%s\n' "${kind#srt:}" > "$LIBRARY/$rel" ;;
    *) echo "Tipo desconocido en el caso $n: $kind" >&2; exit 1 ;;
  esac
  printf '| %d | `%s` | %s | %s | [ ] |\n' "$n" "$rel" "$check" "$expect" >> "$LIBRARY/CASES.md"
done <<'CASES'
## Películas: película simple y subtítulos
Movies/Iron Man (2008)/Iron Man (2008).mkv|video:h264-1080|Película simple, sin etiqueta de ID ni versiones|Una película «Iron Man» (2008) con póster de TMDB y una sola versión.
Movies/Iron Man (2008)/Iron Man (2008).es.srt|srt:Iron Man - subtítulo de prueba: español (es)|Subtítulo español sin región|Pista externa «Español», asociada a esta película.
Movies/Iron Man (2008)/Iron Man (2008).es-MX.srt|srt:Iron Man - subtítulo de prueba: español de México (es-MX)|Subtítulo con región (es-MX)|Pista «Español» de México (o «Español» sin región). Anotar cómo lo presenta Jellyfin.
Movies/Iron Man (2008)/Iron Man (2008).en.forced.srt|srt:Iron Man - subtítulo de prueba: inglés forzado (en, forced)|Marca forzado (.forced)|Pista «Inglés» marcada como forzada.
Movies/Iron Man (2008)/Iron Man (2008).en.sdh.srt|srt:Iron Man - subtítulo de prueba: inglés SDH (en, sdh)|Marca SDH (.sdh)|Pista «Inglés» marcada como SDH (para personas con discapacidad auditiva). Anotar si Jellyfin la ignora.
Movies/Iron Man (2008)/Iron Man (2008).pt-BR.srt|srt:Iron Man - subtítulo de prueba: portugués de Brasil (pt-BR)|Idioma con región (pt-BR)|Pista «Portugués» (Brasil). Anotar cómo la presenta.
## Películas: versiones
Movies/Dune (2021)/Dune (2021) - 2160p.mkv|video:h264-2160|Versión 2160p (vídeo real de 3840x2160)|Dos versiones en la misma película (selector de versión), con la etiqueta «2160p».
Movies/Dune (2021)/Dune (2021) - 1080p HEVC.mkv|video:hevc-1080|Versión con códec HEVC (vídeo real HEVC 1920x1080)|Segunda versión «1080p HEVC». En la información técnica, códec HEVC.
Movies/Dune (2021)/Dune (2021) - 2160p.es.srt|srt:Dune - subtítulo de prueba: español (es), versión 2160p|Subtítulo ligado a una versión|Aparece solo con la versión 2160p, no con la 1080p HEVC.
## Películas: versiones con etiqueta de ID en los archivos
Movies/Blade Runner 2049 (2017) [tmdbid-335984]/Blade Runner 2049 (2017) [tmdbid-335984] - 2160p.mkv|video:h264-2160|Etiqueta de ID en carpeta y en archivos de versiones (§9.1 solo cubre la carpeta)|Una sola película (TMDB 335984) con dos versiones. Si no se agrupan, es el hallazgo principal del spike.
Movies/Blade Runner 2049 (2017) [tmdbid-335984]/Blade Runner 2049 (2017) [tmdbid-335984] - 1080p.mkv|video:h264-1080|Segunda versión con la misma etiqueta|Igual que la anterior, con las versiones «2160p» y «1080p».
## Películas: partes
Movies/Gone with the Wind (1939)/Gone with the Wind (1939) - part1.mkv|video:h264-1080|Parte 1 de una película|Una película con dos partes que se reproducen en orden (no dos películas ni versiones).
Movies/Gone with the Wind (1939)/Gone with the Wind (1939) - part2.mkv|video:h264-1080|Parte 2 de la misma película|La parte 2 sigue a la 1 dentro de la misma película.
Movies/Gone with the Wind (1939)/Gone with the Wind (1939) - part1.es.srt|srt:Gone with the Wind - subtítulo de prueba: español (es), parte 1|Subtítulo de una parte|Subtítulo asociado solo a la parte 1.
## Películas: títulos saneados, no latinos y remakes
Movies/Mission - Impossible (1996)/Mission - Impossible (1996).mkv|video:h264-1080|Título saneado: en TMDB «Mission: Impossible», con ': ' convertido en ' - '|Coincide con TMDB (1996) pese a los dos puntos del título original. El nombre de carpeta no lleva dos puntos.
Movies/Le Fabuleux Destin d'Amélie Poulain (2001)/Le Fabuleux Destin d'Amélie Poulain (2001).mkv|video:h264-1080|Título original latino con acento y apóstrofo|Coincide con TMDB (2001) y muestra el título sin caracteres rotos.
Movies/Spirited Away (2001)/Spirited Away (2001).mkv|video:h264-1080|Título original no latino (japonés): en el disco va en inglés (§5)|Coincide con TMDB (2001) y muestra «Spirited Away».
Movies/The Lion King (1994)/The Lion King (1994).mkv|video:h264-1080|Remake: mismo nombre, año 1994|Película separada de la de 2019, con su propia ficha. Jellyfin no debe fusionarlas.
Movies/The Lion King (2019)/The Lion King (2019).mkv|video:h264-1080|Remake: mismo nombre, año 2019|Película separada de la de 1994, con su propia ficha.
## Series: especiales
TV Shows/Breaking Bad (2008)/Season 00/Breaking Bad (2008) - S00E01 - Good Cop Bad Cop.mkv|video:h264-1080|Especial (temporada 0)|Aparece en «Especiales» (Season 00), fuera de la temporada 1.
## Series: capítulos, subtítulos y versiones
TV Shows/Breaking Bad (2008)/Season 01/Breaking Bad (2008) - S01E01 - Pilot.mkv|video:h264-1080|Capítulo simple|Episodio 1 de la temporada 1, «Pilot».
TV Shows/Breaking Bad (2008)/Season 01/Breaking Bad (2008) - S01E01 - Pilot.en.forced.srt|srt:Breaking Bad S01E01 - subtítulo de prueba: inglés forzado (en, forced)|Subtítulo forzado de capítulo|Pista «Inglés» forzada en el episodio 1.
TV Shows/Breaking Bad (2008)/Season 01/Breaking Bad (2008) - S01E01 - Pilot.es-MX.srt|srt:Breaking Bad S01E01 - subtítulo de prueba: español de México (es-MX)|Subtítulo de capítulo con región|Pista «Español» de México en el episodio 1.
TV Shows/Breaking Bad (2008)/Season 01/Breaking Bad (2008) - S01E02 - Cat's in the Bag... - 2160p.mkv|video:h264-2160|Versión de capítulo con puntos dentro del título|Un episodio (S01E02) con dos versiones. Anotar si Jellyfin las muestra como dos episodios.
TV Shows/Breaking Bad (2008)/Season 01/Breaking Bad (2008) - S01E02 - Cat's in the Bag... - 1080p.mkv|video:h264-1080|Segunda versión del mismo capítulo|Igual que la anterior, con las versiones «2160p» y «1080p».
TV Shows/Breaking Bad (2008)/Season 01/Breaking Bad (2008) - S01E03-E04 - ...And the Bag's in the River + Cancer Man.mkv|video:h264-1080|Un archivo con dos capítulos consecutivos|Dos episodios (3 y 4) desde un solo archivo, con los títulos unidos por ' + '.
## Series: títulos colapsados, identificación y nombres especiales
TV Shows/Lost (2004)/Season 01/Lost (2004) - S01E01-E02 - Pilot.mkv|video:h264-1080|Dos capítulos cuyos títulos (Pilot (1), Pilot (2)) se colapsan en «Pilot»|Dos episodios (1 y 2) desde un archivo, con el título «Pilot».
TV Shows/Lost (2004)/Season 01/Lost (2004) - S01E03 - Tabula Rasa.mkv|video:h264-1080|Capítulo simple|Episodio 3, «Tabula Rasa».
TV Shows/The Office (2005) [tmdbid-2316]/Season 01/The Office (2005) - S01E01 - Pilot.mkv|video:h264-1080|Etiqueta de ID en la carpeta de la serie|Serie vinculada a TMDB 2316, sin depender del nombre.
TV Shows/The Office (2001)/Season 01/The Office (2001) - S01E01 - Downsize.mkv|video:h264-1080|Remake con el mismo nombre, desambiguado por año|Serie separada de la de 2005 (versión británica), con su propia ficha.
TV Shows/Squid Game (2021)/Season 01/Squid Game (2021) - S01E01 - Red Light, Green Light.mkv|video:h264-1080|Coma en el título del capítulo|Título «Red Light, Green Light» íntegro.
TV Shows/What If... (2021)/Season 01/What If... (2021) - S01E01 - What If... Captain Carter Were the First Avenger.mkv|video:h264-1080|Título de serie que termina en puntos (What If...?) y título de capítulo con puntos|Serie «What If...» (2021). Los puntos se conservan (§5).
CASES

echo "Biblioteca generada en $LIBRARY: $n archivos."
echo "Tamaño: $(du -sh "$LIBRARY" | cut -f1)."
echo "Casos y resultados esperados: $LIBRARY/CASES.md"
