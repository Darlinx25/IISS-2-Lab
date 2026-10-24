#!/usr/bin/env bash

BASE_URL="${BASE_URL:-http://localhost:8080}"

run() {
  echo "$ $*"
  "$@"
  echo ""
}

echo "=== Test API de Productos ==="
echo ""

echo "--- POST /api/productos (crear teclado) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" -X POST "$BASE_URL/api/productos" \
  -H "Content-Type: application/json" \
  -d '{
    "nombre": "Teclado Mecanico Redragon",
    "descripcion": "Teclado mecanico RGB",
    "precioUnitario": 89.99,
    "stock": 30,
    "imagenes": [
      "https://ejemplo.com/imagenes/teclado1.jpg",
      "https://ejemplo.com/imagenes/teclado2.jpg"
    ]
  }'

echo "--- GET /api/productos (incluye el creado) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL/api/productos"

echo "--- GET /api/productos/3 (producto creado) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL/api/productos/3"

echo "--- PUT /api/productos/3 (actualizar) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" -X PUT "$BASE_URL/api/productos/3" \
  -H "Content-Type: application/json" \
  -d '{
    "nombre": "Teclado Mecanico Redragon Pro",
    "descripcion": "Teclado mecanico RGB con teclas hotswap",
    "precioUnitario": 109.99,
    "stock": 25,
    "imagenes": [
      "https://ejemplo.com/imagenes/teclado1.jpg"
    ]
  }'

echo "--- GET /api/productos/3 (despues del PUT) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL/api/productos/3"

echo "--- PATCH /api/productos/2 (modificar stock) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" -X PATCH "$BASE_URL/api/productos/2" \
  -H "Content-Type: application/json" \
  -d '{
    "stock": 10
  }'

echo "--- GET /api/productos/2 (despues del PATCH) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL/api/productos/2"

echo "--- POST /api/productos (datos invalidos) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" -X POST "$BASE_URL/api/productos" \
  -H "Content-Type: application/json" \
  -d '{
    "nombre": "",
    "descripcion": "Sin nombre",
    "precioUnitario": 10.0,
    "stock": 5,
    "imagenes": []
  }'

echo "--- GET /api/productos/999 (no existe) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" "$BASE_URL/api/productos/999"

echo "--- PUT /api/productos/999 (no existe) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" -X PUT "$BASE_URL/api/productos/999" \
  -H "Content-Type: application/json" \
  -d '{
    "nombre": "Test",
    "descripcion": "Test",
    "precioUnitario": 1.0,
    "stock": 1,
    "imagenes": []
  }'

echo "--- PATCH /api/productos/999 (no existe) ---"
run curl -s -w "\n[HTTP %{http_code}]\n" -X PATCH "$BASE_URL/api/productos/999" \
  -H "Content-Type: application/json" \
  -d '{
    "stock": 10
  }'

echo "=== Fin test API de Productos ==="