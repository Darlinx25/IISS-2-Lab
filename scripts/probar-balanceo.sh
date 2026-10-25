#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

SERVICE="${SERVICE:-ordenes-api-cluster}"
BASE_URL="${BASE_URL:-http://localhost:8081}"
PRODUCTOS_URL="${PRODUCTOS_URL:-http://localhost:8080}"
REQUESTS="${REQUESTS:-15}"
LOG_DIR="${LOG_DIR:-logs}"
RESTORE="${RESTORE:-1}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-180}"
RUN_ID="$(date +%Y%m%d%H%M%S)-$$"
TMP_DIR="$(mktemp -d)"
FAILED=0

cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT

log() { printf '\n==> %s\n' "$*"; }
ok() { printf '[OK] %s\n' "$*"; }
warn() { printf '[AVISO] %s\n' "$*"; }
mark_fail() { printf '[FALLO] %s\n' "$*"; FAILED=1; }
die() { printf '[ERROR] %s\n' "$*" >&2; exit 1; }

running_tasks() {
  docker service ps "$SERVICE" \
    --filter desired-state=running \
    --format '{{.ID}}|{{.Name}}|{{.CurrentState}}' \
    | awk -F'|' '$3 ~ /^Running/ {print $1"|"$2}'
}

task_container_id() {
  docker inspect --format '{{.Status.ContainerStatus.ContainerID}}' "$1"
}

count_posts() {
  local cid="$1"
  docker logs "$cid" 2>&1 | grep -cE 'POST[[:space:]]+"/api/ordenes"' || true
}

wait_running_count() {
  local expected="$1"
  local elapsed=0
  local count
  local replicas
  while [ "$elapsed" -lt "$WAIT_TIMEOUT" ]; do
    count="$(running_tasks | wc -l | tr -d ' ')"
    replicas="$(docker service ls --filter "name=$SERVICE" --format '{{.Name}}|{{.Replicas}}' | awk -F'|' -v s="$SERVICE" '$1 == s {print $2}')"
    if [ "$count" -eq "$expected" ] && [ "$replicas" = "$expected/$expected" ]; then
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  return 1
}

save_logs() {
  local phase="$1"
  local dir="$LOG_DIR/$phase"
  local task_id
  local task_name
  local cid
  mkdir -p "$dir"
  while IFS='|' read -r task_id task_name; do
    [ -z "$task_id" ] && continue
    cid="$(task_container_id "$task_id")"
    docker logs "$cid" > "$dir/${task_name}.log" 2>&1 || true
  done < <(running_tasks)
  docker service logs "$SERVICE" > "$dir/servicio.log" 2>&1 || true
}

send_requests() {
  local phase="$1"
  local total="$2"
  local i
  local code
  local response
  local body
  local ok_count=0
  log "Enviando $total requests a POST /api/ordenes ($phase)..."
  for ((i = 1; i <= total; i++)); do
    body="{\"email\":\"cluster-${RUN_ID}-${i}@test.local\",\"direccionEnvio\":\"Calle Cluster ${i}, Montevideo\",\"telefono\":\"+5989900${i}\",\"productos\":[{\"productoId\":${PRODUCT_ID},\"cantidad\":1}]}"
    code="$(curl -sS -o "$TMP_DIR/response.json" -w '%{http_code}' -X POST "$BASE_URL/api/ordenes" -H 'Content-Type: application/json' -d "$body" || true)"
    if [ "$code" = "201" ]; then
      ok_count=$((ok_count + 1))
    else
      response="$(cat "$TMP_DIR/response.json" 2>/dev/null || true)"
      warn "Request $i devolvió HTTP ${code:-sin respuesta}: $response"
    fi
  done
  printf 'Requests exitosos: %d/%d\n' "$ok_count" "$total"
  if [ "$ok_count" -ne "$total" ]; then
    mark_fail "No todos los requests de la fase '$phase' devolvieron HTTP 201"
  fi
}

run_phase() {
  local label="$1"
  local expected="$2"
  local total="$3"
  local -A base=()
  local -A cid_by_id=()
  local -A name_by_id=()
  local task_id
  local task_name
  local cid
  local after
  local before_count
  local delta
  local total_delta=0
  local fail_before="$FAILED"

  log "Fase '$label' con $expected réplica(s) activa(s)"
  if ! wait_running_count "$expected"; then
    mark_fail "No se alcanzaron $expected réplica(s) activas para la fase '$label'"
    return 0
  fi

  while IFS='|' read -r task_id task_name; do
    [ -z "$task_id" ] && continue
    cid="$(task_container_id "$task_id")"
    cid_by_id["$task_id"]="$cid"
    name_by_id["$task_id"]="$task_name"
    base["$task_id"]="$(count_posts "$cid")"
  done < <(running_tasks)

  if [ "${#base[@]}" -ne "$expected" ]; then
    mark_fail "Se esperaban $expected instancias pero se encontraron ${#base[@]}"
  fi

  send_requests "$label" "$total"
  sleep 1

  printf '\nVerificación de logs para la fase %s:\n' "$label"
  printf '  %-48s %8s\n' "Instancia" "Requests"
  for task_id in "${!base[@]}"; do
    cid="${cid_by_id[$task_id]}"
    after="$(count_posts "$cid")"
    before_count="${base[$task_id]}"
    delta=$((after - before_count))
    printf '  %-48s %8d\n' "${name_by_id[$task_id]}" "$delta"
    total_delta=$((total_delta + delta))
    if [ "$delta" -eq 0 ]; then
      mark_fail "La instancia ${name_by_id[$task_id]} no recibió ningún request en la fase '$label'"
    fi
  done
  printf '  %-48s %8d\n' "TOTAL" "$total_delta"

  if [ "$total_delta" -ne "$total" ]; then
    mark_fail "Se enviaron $total requests pero los logs registraron $total_delta en la fase '$label'"
  fi

  save_logs "$label"
  if [ "$FAILED" -eq "$fail_before" ]; then
    ok "Fase '$label' sin errores nuevos"
  else
    printf '[FALLO] Fase %s con errores\n' "$label"
  fi
  return 0
}

