#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

IMAGE="${IMAGE:-ordenes-api:lab5}"
SERVICE="${SERVICE:-ordenes-api-cluster}"
REPLICAS="${REPLICAS:-3}"
PORT="${PORT:-8081}"
NETWORK="${NETWORK:-iiss-net}"
COMPOSE_SERVICES=(productos-db productos-api ordenes-db mosquitto)
DEPENDENCY_CONTAINERS=(productos-db productos-api ordenes-db mosquitto)
COMPOSE_CMD=(docker compose -f docker-compose.yml -f scripts/docker-compose.cluster.yml)

log() { printf '\n==> %s\n' "$*"; }
ok() { printf '[OK] %s\n' "$*"; }
fail() { printf '[ERROR] %s\n' "$*" >&2; exit 1; }

wait_health() {
  local container="$1"
  local timeout="${2:-120}"
  local elapsed=0
  local status
  while [ "$elapsed" -lt "$timeout" ]; do
    status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container" 2>/dev/null || true)"
    if [ "$status" = "healthy" ] || [ "$status" = "running" ]; then
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  return 1
}

wait_http() {
  local url="$1"
  local timeout="${2:-120}"
  local elapsed=0
  while [ "$elapsed" -lt "$timeout" ]; do
    if curl -fsS -o /dev/null "$url" 2>/dev/null; then
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  return 1
}

wait_replicas() {
  local expected="$1"
  local timeout="${2:-180}"
  local elapsed=0
  local replicas
  while [ "$elapsed" -lt "$timeout" ]; do
    replicas="$(docker service ls --filter "name=$SERVICE" --format '{{.Name}}|{{.Replicas}}' | awk -F'|' -v s="$SERVICE" '$1 == s {print $2}')"
    if [ "$replicas" = "$expected/$expected" ]; then
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  return 1
}

command -v docker >/dev/null 2>&1 || fail "Docker no está instalado"
docker compose version >/dev/null 2>&1 || fail "Docker Compose no está disponible"
docker info >/dev/null 2>&1 || fail "El daemon de Docker no está corriendo"

SWARM_STATE="$(docker info --format '{{.Swarm.LocalNodeState}}')"
if [ "$SWARM_STATE" != "active" ]; then
  log "Inicializando Docker Swarm..."
  if ! docker swarm init >/dev/null 2>&1; then
    ADVERTISE_ADDR="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i <= NF; i++) if ($i == "src") {print $(i + 1); exit}}')"
    if [ -z "$ADVERTISE_ADDR" ]; then
      ADVERTISE_ADDR="$(hostname -I 2>/dev/null | awk '{for (i = 1; i <= NF; i++) if ($i ~ /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$/) {print $i; exit}}')"
    fi
    [ -n "$ADVERTISE_ADDR" ] || fail "No se pudo determinar una IPv4 para inicializar Swarm"
    log "Reintentando con --advertise-addr $ADVERTISE_ADDR..."
    docker swarm init --advertise-addr "$ADVERTISE_ADDR" >/dev/null || fail "No se pudo inicializar Swarm"
  fi
  ok "Swarm inicializado"
else
  ok "Swarm ya está activo"
fi

if [ "$(docker info --format '{{.Swarm.ControlAvailable}}')" != "true" ]; then
  fail "Este nodo no es manager de Swarm; no puede crear servicios"
fi

log "Construyendo imagen $IMAGE..."
docker build -t "$IMAGE" ./ordenes-api
ok "Imagen construida"

log "Levantando dependencias con Docker Compose..."
"${COMPOSE_CMD[@]}" up -d "${COMPOSE_SERVICES[@]}"
ok "Contenedores de dependencias iniciados"

wait_health productos-db 180 || fail "productos-db no quedó saludable"
wait_health ordenes-db 180 || fail "ordenes-db no quedó saludable"
wait_health mosquitto 180 || fail "mosquitto no quedó saludable"
wait_http "http://localhost:8080/api/productos" 180 || fail "productos-api no respondió"
ok "Dependencias listas"

if ! docker network inspect "$NETWORK" >/dev/null 2>&1; then
  log "Creando red overlay $NETWORK..."
  docker network create --driver overlay --attachable "$NETWORK" >/dev/null
fi
ok "Red $NETWORK disponible"

for container in "${DEPENDENCY_CONTAINERS[@]}"; do
  if docker inspect --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}} {{end}}' "$container" 2>/dev/null | grep -q "$NETWORK"; then
    continue
  fi
  log "Conectando $container a $NETWORK..."
  docker network connect "$NETWORK" "$container"
done
ok "Dependencias conectadas a $NETWORK"

if docker service inspect "$SERVICE" >/dev/null 2>&1; then
  log "Eliminando servicio existente $SERVICE..."
  docker service rm "$SERVICE" >/dev/null
  elapsed=0
  while docker service inspect "$SERVICE" >/dev/null 2>&1; do
    if [ "$elapsed" -ge 30 ]; then
      fail "El servicio $SERVICE no se pudo eliminar"
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  sleep 2
  ok "Servicio anterior eliminado"
fi

if docker ps --format '{{.Names}}' | grep -qx 'ordenes-api'; then
  log "Deteniendo ordenes-api de Docker Compose para liberar el puerto $PORT..."
  "${COMPOSE_CMD[@]}" stop ordenes-api >/dev/null
  ok "ordenes-api de Compose detenido"
fi

PORT_OWNER=""
for ((i = 0; i < 30; i++)); do
  PORT_OWNER="$(docker ps --filter "publish=$PORT" --format '{{.Names}}' | head -n 1)"
  [ -z "$PORT_OWNER" ] && break
  sleep 1
done
if [ -n "$PORT_OWNER" ]; then
  fail "El puerto $PORT está ocupado por el contenedor '$PORT_OWNER'"
fi

log "Creando servicio $SERVICE con $REPLICAS réplicas..."
docker service create \
  --name "$SERVICE" \
  --replicas "$REPLICAS" \
  --network "$NETWORK" \
  --publish "published=$PORT,target=$PORT" \
  --env DB_HOST=ordenes-db \
  --env DB_PORT=3306 \
  --env DB_NAME=ordenes \
  --env DB_USER=user \
  --env DB_PASSWORD=password \
  --env PRODUCTOS_API_URL=http://productos-api:8080 \
  --env MQTT_BROKER_URL=tcp://mosquitto:1883 \
  --restart-condition any \
  --update-parallelism 1 \
  --update-delay 5s \
  "$IMAGE" >/dev/null

if ! wait_replicas "$REPLICAS" 180; then
  docker service ps "$SERVICE" || true
  fail "El servicio no alcanzó $REPLICAS/$REPLICAS réplicas"
fi

if ! wait_http "http://localhost:$PORT/api/ordenes" 180; then
  docker service ps "$SERVICE" || true
  fail "La API de órdenes no respondió en http://localhost:$PORT/api/ordenes"
fi

ok "Cluster listo con $REPLICAS réplicas"
docker service ls --filter "name=$SERVICE"
docker service ps "$SERVICE"
printf '\nEndpoint: http://localhost:%s/api/ordenes\n' "$PORT"
printf 'Logs:     docker service logs -f %s\n' "$SERVICE"
printf 'Prueba:   ./scripts/probar-balanceo.sh\n'
