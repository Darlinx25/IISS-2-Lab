package main


import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mqtt "github.com/eclipse/paho.mqtt.golang"

	ordenesdb "ordenes-api/mysql"
)

const productosAPIURLDefault = "http://localhost:8080"

const (
	mqttBrokerDefault = "tcp://localhost:1883"
	mqttTopicOrdenes  = "ordenes/para-procesar"
)

var productosAPIURL = func() string {
	if url := os.Getenv("PRODUCTOS_API_URL"); url != "" {
		return url
	}
	return productosAPIURLDefault
}()

var mqttBrokerURL = getEnv("MQTT_BROKER_URL", mqttBrokerDefault)

type OrdenRequest struct {
	Email          string                       `json:"email"`
	DireccionEnvio string                       `json:"direccionEnvio"`
	Telefono       string                       `json:"telefono"`
	Productos      []ordenesdb.ProductoOrden    `json:"productos"`
}

type Producto struct {
	ID             int64    `json:"id"`
	Nombre         string   `json:"nombre"`
	Descripcion    string   `json:"descripcion"`
	PrecioUnitario float64  `json:"precioUnitario"`
	Stock          int      `json:"stock"`
	Imagenes       []string `json:"imagenes"`
}

type ProductoDetalle struct {
	ProductoID     int64    `json:"productoId"`
	Cantidad       int      `json:"cantidad"`
	Nombre         string   `json:"nombre"`
	Descripcion    string   `json:"descripcion"`
	PrecioUnitario float64  `json:"precioUnitario"`
	Stock          int      `json:"stock"`
	Imagenes       []string `json:"imagenes"`
}

type OrdenDetalle struct {
	ID             int64             `json:"id"`
	Email          string            `json:"email"`
	DireccionEnvio string            `json:"direccionEnvio"`
	Telefono       string            `json:"telefono"`
	Estado         string            `json:"estado"`
	FechaCreacion  string            `json:"fechaCreacion"`
	Productos      []ProductoDetalle `json:"productos"`
}

type MensajeOrden struct {
	ID            int64  `json:"id"`
	Estado        string `json:"estado"`
	FechaCreacion string `json:"fechaCreacion"`
}

type IdResponse struct {
	ID int64 `json:"id"`
}

type ErrorResponse struct {
	Mensaje string `json:"mensaje"`
}

const (
	EstadoCreated         = "Created"
	EstadoPendienteMQTT   = "Pendiente Mosquitto"
	EstadoConfirmed       = "Confirmed"
	EstadoProcessing      = "Processing"
	EstadoReadyToDelivery = "Ready to Delivery"
	EstadoNoStock         = "No Stock"
	EstadoShipped         = "Shipped"
	EstadoDelivered       = "Delivered"
	EstadoCancelled       = "Cancelled"
)

var estadosActualizables = map[string]bool{
	EstadoProcessing:      true,
	EstadoReadyToDelivery: true,
	EstadoNoStock:         true,
}

const intervaloReconciliacion = 5 * time.Second

type Handlers struct {
	repo       ordenesdb.OrdenRepository
	mqttClient mqtt.Client
}

