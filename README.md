## Taller Ingenieria
- Facundo Salaberry
- Kevin Jaffe
- Ignacio Ortega
- Alexis Menchaca

### MER

<img width="957" height="970" alt="image" src="https://github.com/user-attachments/assets/e0e32941-a77c-471f-8df9-2918f7387a52" />

### Puertos

| Servicio | URL |
|---|---|
| Interfaz web | http://localhost:8083 |
| API de productos | http://localhost:8080 |
| API de órdenes | http://localhost:8081 |
| API de facturas (procesador) | http://localhost:8082 |
| Jenkins | http://localhost:8090 |
| MySQL productos | localhost:3306 |
| MySQL órdenes | localhost:3307 |
| MySQL facturas | localhost:3308 |
| MQTT (mosquitto) | localhost:1883 |


### Estructura del proyecto
```
IISS-2-Lab/
├── docker-compose.yml
├── README.md
├── ordenes.yaml
├── productos.yaml
│
├── ordenes-api/
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   ├── main.go
│   ├── test-api.sh
│   └── mysql/
│       └── repository.go
│
├── productos-api/
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   ├── main.go
│   ├── test-api.sh
│   └── mysql/
│       └── repository.go
│
├── procesador-api/
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   ├── main.go
│   ├── test-api.sh
│   └── mysql/
│       └── repository.go
│
├── web/
│   ├── Dockerfile
│   ├── index.html
│   ├── app.js
│   ├── styles.css
│   └── nginx.conf
│
├── jenkins/
│   ├── Jenkinsfile
│   └── entradas/        
│
├── mosquitto/
│   └── config/
│       └── mosquitto.conf
│
└── db/
    ├── productos/
    │   └── init.sql
    ├── ordenes/
    │   └── init.sql
    └── facturas/
        └── init.sql
```

### Interfaz web

