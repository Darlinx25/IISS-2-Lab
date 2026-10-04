#!/usr/bin/env bash

BASE_URL_ORDENES="${BASE_URL_ORDENES:-http://localhost:8081}"
BASE_URL_PRODUCTOS="${BASE_URL_PRODUCTOS:-http://localhost:8080}"
BASE_URL_FACTURAS="${BASE_URL_FACTURAS:-http://localhost:8082}"

run() {
  echo "$ $*"
  "$@"
  echo ""
}

echo "=== Test Procesador + Factura (API) ==="
echo ""
echo "Recorda levantar todo antes: docker compose up -d --build"
echo ""

echo "--- 0. Estado inicial de stock ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_PRODUCTOS/api/productos/1"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_PRODUCTOS/api/productos/2"

echo "--- 1. Crear orden con stock (publica en MQTT y procesa) ---"
RESP=$(curl -s -X POST "$BASE_URL_ORDENES/api/ordenes" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "cliente@test.com",
    "direccionEnvio": "Av. Test 123, Montevideo",
    "telefono": "+59899111222",
    "productos": [
      {
        "productoId": 1,
        "cantidad": 2
      }
    ]
  }')
echo "Respuesta: $RESP"
ORDER_ID=$(echo "$RESP" | sed -n 's/.*"id": *\([0-9][0-9]*\).*/\1/p')
echo "Orden creada: $ORDER_ID"
echo ""

echo "--- 2. Esperando al consumidor (MQTT) ---"
sleep 3

echo "--- 3. Estado de la orden procesada ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_ORDENES/api/ordenes/$ORDER_ID"

echo "--- 4. Stock del producto 1 (debe haber decrementado) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_PRODUCTOS/api/productos/1"

echo "--- 5. Factura de la orden ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_FACTURAS/api/ordenes/$ORDER_ID/factura"

echo "--- 6. Detalle de la factura por id ---"
FACTURA_ID=$(curl -s "$BASE_URL_FACTURAS/api/ordenes/$ORDER_ID/factura" | sed -n 's/.*"id": *\([0-9][0-9]*\).*/\1/p' | head -1)
echo "Factura id: $FACTURA_ID"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_FACTURAS/api/facturas/$FACTURA_ID"

echo "--- 7. Listado completo de facturas ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_FACTURAS/api/facturas"

echo "--- 8. Factura inexistente (404) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_FACTURAS/api/facturas/999"

echo "--- 9. Pedido sin stock (insercion directa + mensaje) ---"
run docker exec facturas-db mysql -uuser -ppassword facturas -e "DELETE FROM facturas WHERE orden_id = 9000;"
run docker exec ordenes-db mysql -uuser -ppassword ordenes -e "DELETE FROM ordenes WHERE id = 9000; INSERT INTO ordenes (id, email, direccion_envio, telefono, estado, fecha_creacion) VALUES (9000, 'clienteNoStock@test.com', 'Av. Test 456', '+59899111333', 'Created', NOW()); INSERT INTO orden_productos (orden_id, producto_id, cantidad) VALUES (9000, 2, 999);"
run docker exec mosquitto mosquitto_pub -t "ordenes/para-procesar" -m '{"id":9000,"estado":"Created"}' -q 1
sleep 3
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_ORDENES/api/ordenes/9000"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_PRODUCTOS/api/productos/2"

echo "--- 10. Idempotencia (republicar una orden ya procesada) ---"
run docker exec mosquitto mosquitto_pub -t "ordenes/para-procesar" -m "{\"id\":$ORDER_ID,\"estado\":\"Created\"}" -q 1
sleep 3
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_PRODUCTOS/api/productos/1"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_FACTURAS/api/ordenes/$ORDER_ID/factura"

echo ""
echo "--- 11. Caida del broker: la orden no se debe perder ---"

estado() {
  curl -s "$BASE_URL_ORDENES/api/ordenes/$1" | sed -n 's/.*"Estado":"\([^"]*\)".*/\1/p'
}

stock() {
  curl -s "$BASE_URL_PRODUCTOS/api/productos/$1" | sed -n 's/.*"Stock": *\([0-9][0-9]*\).*/\1/p'
}

fallos=0
check() {
  if [ "$2" = "$3" ]; then
    echo "  OK   $1: $2"
  else
    echo "  FALLA $1: se esperaba '$3' y se obtuvo '$2'"
    fallos=$((fallos + 1))
  fi
}

echo "Stock inicial del producto 1: $(stock 1)"
STOCK_ANTES=$(stock 1)

echo "Deteniendo el broker MQTT..."
run docker stop mosquitto
sleep 3

echo "Creando una orden con el broker caido (debe quedar pendiente):"
RESP=$(curl -s -w "\n[HTTP %{http_code}]" -X POST "$BASE_URL_ORDENES/api/ordenes" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "clienteCaida@test.com",
    "direccionEnvio": "Av. Caida 789, Montevideo",
    "telefono": "+59899444555",
    "productos": [
      {
        "productoId": 1,
        "cantidad": 1
      }
    ]
  }')
echo "$RESP"
PEND_ID=$(echo "$RESP" | sed -n 's/.*"id": *\([0-9][0-9]*\).*/\1/p')

if [ -z "$PEND_ID" ]; then
  echo "  FALLA no se pudo crear la orden con el broker caido"
  fallos=$((fallos + 1))
else
  check "la orden nace pendiente de MQTT" "$(estado "$PEND_ID")" "Pendiente Mosquitto"
  check "no se descuenta stock todavia" "$(stock 1)" "$STOCK_ANTES"
fi

echo "Levantando el broker MQTT de nuevo..."
run docker start mosquitto

echo "Esperando al reconciliador y al consumidor..."
sleep 15

if [ -n "$PEND_ID" ]; then
  run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_ORDENES/api/ordenes/$PEND_ID"
  check "la orden se proceso al reconectar" "$(estado "$PEND_ID")" "Ready to Delivery"
  check "el stock se descuenta una sola vez" "$(stock 1)" "$((STOCK_ANTES - 1))"
  run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL_FACTURAS/api/ordenes/$PEND_ID/factura"
fi

echo ""
if [ "$fallos" -eq 0 ]; then
  echo "=== Recuperacion ante caida del broker: OK ==="
else
  echo "=== Recuperacion ante caida del broker: $fallos FALLO(S) ==="
fi

echo ""
echo "=== Fin test Procesador + Factura (API) ==="