func main() {
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "3306")
	dbName := getEnv("DB_NAME", "ordenes")
	dbUser := getEnv("DB_USER", "user")
	dbPass := getEnv("DB_PASSWORD", "password")

	dsn := dbUser + ":" + dbPass + "@tcp(" + dbHost + ":" + dbPort + ")/" + dbName + "?parseTime=true"

	var db *sql.DB
	var err error

	for i := 0; i < 30; i++ {
		db, err = sql.Open("mysql", dsn)
		if err == nil {
			err = db.Ping()
		}
		if err == nil {
			break
		}
		fmt.Printf("Esperando MySQL... (%d/30)\n", i+1)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		panic("No se pudo conectar a la base de datos: " + err.Error())
	}

	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	repo := ordenesdb.NewOrdenRepository(db)
	h := &Handlers{repo: repo, mqttClient: conectarMQTT()}

	go h.reconciliarPendientes()

	router := gin.Default()

	router.GET("/api/ordenes", h.obtenerOrdenes)
	router.POST("/api/ordenes", h.crearOrden)
	router.GET("/api/ordenes/:id", h.obtenerOrdenPorID)
	router.GET("/api/ordenes/:id/detalle", h.obtenerDetalleOrden)
	router.PATCH("/api/ordenes/:id", h.modificarEstadoOrden)

	fmt.Println("======================================")
	fmt.Println("API de Órdenes")
	fmt.Println("======================================")
	fmt.Println("Servidor escuchando en:")
	fmt.Println("http://localhost:8081")
	fmt.Println("MQTT broker:", mqttBrokerURL)
	fmt.Println("======================================")

	puerto := os.Getenv("PORT")
	if puerto == "" {
		puerto = "8081"
	}

	if err := router.Run(":" + puerto); err != nil {
		panic(err)
	}
}

func conectarMQTT() mqtt.Client {
	clientID := getEnv("MQTT_CLIENT_ID", "")
	if clientID == "" {
		hostname, err := os.Hostname()
		if err != nil || hostname == "" {
			hostname = strconv.FormatInt(time.Now().UnixNano(), 10)
		}
		clientID = "ordenes-api-" + hostname
	}

	fmt.Println("MQTT client ID:", clientID)

	opts := mqtt.NewClientOptions().
		AddBroker(mqttBrokerURL).
		SetClientID(clientID).
		SetKeepAlive(30 * time.Second).
		SetPingTimeout(10 * time.Second).
		SetConnectTimeout(3 * time.Second).
		SetConnectRetry(true).
		SetConnectRetryInterval(2 * time.Second).
		SetMaxReconnectInterval(10 * time.Second).
		SetAutoReconnect(true).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			fmt.Printf("Se perdió la conexión con MQTT (%v). Las órdenes nuevas quedarán pendientes.\n", err)
		}).
		SetReconnectingHandler(func(_ mqtt.Client, _ *mqtt.ClientOptions) {
			fmt.Println("Reconectando con el broker MQTT...")
		}).
		SetOnConnectHandler(func(_ mqtt.Client) {
			fmt.Println("Conectado al broker MQTT en", mqttBrokerURL)
		})

	client := mqtt.NewClient(opts)

	token := client.Connect()
	token.WaitTimeout(3 * time.Second)

	if token.Error() != nil {
		fmt.Printf("MQTT no disponible en %s (%v). Las órdenes quedarán pendientes y se publicarán al reconectar.\n", mqttBrokerURL, token.Error())
	} else {
		fmt.Println("MQTT conectado en", mqttBrokerURL)
	}

	return client
}

func (h *Handlers) obtenerOrdenes(c *gin.Context) {
	ordenes, err := h.repo.ObtenerTodas()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Mensaje: "Error al obtener las órdenes",
		})
		return
	}
	c.JSON(http.StatusOK, ordenes)
}

func (h *Handlers) crearOrden(c *gin.Context) {
	var request OrdenRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Mensaje: "Los datos de la orden son inválidos",
		})
		return
	}

	if err := validarOrdenRequest(request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Mensaje: err.Error(),
		})
		return
	}

	for _, item := range request.Productos {
		producto, err := obtenerProductoDesdeAPI(item.ProductoID)

		if err != nil {
			c.JSON(http.StatusConflict, ErrorResponse{
				Mensaje: "El producto con identificador " +
					strconv.FormatInt(item.ProductoID, 10) +
					" no existe",
			})
			return
		}

		if producto.Stock < item.Cantidad {
			c.JSON(http.StatusConflict, ErrorResponse{
				Mensaje: "Stock insuficiente para el producto con identificador " +
					strconv.FormatInt(item.ProductoID, 10),
			})
			return
		}
	}

	ahora := time.Now()

	orden := ordenesdb.Orden{
		Email:          request.Email,
		DireccionEnvio: request.DireccionEnvio,
		Telefono:       request.Telefono,
		Estado:         EstadoPendienteMQTT,
		FechaCreacion:  ahora.Format("2006-01-02 15:04:05"),
		Productos:      request.Productos,
	}

	id, err := h.repo.Crear(orden)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Mensaje: "Error al crear la orden",
		})
		return
	}

	if err := h.publicarOrPendiente(id, ahora.Format(time.RFC3339)); err != nil {
		fmt.Printf("Orden %d quedó pendiente de publicación: %v\n", id, err)
	}

	c.JSON(http.StatusCreated, IdResponse{ID: id})
}