La interfaz web está disponible en [http://localhost:8083](http://localhost:8083/).
Consume `ordenes-api` a través de nginx, que actúa como reverse proxy.

Muestra:

1. Una tabla con las órdenes de compra (Id, Email, Teléfono, Status y Fecha de creación),
   ordenadas por defecto de la más reciente a la más antigua.
2. Al hacer click sobre una orden, se expande el detalle con los productos de esa orden
   (cantidad, precio unitario y subtotal).
3. Para las órdenes en estado `Ready to Delivery`, se muestra además el detalle de la
   facturación del pedido (número de factura, fecha de emisión, ítems y monto total).

### Jenkins

Interfaz web:
[http://localhost:8090](http://localhost:8090/)

Obtener clave de Jenkins
```
docker exec -it jenkins cat /var/jenkins_home/secrets/initialAdminPassword
```
1. Al hacer docker compose up, en los logs de Jenkins obtenemos la password temporal de admin que nos pide para logearnos.
2. Luego en la interfaz web de Jenkins instalamos lo recomendado en la primera vez iniciada.
3. Creamos job tipo pipeline.
4. Pegamos contenido de `Jenkinsfile` en la sección script para que el job lo use y ponemos guardar.
5. Poner el codigo nuevo en el directorio `jenkins/entradas/nueva version/`
6. Luego hacemos build now, y esperamos que realize el job y vemos si es fail o success.

Si todo sale bien Jenkins copiará el código que colocamos en `entradas/` al directorio `deploy/`

### Flujo de procesamiento (MQTT)

Al crear una orden en `ordenes-api` (estado `Created`), la API publica un mensaje en el topic `ordenes/para-procesar` con QoS 1. El servicio `procesador-api` consume el mensaje y:

1. Verifica el stock de cada producto contra `productos-api`.
2. Si falta stock, marca la orden como `No Stock` (no genera factura).
3. Si hay stock, decrementa el stock y marca la orden como `Ready to Delivery`.
4. Genera la factura en `facturas-db` (encabezado + ítems con `cantidad` y `precio_unitario`; `monto_total = Σ cantidad × precio_unitario`).

Las facturas se consultan a través de `procesador-api` (puerto `8082`), sin acceso directo a la base.

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

### API de Facturas (Procesador)

El servicio `procesador-api` procesa las órdenes vía MQTT y expone las facturas generadas en el puerto `8082`.

Obtener la factura de una orden.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8082/api/ordenes/1002/factura
```

Obtener todas las facturas.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8082/api/facturas
```

Obtener una factura por ID.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8082/api/facturas/1
```

Intentar obtener una factura que no existe.

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8082/api/facturas/999
```

Publicar un mensaje manualmente (caso "sin stock" o reprocesamiento).

```bash
docker exec mosquitto mosquitto_pub -t "ordenes/para-procesar" -m '{"id":9999,"estado":"Created"}' -q 1
```


### Docker Swarm - Balanceo de carga (API de Órdenes)

Para simular un cluster con múltiples instancias de `ordenes-api` y balancear las solicitudes `POST /api/ordenes`, se utiliza Docker Swarm con el *ingress routing mesh* sobre el puerto `8081`.

#### Requisitos

- Docker con Swarm habilitado
- Imagen `ordenes-api:lab5` construida
- Dependencias levantadas: `ordenes-db`, `productos-api`, `productos-db`, `mosquitto`

#### Inicializar Swarm

```bash
docker swarm init
```

Verificar el nodo:

```bash
docker node ls
```

#### Crear red overlay

```bash
docker network create --driver overlay --attachable iiss-net
```

#### Construir imagen de `ordenes-api`

Desde la raíz del proyecto:

```bash
docker build -t ordenes-api:lab5 ./ordenes-api
```

#### Conectar dependencias a la red overlay

Si las dependencias fueron levantadas con `docker compose`, es necesario conectarlas a `iiss-net` para que las réplicas del Swarm puedan resolver sus nombres:

```bash
docker network connect iiss-net productos-db
docker network connect iiss-net ordenes-db
docker network connect iiss-net productos-api
docker network connect iiss-net mosquitto
```

#### Desplegar 3 réplicas de `ordenes-api`

```bash
docker service create \
  --name ordenes-api-cluster \
  --network iiss-net \
  --replicas 3 \
  --publish published=8081,target=8081 \
  --env DB_HOST=ordenes-db \
  --env DB_PORT=3306 \
  --env DB_NAME=ordenes \
  --env DB_USER=user \
  --env DB_PASSWORD=password \
  --env PRODUCTOS_API_URL=http://productos-api:8080 \
  --env MQTT_BROKER_URL=tcp://mosquitto:1883 \
  ordenes-api:lab5
```

#### Verificar réplicas y estado

Listar servicios:

```bash
docker service ls
```

Ver el estado de cada réplica:

```bash
docker service ps ordenes-api-cluster
```

Deben aparecer `3/3` en `REPLICAS` y todas las tareas en estado `Running`.

#### Verificar balanceo de carga

Para visualizar los requests HTTP que llegan a las réplicas, filtrar los logs por el acceso de Gin:

**Bash / WSL:**

```bash
docker service logs ordenes-api-cluster -f 2>&1 | grep "\[GIN\].*POST"
```

**PowerShell:**

```powershell
docker service logs ordenes-api-cluster -f 2>&1 | Select-String -Pattern "\[GIN\].*POST"
```

Las peticiones `POST /api/ordenes` deberán aparecer con el nombre de cada réplica (`ordenes-api-cluster.1`, `.2`, `.3`), demostrando que el *ingress routing mesh* distribuye el tráfico entre las instancias.

#### Escalar réplicas

Para disminuir o aumentar la cantidad de instancias:

```bash
docker service scale ordenes-api-cluster=2
docker service scale ordenes-api-cluster=1
docker service scale ordenes-api-cluster=3
```

Tras escalar, verificar con `docker service ps ordenes-api-cluster` y con los logs filtrados por `[GIN]` que únicamente las réplicas activas reciben las solicitudes `POST /api/ordenes`.

#### Scripts Parte 5

```bash
# Levantar Swarm, dependencias y 3 réplicas de ordenes-api
./scripts/levantar-cluster.sh

# Probar balanceo, bajar a 2 y 1 instancia, y verificar logs
./scripts/probar-balanceo.sh

# Opcional: no restaurar a 3 réplicas al final
RESTORE=0 ./scripts/probar-balanceo.sh

# Opcional: cantidad de requests por fase
REQUESTS=30 ./scripts/probar-balanceo.sh

# Bajar servicio, contenedores y red (conserva datos e imágenes)
./scripts/bajar-cluster.sh

# Borrar también datos, imágenes y logs
./scripts/bajar-cluster.sh --all

# Borrar todo lo anterior y salir de Docker Swarm
./scripts/bajar-cluster.sh --all --swarm
```

La evidencia de logs queda en `logs/<fase>/`.
No se usa nginx: el routing mesh de Swarm balancea el puerto publicado.
