#!/usr/bin/env bash
# Jellyfin local para el spike de nombres (fase 0.5, spec §5 y §9.1).
#
# Desechable: no forma parte de la app. Levanta un Jellyfin con la misma versión
# que el NAS del usuario (12.2) en el contenedor ve-spike-jellyfin. La biblioteca
# generada por make-library.sh se monta en /media en solo lectura; la configuración
# y la caché viven en .dev/spike-jellyfin/ (ignorada por git).
#
# Uso (Git Bash, con Docker en marcha):
#   bash scripts/spike/jellyfin.sh up       crea o arranca el contenedor y espera a que responda
#   bash scripts/spike/jellyfin.sh down     para el contenedor y lo elimina
#   bash scripts/spike/jellyfin.sh reset    down + borra la configuración y la caché
#   bash scripts/spike/jellyfin.sh status   muestra el estado del contenedor
# Equivalentes: task spike:jellyfin:up, spike:jellyfin:down, spike:jellyfin:reset.
set -euo pipefail

# Git Bash no debe reescribir las rutas que pasan a Docker (p. ej. /media).
export MSYS_NO_PATHCONV=1

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SPIKE_DIR="$ROOT/.dev"
LIBRARY="$SPIKE_DIR/spike-library"
STATE="$SPIKE_DIR/spike-jellyfin"
CONFIG="$STATE/config"
CACHE="$STATE/cache"

NAME='ve-spike-jellyfin'
# Misma versión que el NAS del usuario (decisión 2026-10-10).
IMAGE='jellyfin/jellyfin:12.2.20261005-225228'
PORT=8096
# Segundos máximos que `up` espera a que Jellyfin responda (el primer arranque es lento).
WAIT_SECONDS="${JELLYFIN_WAIT_SECONDS:-180}"

# Ruta del host en el formato que entiende Docker (C:/... en Windows).
host_path() {
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -m "$1"
  else
    printf '%s' "$1"
  fi
}

require_docker() {
  if ! docker info >/dev/null 2>&1; then
    echo "Docker no responde. Arranca Docker Desktop y vuelve a intentarlo." >&2
    exit 1
  fi
}

exists() {
  docker container inspect "$NAME" >/dev/null 2>&1
}

running() {
  [[ "$(docker container inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null || true)" == true ]]
}

# health_ok: 0 si /health responde «Healthy» en el puerto publicado.
health_ok() {
  [[ "$(curl -fsS --max-time 5 "http://127.0.0.1:${PORT}/health" 2>/dev/null || true)" == Healthy ]]
}

wait_healthy() {
  local deadline=$((SECONDS + WAIT_SECONDS))
  until health_ok; do
    if ! running; then
      echo "El contenedor $NAME se ha parado. Revisa: docker logs $NAME" >&2
      exit 1
    fi
    if ((SECONDS >= deadline)); then
      echo "Jellyfin no respondió «Healthy» en ${WAIT_SECONDS} s. Revisa: docker logs $NAME" >&2
      exit 1
    fi
    sleep 2
  done
}

cmd_up() {
  if [[ ! -d "$LIBRARY" ]]; then
    echo "No existe $LIBRARY. Genérala antes con: bash scripts/spike/make-library.sh" >&2
    exit 1
  fi
  require_docker

  if running; then
    echo "$NAME ya está en marcha."
  elif exists; then
    docker start "$NAME" >/dev/null
  else
    mkdir -p "$CONFIG" "$CACHE"
    # Sin GPU ni hardware, usuario y zona horaria por defecto de la imagen oficial.
    # Solo el puerto 8096 de este equipo; la biblioteca, en solo lectura.
    docker run -d --name "$NAME" \
      -p "127.0.0.1:${PORT}:8096" \
      -v "$(host_path "$CONFIG"):/config" \
      -v "$(host_path "$CACHE"):/cache" \
      -v "$(host_path "$LIBRARY"):/media:ro" \
      "$IMAGE" >/dev/null
  fi

  echo "Esperando a Jellyfin (hasta ${WAIT_SECONDS} s)..."
  wait_healthy
  echo "Jellyfin listo: http://localhost:${PORT}"
}

cmd_down() {
  require_docker
  if running; then
    docker stop "$NAME" >/dev/null
  fi
  if exists; then
    docker rm "$NAME" >/dev/null
    echo "$NAME parado y eliminado."
  else
    echo "$NAME no existe."
  fi
}

cmd_reset() {
  cmd_down
  rm -rf "$STATE"
  echo "Configuración y caché borradas: $STATE"
}

cmd_status() {
  require_docker
  if running; then
    echo "$NAME: en marcha ($IMAGE)."
    if health_ok; then
      echo "Health: Healthy en http://localhost:${PORT}"
    else
      echo "Health: no responde todavía en http://localhost:${PORT}"
    fi
  elif exists; then
    echo "$NAME: parado."
  else
    echo "$NAME: no existe."
  fi
}

case "${1:-}" in
  up) cmd_up ;;
  down) cmd_down ;;
  reset) cmd_reset ;;
  status) cmd_status ;;
  *)
    echo "Uso: bash scripts/spike/jellyfin.sh up|down|reset|status" >&2
    exit 2
    ;;
esac