func (h *Handlers) publicarOrPendiente(id int64, fechaCreacion string) error {
	if err := h.publicar(id, fechaCreacion); err != nil {
		return err
	}

	confirmada, err := h.repo.ConfirmarPublicada(id, EstadoPendienteMQTT, EstadoCreated)
	if err != nil {
		return fmt.Errorf("la orden %d se publicó pero no se pudo confirmar: %v", id, err)
	}

	if !confirmada {
		fmt.Printf("Orden %d ya había avanzado de estado, no se toca\n", id)
		return nil
	}

	fmt.Printf("Orden %d publicada en el topic '%s'\n", id, mqttTopicOrdenes)
	return nil
}

func (h *Handlers) publicar(id int64, fechaCreacion string) error {
	if h.mqttClient == nil {
		return errors.New("cliente MQTT no inicializado")
	}

	mensaje := MensajeOrden{
		ID:            id,
		Estado:        EstadoCreated,
		FechaCreacion: fechaCreacion,
	}

	payload, err := json.Marshal(mensaje)
	if err != nil {
		return fmt.Errorf("no se pudo serializar la orden %d: %v", id, err)
	}

	token := h.mqttClient.Publish(mqttTopicOrdenes, 1, false, payload)

	if !token.WaitTimeout(3 * time.Second) {
		return fmt.Errorf("timeout esperando el PUBACK de la orden %d", id)
	}

	if token.Error() != nil {
		return fmt.Errorf("no se pudo publicar la orden %d: %v", id, token.Error())
	}

	return nil
}

func (h *Handlers) reconciliarPendientes() {
	for {
		time.Sleep(intervaloReconciliacion)

		if h.mqttClient == nil || !h.mqttClient.IsConnectionOpen() {
			continue
		}

		pendientes, err := h.repo.ObtenerPorEstado(EstadoPendienteMQTT)
		if err != nil {
			fmt.Printf("Error buscando órdenes pendientes: %v\n", err)
			continue
		}

		for _, orden := range pendientes {
			if err := h.publicarOrPendiente(orden.ID, orden.FechaCreacion); err != nil {
				fmt.Printf("Orden %d sigue pendiente: %v\n", orden.ID, err)
			}
		}
	}
}

func (h *Handlers) modificarEstadoOrden(c *gin.Context) {
	id, ok := obtenerID(c)
	if !ok {
		return
	}

	var request struct {
		Estado string   `json:"estado"`
		Desde  []string `json:"desde"`
	}

	if err := c.ShouldBindJSON(&request); err != nil || !estadosActualizables[request.Estado] {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Mensaje: "El estado de la orden es inválido",
		})
		return
	}

	if len(request.Desde) > 0 {
		ok, err := h.repo.ReclamarEstado(id, request.Desde, request.Estado)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Mensaje: "Error al modificar la orden",
			})
			return
		}
		if !ok {
			c.JSON(http.StatusConflict, ErrorResponse{
				Mensaje: "La orden ya no está en " + strings.Join(request.Desde, " ni "),
			})
			return
		}
	} else {
		err := h.repo.CambiarEstado(id, request.Estado)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Mensaje: "No existe una orden con el identificador " +
					strconv.FormatInt(id, 10),
			})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Mensaje: "Error al modificar la orden",
			})
			return
		}
	}

	orden, err := h.repo.ObtenerPorID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Mensaje: "Error al obtener la orden",
		})
		return
	}

	c.JSON(http.StatusOK, orden)
}

