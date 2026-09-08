## Taller Ingenieria
- Facundo Salaberry
- Kevin Jaffe
- Ignacio Ortega
- Alexis Menchaca

### MER

<img width="1129" height="973" alt="image" src="https://github.com/user-attachments/assets/35cf23f6-3662-447d-8bfb-3d563a14c3b1" />

### Estructura del proyecto
```
iiss2-apis/
├── docker-compose.yml
├── ordenes.yaml
├── productos.yaml
│
├── ordenes-api/
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   ├── main.go
│   └── test-api.sh
│
└── productos-api/
    ├── Dockerfile
    ├── go.mod
    ├── go.sum
    ├── main.go
    └── test-api.sh
```
  ### API de Productos

Pruebas básicas de creación, consulta, actualización y manejo de errores de la API de productos.

Crear un producto nuevo.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" -X POST http://localhost:8080/api/productos \
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
```

Obtener todos los productos.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8080/api/productos
```

Obtener un producto por ID.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8080/api/productos/3
```

Actualizar un producto completo.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" -X PUT http://localhost:8080/api/productos/3 \
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
```

Obtener el producto después de actualizarlo.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8080/api/productos/3
```

Modificar parcialmente el stock de un producto.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" -X PATCH http://localhost:8080/api/productos/2 \
  -H "Content-Type: application/json" \
  -d '{
    "stock": 10
  }'
```

Obtener el producto después del PATCH.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8080/api/productos/2
```

Intentar crear un producto con datos inválidos.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" -X POST http://localhost:8080/api/productos \
  -H "Content-Type: application/json" \
  -d '{
    "nombre": "",
    "descripcion": "Sin nombre",
    "precioUnitario": 10.0,
    "stock": 5,
    "imagenes": []
  }'
```

Intentar obtener un producto que no existe.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8080/api/productos/999
```

Intentar actualizar un producto que no existe.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" -X PUT http://localhost:8080/api/productos/999 \
  -H "Content-Type: application/json" \
  -d '{
    "nombre": "Test",
    "descripcion": "Test",
    "precioUnitario": 1.0,
    "stock": 1,
    "imagenes": []
  }'
```

Intentar modificar parcialmente un producto que no existe.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" -X PATCH http://localhost:8080/api/productos/999 \
  -H "Content-Type: application/json" \
  -d '{
    "stock": 10
  }'
```

### API de Órdenes

Pruebas de creación y consulta de órdenes, incluyendo casos de productos inexistentes, stock insuficiente y datos inválidos.

Crear una nueva orden.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" -X POST http://localhost:8081/api/ordenes \
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
```

Obtener todas las órdenes.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8081/api/ordenes
```

Obtener una orden por ID.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8081/api/ordenes/1002
```

Obtener el detalle de una orden.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8081/api/ordenes/1002/detalle
```

Intentar crear una orden con un producto inexistente.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" -X POST http://localhost:8081/api/ordenes \
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
```

Intentar crear una orden con stock insuficiente.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" -X POST http://localhost:8081/api/ordenes \
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
```

Intentar crear una orden con datos inválidos.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" -X POST http://localhost:8081/api/ordenes \
  -H "Content-Type: application/json" \
  -d '{
    "email": "email-invalido",
    "direccionEnvio": "",
    "telefono": "",
    "productos": []
  }'
```

Intentar obtener una orden que no existe.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8081/api/ordenes/999
```

Intentar obtener el detalle de una orden que no existe.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8081/api/ordenes/999/detalle
```
