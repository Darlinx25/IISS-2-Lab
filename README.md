# Taller Ingeniería — IISS-2 Lab

- Facundo Salaberry
- Kevin Jaffe
- Ignacio Ortega
- Alexis Menchaca

Sistema de órdenes de compra con APIs en Go, MySQL, MQTT, interfaz web, Jenkins y despliegue con Docker Compose o Docker Swarm.

## 1. Puesta en marcha

Requiere Docker y Docker Compose v2.

### Stack completo con Docker Compose

```bash
docker compose up -d --build
```

| Servicio | URL | Función |
|---|---|---|
| Interfaz web | http://localhost:8083 | UI de órdenes |
| API de productos | http://localhost:8080 | CRUD de productos |
| API de órdenes | http://localhost:8081 | Creación y consulta de órdenes |
| API de facturas | http://localhost:8082 | Consulta de facturas generadas |
| Jenkins | http://localhost:8090 | Pipeline CI/CD |
| MySQL productos | localhost:3306 | Base `productos` |
| MySQL órdenes | localhost:3307 | Base `ordenes` |
| MySQL facturas | localhost:3308 | Base `facturas` |
| MQTT (mosquitto) | localhost:1883 | Broker de mensajes |

> En el cluster de la Parte 5, `scripts/docker-compose.cluster.yml` evita publicar los puertos de MySQL para no chocar con otras instancias locales.

### Cluster Swarm (Parte 5)

Podés desplegarlo paso a paso con los comandos de la sección 7 o usar los scripts como atajo:

```bash
./scripts/levantar-cluster.sh   # Swarm + dependencias + 3 réplicas
./scripts/probar-balanceo.sh    # prueba, escala 3 -> 2 -> 1 y restaura
./scripts/bajar-cluster.sh      # baja servicio, contenedores y red
```

La API balanceada queda en `http://localhost:8081/api/ordenes`.

## 2. Arquitectura

### MER

<img width="957" height="970" alt="MER" src="https://github.com/user-attachments/assets/e0e32941-a77c-471f-8df9-2918f7387a52" />

### Flujo MQTT

Al crear una orden en `ordenes-api` (estado `Created`), la API publica un mensaje en el topic `ordenes/para-procesar` con QoS 1. El servicio `procesador-api` consume el mensaje y:

1. Verifica el stock de cada producto contra `productos-api`.
2. Si falta stock, marca la orden como `No Stock` (no genera factura).
3. Si hay stock, decrementa el stock y marca la orden como `Ready to Delivery`.
4. Genera la factura en `facturas-db` (encabezado + ítems con `cantidad` y `precio_unitario`; `monto_total = Σ cantidad × precio_unitario`).

Las facturas se consultan a través de `procesador-api` (puerto `8082`), sin acceso directo a la base.

## 3. Estructura del proyecto

<details>
<summary>Ver árbol de archivos</summary>

```text
IISS-2-Lab/
├── docker-compose.yml
├── ordenes.yaml
├── productos.yaml
├── README.md
│
├── scripts/
│   ├── levantar-cluster.sh
│   ├── probar-balanceo.sh
│   ├── bajar-cluster.sh
│   └── docker-compose.cluster.yml
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
│   ├── msg.json
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
├── db/
│   ├── productos/
│   │   └── init.sql
│   ├── ordenes/
│   │   └── init.sql
│   └── facturas/
│       └── init.sql
│
├── deploy/          # salida del pipeline de Jenkins
└── logs/            # evidencia generada por probar-balanceo.sh
```

</details>

## 4. APIs

### Productos

#### Crear

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

#### Listar y consultar

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8080/api/productos
```

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8080/api/productos/3
```

#### Actualizar

El `PUT` reemplaza el producto completo:

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

El `PATCH` modifica solo los campos enviados:

```bash
curl -s -w "\n[HTTP %{http_code}]\n" -X PATCH http://localhost:8080/api/productos/2 \
  -H "Content-Type: application/json" \
  -d '{
    "stock": 10
  }'
```