func (h *Handlers) obtenerOrdenPorID(c *gin.Context) {
	id, ok := obtenerID(c)
	if !ok {
		return
	}

	orden, err := h.repo.ObtenerPorID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Mensaje: "Error al obtener la orden",
		})
		return
	}

	if orden == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Mensaje: "No existe una orden con el identificador " +
				strconv.FormatInt(id, 10),
		})
		return
	}

	c.JSON(http.StatusOK, orden)
}

func (h *Handlers) obtenerDetalleOrden(c *gin.Context) {
	id, ok := obtenerID(c)
	if !ok {
		return
	}

	orden, err := h.repo.ObtenerPorID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Mensaje: "Error al obtener la orden",
		})
		return
	}

	if orden == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Mensaje: "No existe una orden con el identificador " +
				strconv.FormatInt(id, 10),
		})
		return
	}

	productosDetalle := make([]ProductoDetalle, 0)

	for _, item := range orden.Productos {
		producto, err := obtenerProductoDesdeAPI(item.ProductoID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Mensaje: "No se pudo obtener el producto con identificador " +
					strconv.FormatInt(item.ProductoID, 10),
			})
			return
		}

		detalle := ProductoDetalle{
			ProductoID:     producto.ID,
			Cantidad:       item.Cantidad,
			Nombre:         producto.Nombre,
			Descripcion:    producto.Descripcion,
			PrecioUnitario: producto.PrecioUnitario,
			Stock:          producto.Stock,
			Imagenes:       producto.Imagenes,
		}

		productosDetalle = append(productosDetalle, detalle)
	}

	respuesta := OrdenDetalle{
		ID:             orden.ID,
		Email:          orden.Email,
		DireccionEnvio: orden.DireccionEnvio,
		Telefono:       orden.Telefono,
		Estado:         orden.Estado,
		FechaCreacion:  orden.FechaCreacion,
		Productos:      productosDetalle,
	}

	c.JSON(http.StatusOK, respuesta)
}

func obtenerProductoDesdeAPI(id int64) (*Producto, error) {
	url := productosAPIURL +
		"/api/productos/" +
		strconv.FormatInt(id, 10)

	req, err := http.NewRequest(
		http.MethodGet,
		url,
		nil,
	)

	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("producto no encontrado")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"API de productos respondió con HTTP %d",
			resp.StatusCode,
		)
	}

	var producto Producto

	if err := json.NewDecoder(resp.Body).Decode(&producto); err != nil {
		return nil, err
	}

	return &producto, nil
}

func validarOrdenRequest(request OrdenRequest) error {
	if request.Email == "" {
		return errors.New("el email es obligatorio")
	}

	if _, err := mail.ParseAddress(request.Email); err != nil {
		return errors.New("el email no es válido")
	}

	if strings.TrimSpace(request.DireccionEnvio) == "" {
		return errors.New("la dirección de envío es obligatoria")
	}

	if strings.TrimSpace(request.Telefono) == "" {
		return errors.New("el teléfono es obligatorio")
	}

	if len(request.Productos) == 0 {
		return errors.New("la orden debe contener al menos un producto")
	}

	for _, producto := range request.Productos {
		if producto.ProductoID <= 0 {
			return errors.New(
				"el identificador del producto debe ser mayor que cero",
			)
		}

		if producto.Cantidad < 1 {
			return errors.New(
				"la cantidad del producto debe ser mayor o igual a 1",
			)
		}
	}

	return nil
}

func obtenerID(c *gin.Context) (int64, bool) {
	idString := c.Param("id")

	id, err := strconv.ParseInt(idString, 10, 64)

	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Mensaje: "El identificador de la orden es inválido",
		})
		return 0, false
	}

	return id, true
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
