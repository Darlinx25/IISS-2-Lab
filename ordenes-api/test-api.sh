#!/usr/bin/env bash

BASE_URL="${BASE_URL:-http://localhost:8081}"

run() {
  echo "$ $*"
  "$@"
  echo ""
}

echo "=== Test API de Ordenes ==="
echo ""

echo "--- POST /api/ordenes (crear) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" -X POST "$BASE_URL/api/ordenes" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "cliente2@email.com",
    "direccionEnvio": "Av. Roosevelt 1000, Montevideo",
    "telefono": "+59899000111",
    "productos": [
      {
        "productoId": 1,
        "cantidad": 2
      },
      {
        "productoId": 2,
        "cantidad": 1
      }
    ]
  }'

echo "--- GET /api/ordenes (incluye la creada) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL/api/ordenes"

echo "--- GET /api/ordenes/1002 (orden creada) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL/api/ordenes/1002"

echo "--- GET /api/ordenes/1002/detalle ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL/api/ordenes/1002/detalle"

echo "--- POST /api/ordenes (producto inexistente) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" -X POST "$BASE_URL/api/ordenes" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "cliente3@email.com",
    "direccionEnvio": "Av. Brasil 500, Montevideo",
    "telefono": "+59899000222",
    "productos": [
      {
        "productoId": 999,
        "cantidad": 1
      }
    ]
  }'

echo "--- POST /api/ordenes (stock insuficiente) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" -X POST "$BASE_URL/api/ordenes" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "cliente4@email.com",
    "direccionEnvio": "Av. Uruguay 200, Montevideo",
    "telefono": "+59899000333",
    "productos": [
      {
        "productoId": 1,
        "cantidad": 1000
      }
    ]
  }'

echo "--- POST /api/ordenes (datos invalidos) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" -X POST "$BASE_URL/api/ordenes" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "email-invalido",
    "direccionEnvio": "",
    "telefono": "",
    "productos": []
  }'

echo "--- GET /api/ordenes/999 (no existe) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL/api/ordenes/999"

echo "--- GET /api/ordenes/999/detalle (no existe) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL/api/ordenes/999/detalle"

echo "=== Fin test API de Ordenes ==="