#### Errores representativos

Crear un producto con datos inválidos:

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

Consultar un producto inexistente:

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8080/api/productos/999
```

### Órdenes

#### Crear

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

#### Consultar

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8081/api/ordenes
```

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8081/api/ordenes/1002
```

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8081/api/ordenes/1002/detalle
```

#### Errores representativos

Crear una orden con un producto inexistente:

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

Crear una orden con stock insuficiente:

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

Crear una orden con datos inválidos:

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

Un ID inexistente devuelve `404` tanto para la orden como para su detalle.

### Facturas

El servicio `procesador-api` procesa las órdenes vía MQTT y expone las facturas generadas en el puerto `8082`.

#### Consultar

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8082/api/ordenes/1002/factura
```

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8082/api/facturas
```

```bash
curl -s -w "\n[HTTP %{http_code}]\n" http://localhost:8082/api/facturas/1
```

#### Reprocesar por MQTT (opcional)

```bash
docker exec mosquitto mosquitto_pub \
  -t "ordenes/para-procesar" \
  -m '{"id":9999,"estado":"Created"}' \
  -q 1
```

## 5. Interfaz web

La interfaz web está disponible en [http://localhost:8083](http://localhost:8083/). Consume `ordenes-api` a través de nginx, que actúa como reverse proxy.

Muestra:

1. Una tabla con las órdenes de compra (Id, Email, Teléfono, Status y Fecha de creación), ordenadas por defecto de la más reciente a la más antigua.
2. Al hacer click sobre una orden, se expande el detalle con los productos de esa orden (cantidad, precio unitario y subtotal).
3. Para las órdenes en estado `Ready to Delivery`, se muestra además el detalle de la facturación del pedido (número de factura, fecha de emisión, ítems y monto total).

## 6. Jenkins

Interfaz web: [http://localhost:8090](http://localhost:8090/)

Obtener la clave inicial de Jenkins:

```bash
docker exec -it jenkins cat /var/jenkins_home/secrets/initialAdminPassword
```

1. Al hacer `docker compose up`, en los logs de Jenkins aparece la password temporal de admin.
2. En la interfaz web de Jenkins, instalar lo recomendado en el primer inicio.
3. Crear un job tipo pipeline.
4. Pegar el contenido de `Jenkinsfile` en la sección de script y guardar.
5. Colocar el código nuevo en `jenkins/entradas/nueva version/`.
6. Ejecutar `Build Now` y esperar el resultado del job.

Si todo sale bien, Jenkins copia el código de `jenkins/entradas/` al directorio `deploy/`.

## 7. Docker Swarm — Balanceo de carga

Para simular un cluster con múltiples instancias de `ordenes-api` y balancear las solicitudes `POST /api/ordenes`, se utiliza Docker Swarm con el *ingress routing mesh* sobre el puerto `8081`.

No se usa nginx para el balanceo: el routing mesh de Swarm distribuye el tráfico entre las réplicas.

### Requisitos

- Docker con Swarm habilitado.
- Imagen `ordenes-api:lab5` construida.
- Dependencias levantadas: `ordenes-db`, `productos-api`, `productos-db` y `mosquitto`.

### Despliegue manual (sin scripts)

Estos comandos permiten levantar el cluster sin depender de los scripts.

#### 1. Inicializar Swarm

```bash
docker swarm init
```

Si el nodo tiene varias IPs, indicar una:

```bash
docker swarm init --advertise-addr <IP>
```

Verificar el nodo:

```bash
docker node ls
```

#### 2. Crear red overlay

```bash
docker network create --driver overlay --attachable iiss-net
```

#### 3. Construir la imagen

```bash
docker build -t ordenes-api:lab5 ./ordenes-api
```

#### 4. Levantar dependencias

En este entorno se usa el override para no publicar MySQL y evitar conflictos en `3306`/`3307`:

```bash
docker compose -f docker-compose.yml -f scripts/docker-compose.cluster.yml \
  up -d productos-db productos-api ordenes-db mosquitto
```

Si el `ordenes-api` de Docker Compose está corriendo, detenelo para liberar el puerto `8081`:

```bash
docker compose -f docker-compose.yml -f scripts/docker-compose.cluster.yml stop ordenes-api
```

#### 5. Conectar dependencias a la red overlay

```bash
docker network connect iiss-net productos-db
docker network connect iiss-net ordenes-db
docker network connect iiss-net productos-api
docker network connect iiss-net mosquitto
```

#### 6. Crear el servicio con 3 réplicas

Si el servicio ya existe, eliminarlo antes:

```bash
docker service rm ordenes-api-cluster
```

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

### Verificar réplicas

```bash
docker service ls
docker service ps ordenes-api-cluster
```

Deben aparecer `3/3` en `REPLICAS` y todas las tareas en estado `Running`.

### Verificar balanceo

```bash
docker service logs ordenes-api-cluster -f 2>&1 | grep "\[GIN\].*POST"
```

Las peticiones `POST /api/ordenes` deben aparecer con el nombre de cada réplica (`ordenes-api-cluster.1`, `.2`, `.3`). El script `probar-balanceo.sh` hace esta verificación automáticamente y guarda la evidencia en `logs/<fase>/`.

### Escalar réplicas

```bash
docker service scale ordenes-api-cluster=2
docker service scale ordenes-api-cluster=1
docker service scale ordenes-api-cluster=3
```

Tras escalar, verificar con `docker service ps ordenes-api-cluster` que únicamente las réplicas activas reciben las solicitudes.

### Automatización opcional (scripts)

Si no querés escribir los comandos manualmente:

```bash
# Levantar Swarm, dependencias y 3 réplicas de ordenes-api
./scripts/levantar-cluster.sh

# Probar balanceo, bajar a 2 y 1 instancia, y verificar logs
./scripts/probar-balanceo.sh

# Opcional: no restaurar a 3 réplicas al final
RESTORE=0 ./scripts/probar-balanceo.sh

# Opcional: cantidad de requests por fase
REQUESTS=30 ./scripts/probar-balanceo.sh
```

`levantar-cluster.sh` usa `scripts/docker-compose.cluster.yml` para levantar las dependencias sin publicar MySQL y evitar conflictos en `3306`/`3307`. Los comandos manuales de arriba son equivalentes.

## 8. Bajar y limpiar

### Bajar el cluster manualmente

```bash
# Quitar el servicio y la red overlay
docker service rm ordenes-api-cluster
docker network rm iiss-net

# Bajar los contenedores del proyecto (conserva los volúmenes)
docker compose -f docker-compose.yml -f scripts/docker-compose.cluster.yml down
```

Si querés borrar también los datos:

```bash
docker compose -f docker-compose.yml -f scripts/docker-compose.cluster.yml down -v
```

Borrar la imagen y los logs:

```bash
docker image rm -f ordenes-api:lab5
rm -rf logs
```

Salir de Docker Swarm:

```bash
docker swarm leave --force
```

### Automatización opcional (scripts)

```bash
# Baja servicio, contenedores y red; conserva datos, imágenes y Swarm
./scripts/bajar-cluster.sh

# Borra además volúmenes/BD, imágenes y logs
./scripts/bajar-cluster.sh --all

# Borrado total, incluyendo salir de Docker Swarm
./scripts/bajar-cluster.sh --all --swarm
```

| Opción | Efecto |
|---|---|
| `--volumes` | Borra los volúmenes de datos del proyecto (pierde la BD) |
| `--images` | Borra `ordenes-api:lab5` y las imágenes `iiss-2-lab-*` |
| `--logs` | Borra el directorio `logs/` |
| `--swarm` | Ejecuta `docker swarm leave --force` |
| `--all` | Equivale a `--volumes --images --logs` |

`bajar-cluster.sh` baja todo el proyecto de Docker Compose, no solo las dependencias del cluster.

La evidencia de las pruebas de balanceo queda en `logs/<fase>/`.
