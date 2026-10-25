#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

SERVICE="${SERVICE:-ordenes-api-cluster}"
NETWORK="${NETWORK:-iiss-net}"
IMAGE="${IMAGE:-ordenes-api:lab5}"
COMPOSE_CMD=(docker compose -f docker-compose.yml -f scripts/docker-compose.cluster.yml)

BORRAR_DATOS=0
BORRAR_IMAGENES=0
BORRAR_LOGS=0
SALIR_SWARM=0

log() { printf '\n==> %s\n' "$*"; }
ok() { printf '[OK] %s\n' "$*"; }
warn() { printf '[AVISO] %s\n' "$*"; }
fail() { printf '[ERROR] %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Uso: ./scripts/bajar-cluster.sh [opciones]

Sin opciones baja el servicio, los contenedores y la red, pero conserva
los volúmenes con datos, las imágenes y el modo Swarm.

Opciones:
  --volumes    Borra los volúmenes de datos del proyecto (pierde la BD)
  --images     Borra ordenes-api:lab5 y las imágenes iiss-2-lab-*
  --logs       Borra el directorio logs/
  --swarm      Sale de Docker Swarm (docker swarm leave --force)
  --all        Equivale a --volumes --images --logs
  -h, --help   Muestra esta ayuda
EOF
}

for arg in "$@"; do
  case "$arg" in
    --volumes) BORRAR_DATOS=1 ;;
    --images) BORRAR_IMAGENES=1 ;;
    --logs) BORRAR_LOGS=1 ;;
    --swarm) SALIR_SWARM=1 ;;
    --all)
      BORRAR_DATOS=1
      BORRAR_IMAGENES=1
      BORRAR_LOGS=1
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      printf '[ERROR] Opción desconocida: %s\n' "$arg" >&2
      usage
      exit 1
      ;;
  esac
done

command -v docker >/dev/null 2>&1 || fail "Docker no está instalado"
docker info >/dev/null 2>&1 || fail "El daemon de Docker no está corriendo"

SWARM_STATE="$(docker info --format '{{.Swarm.LocalNodeState}}')"
CONTROL_AVAILABLE="$(docker info --format '{{.Swarm.ControlAvailable}}')"

if [ "$SWARM_STATE" = "active" ] && [ "$CONTROL_AVAILABLE" = "true" ]; then
  if docker service inspect "$SERVICE" >/dev/null 2>&1; then
    log "Eliminando servicio Swarm $SERVICE..."
    docker service rm "$SERVICE" >/dev/null
    elapsed=0
    while docker service inspect "$SERVICE" >/dev/null 2>&1; do
      if [ "$elapsed" -ge 60 ]; then
        fail "El servicio $SERVICE no terminó de eliminarse"
      fi
      sleep 1
      elapsed=$((elapsed + 1))
    done
    ok "Servicio $SERVICE eliminado"
  else
    ok "El servicio $SERVICE no existe"
  fi
else
  warn "Swarm no está activo en este nodo; se omite la eliminación del servicio"
fi

log "Bajando contenedores del proyecto con Docker Compose..."
if [ "$BORRAR_DATOS" = "1" ]; then
  "${COMPOSE_CMD[@]}" down --remove-orphans --volumes
  ok "Contenedores, red y volúmenes del proyecto eliminados"
else
  "${COMPOSE_CMD[@]}" down --remove-orphans
  ok "Contenedores y red del proyecto eliminados (volúmenes conservados)"
fi

if docker network inspect "$NETWORK" >/dev/null 2>&1; then
  log "Eliminando red overlay $NETWORK..."
  if docker network rm "$NETWORK" >/dev/null 2>&1; then
    ok "Red $NETWORK eliminada"
  else
    warn "No se pudo eliminar la red $NETWORK; puede seguir en uso"
  fi
else
  ok "La red $NETWORK no existe"
fi

if [ "$BORRAR_IMAGENES" = "1" ]; then
  log "Eliminando imágenes del proyecto..."
  docker image rm -f "$IMAGE" >/dev/null 2>&1 || true
  while IFS= read -r image; do
    [ -z "$image" ] && continue
    docker image rm -f "$image" >/dev/null 2>&1 || true
  done < <(docker images --format '{{.Repository}}:{{.Tag}}' | grep '^iiss-2-lab-' || true)
  ok "Imágenes eliminadas"
fi

if [ "$BORRAR_LOGS" = "1" ]; then
  log "Eliminando logs de evidencia..."
  rm -rf "$ROOT_DIR/logs"
  ok "Directorio logs/ eliminado"
fi

if [ "$SALIR_SWARM" = "1" ]; then
  if [ "$SWARM_STATE" = "active" ]; then
    log "Saliendo de Docker Swarm..."
    docker swarm leave --force >/dev/null
    ok "Este nodo salió de Swarm"
  else
    ok "Este nodo ya no está en Swarm"
  fi
fi

printf '\n=== Limpieza finalizada ===\n'
if [ "$BORRAR_DATOS" = "0" ]; then
  printf 'Los volúmenes con datos se conservaron.\n'
fi
if [ "$SALIR_SWARM" = "0" ] && [ "$SWARM_STATE" = "active" ]; then
  printf 'Docker Swarm sigue activo. Para salir: ./scripts/bajar-cluster.sh --swarm\n'
fi