scale_to() {
  local target="$1"
  local before_tasks
  local after_tasks
  local before_names
  local after_names
  local removed
  local task_name
  local fail_before="$FAILED"

  log "Escalando servicio $SERVICE a $target réplica(s)..."
  save_logs "antes-de-escalar-a-${target}"
  before_tasks="$(running_tasks)"
  before_names="$(printf '%s\n' "$before_tasks" | cut -d'|' -f2 | sed '/^$/d' | sort)"

  docker service scale "$SERVICE=$target" >/dev/null
  if ! wait_running_count "$target"; then
    mark_fail "El servicio no convergió a $target réplica(s)"
    return 0
  fi

  after_tasks="$(running_tasks)"
  after_names="$(printf '%s\n' "$after_tasks" | cut -d'|' -f2 | sed '/^$/d' | sort)"
  removed="$(comm -23 <(printf '%s\n' "$before_names") <(printf '%s\n' "$after_names") | sed '/^$/d')"

  if [ -n "$removed" ]; then
    printf 'Instancia(s) dada(s) de baja:\n'
    while IFS= read -r task_name; do
      printf '  - %s\n' "$task_name"
    done <<< "$removed"
  fi

  if [ "$(running_tasks | wc -l | tr -d ' ')" -ne "$target" ]; then
    mark_fail "Quedaron tareas inesperadas después de escalar a $target"
  fi

  if [ "$FAILED" -eq "$fail_before" ]; then
    ok "Servicio escalado a $target réplica(s)"
  else
    printf '[FALLO] Servicio escalado a %s con errores\n' "$target"
  fi
  return 0
}

printf '=== Prueba de balanceo - Servicio %s ===\n' "$SERVICE"

log "Verificando precondiciones..."
command -v docker >/dev/null 2>&1 || die "Docker no está instalado"
command -v curl >/dev/null 2>&1 || die "curl no está instalado"
docker info >/dev/null 2>&1 || die "El daemon de Docker no está corriendo"
docker service inspect "$SERVICE" >/dev/null 2>&1 || die "No existe el servicio $SERVICE. Ejecutá primero ./scripts/levantar-cluster.sh"
curl -fsS -o /dev/null "$BASE_URL/api/ordenes" || die "La API de órdenes no responde en $BASE_URL"
curl -fsS -o /dev/null "$PRODUCTOS_URL/api/productos" || die "La API de productos no responde en $PRODUCTOS_URL"

log "Creando producto de prueba con stock alto..."
CREATE_RESPONSE="$(curl -sS -X POST "$PRODUCTOS_URL/api/productos" -H 'Content-Type: application/json' -d "{\"nombre\":\"Producto cluster ${RUN_ID}\",\"descripcion\":\"Producto temporal para pruebas de balanceo\",\"precioUnitario\":1,\"stock\":100000,\"imagenes\":[]}")"
if command -v jq >/dev/null 2>&1; then
  PRODUCT_ID="$(printf '%s' "$CREATE_RESPONSE" | jq -r '.id // empty')"
else
  PRODUCT_ID="$(printf '%s' "$CREATE_RESPONSE" | sed -n 's/.*"id"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)"
fi
[ -n "$PRODUCT_ID" ] && [ "$PRODUCT_ID" != "null" ] || die "No se pudo obtener el ID del producto de prueba: $CREATE_RESPONSE"
ok "Producto de prueba ID=$PRODUCT_ID"

mkdir -p "$LOG_DIR"

if ! wait_running_count 3; then
  log "Ajustando el servicio a 3 réplicas para iniciar la prueba..."
  docker service scale "$SERVICE=3" >/dev/null
  wait_running_count 3 || die "El cluster no alcanzó 3/3"
fi

run_phase "fase-1-con-3-replicas" 3 "$REQUESTS"

scale_to 2
run_phase "fase-2-con-2-replicas" 2 "$REQUESTS"

scale_to 1
run_phase "fase-3-con-1-replica" 1 "$REQUESTS"

if [ "$RESTORE" = "1" ]; then
  log "Restaurando 3 réplicas..."
  docker service scale "$SERVICE=3" >/dev/null
  if wait_running_count 3; then
    ok "Cluster restaurado a 3 réplicas"
  else
    mark_fail "No se pudo restaurar el cluster a 3 réplicas"
  fi
fi

printf '\n=== Resumen ===\n'
if [ "$FAILED" -eq 0 ]; then
  printf 'Todas las validaciones pasaron. Evidencia en %s/\n' "$LOG_DIR"
else
  printf 'Hubo validaciones fallidas. Revisá la salida y %s/\n' "$LOG_DIR"
fi
exit "$FAILED"
