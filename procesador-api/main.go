package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	facturasdb "procesador-api/mysql"
)

const (
	mqttBrokerDefault      = "tcp://localhost:1883"
	mqttTopicOrdenes       = "ordenes/para-procesar"
	ordenesAPIURLDefault   = "http://localhost:8081"
	productosAPIURLDefault = "http://localhost:8080"
)

const (
	EstadoCreated         = "Created"
	EstadoPendienteMQTT   = "Pendiente Mosquitto"
	EstadoProcessing      = "Processing"
	EstadoReadyToDelivery = "Ready to Delivery"
	EstadoNoStock         = "No Stock"
)

var estadosProcesables = map[string]bool{
	EstadoCreated:       true,
	EstadoPendienteMQTT: true,
}

var estadosProcesablesList = []string{EstadoCreated, EstadoPendienteMQTT}

var ordenesAPIURL = getEnv("ORDENES_API_URL", ordenesAPIURLDefault)
var productosAPIURL = getEnv("PRODUCTOS_API_URL", productosAPIURLDefault)
var mqttBrokerURL = getEnv("MQTT_BROKER_URL", mqttBrokerDefault)

var facturaRepo facturasdb.FacturaRepository

var httpClient = &http.Client{Timeout: 5 * time.Second}

type MensajeOrden struct {
	ID            int64  `json:"id"`
	Estado        string `json:"estado"`
	FechaCreacion string `json:"fechaCreacion"`
}

type Orden struct {
	ID             int64           `json:"id"`
	Email          string          `json:"email"`
	DireccionEnvio string          `json:"direccionEnvio"`
	Telefono       string          `json:"telefono"`
	Estado         string          `json:"estado"`
	FechaCreacion  string          `json:"fechaCreacion"`
	Productos      []ProductoOrden `json:"productos"`
}

type ProductoOrden struct {
	ProductoID int64
	Cantidad   int
}

type Producto struct {
	ID             int64    `json:"id"`
	Nombre         string   `json:"nombre"`
	Descripcion    string   `json:"descripcion"`
	PrecioUnitario float64  `json:"precioUnitario"`
	Stock          int      `json:"stock"`
	Imagenes       []string `json:"imagenes"`
}

func main() {
	conectarFacturasDB()

	client := conectarMQTT()

	if err := suscribirse(client); err != nil {
		fmt.Printf("Suscripción inicial pendiente: %v\n", err)
	}

	fmt.Println("======================================")
	fmt.Println("Procesador de órdenes")
	fmt.Println("======================================")
	fmt.Println("Suscripto a:", mqttTopicOrdenes)
	fmt.Println("Ordenes API:", ordenesAPIURL)
	fmt.Println("Productos API:", productosAPIURL)
	fmt.Println("======================================")

	go iniciarServidorHTTP()

	select {}
}

func conectarMQTT() mqtt.Client {
	opts := mqtt.NewClientOptions().
		AddBroker(mqttBrokerURL).
		SetClientID("procesador-api").
		SetKeepAlive(30 * time.Second).
		SetPingTimeout(10 * time.Second).
		SetConnectTimeout(3 * time.Second).
		SetConnectRetry(true).
		SetConnectRetryInterval(2 * time.Second).
		SetMaxReconnectInterval(10 * time.Second).
		SetAutoReconnect(true).
		SetCleanSession(false).
		SetResumeSubs(true).
		SetDefaultPublishHandler(onMessage).
		SetOnConnectHandler(func(c mqtt.Client) {
			fmt.Println("Conectado al broker MQTT en", mqttBrokerURL)
			if err := suscribirse(c); err != nil {
				fmt.Println("Aviso:", err)
			}
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			fmt.Printf("Se perdió la conexión con MQTT (%v). Las órdenes encoladas se recuperarán al reconectar.\n", err)
		}).
		SetReconnectingHandler(func(_ mqtt.Client, _ *mqtt.ClientOptions) {
			fmt.Println("Reconectando con el broker MQTT...")
		})

	client := mqtt.NewClient(opts)

	token := client.Connect()
	token.WaitTimeout(3 * time.Second)

	if token.Error() != nil {
		fmt.Printf("MQTT no disponible en %s (%v). Se reintentará en segundo plano.\n", mqttBrokerURL, token.Error())
	} else {
		fmt.Println("MQTT conectado en", mqttBrokerURL)
	}

	return client
}

func conectarFacturasDB() {
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "3306")
	dbName := getEnv("DB_NAME", "facturas")
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
		fmt.Printf("Esperando facturas-db... (%d/30)\n", i+1)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		panic("No se pudo conectar a facturas-db: " + err.Error())
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	facturaRepo = facturasdb.NewFacturaRepository(db)
	fmt.Println("Conectado a facturas-db")
}

func suscribirse(client mqtt.Client) error {
	var ultimoError error

	for i := 0; i < 30; i++ {
		token := client.Subscribe(mqttTopicOrdenes, 1, onMessage)
		token.WaitTimeout(3 * time.Second)
		if token.Error() == nil {
			fmt.Println("Suscripto al topic:", mqttTopicOrdenes)
			return nil
		}
		ultimoError = token.Error()
		fmt.Printf("Reintentando suscripción... (%d/30): %v\n", i+1, ultimoError)
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("no se pudo suscribir al topic %s: %w", mqttTopicOrdenes, ultimoError)
}

var onMessage mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	var m MensajeOrden
	if err := json.Unmarshal(msg.Payload(), &m); err != nil {
		fmt.Printf("Mensaje JSON inválido: %v\n", err)
		return
	}
	fmt.Printf("Mensaje recibido: id=%d estado=%s\n", m.ID, m.Estado)
	if err := procesar(m.ID); err != nil {
		fmt.Printf("Error procesando la orden %d: %v\n", m.ID, err)
	}
}

func procesar(id int64) error {
	orden, err := obtenerOrdenDeAPI(id)
	if err != nil {
		return err
	}

	if orden == nil {
		return fmt.Errorf("la orden %d no existe", id)
	}

	if !estadosProcesables[orden.Estado] {
		fmt.Printf("Orden %d ignorada: estado=%s\n", id, orden.Estado)
		return nil
	}

	factura, err := facturaRepo.ObtenerFacturaPorOrden(id)
	if err != nil {
		return fmt.Errorf("no se pudo verificar la factura de la orden %d: %v", id, err)
	}
	if factura != nil {
		fmt.Printf("Orden %d ignorada: ya tiene la factura %d\n", id, factura.ID)
		return nil
	}

	reclamada, err := reclamarOrden(id, EstadoProcessing)
	if err != nil {
		return fmt.Errorf("no se pudo reclamar la orden %d: %v", id, err)
	}
	if !reclamada {
		fmt.Printf("Orden %d ignorada: ya fue reclamada por otra entrega\n", id)
		return nil
	}

	fmt.Printf("Orden %d reclamada para procesar\n", id)

	detalles := make([]facturasdb.ItemFactura, 0, len(orden.Productos))

	for _, item := range orden.Productos {
		producto, err := obtenerProductoDeAPI(item.ProductoID)
		if err != nil {
			return err
		}

		if producto.Stock < item.Cantidad {
			fmt.Printf("Orden %d sin stock: producto %d (stock %d < %d)\n", id, item.ProductoID, producto.Stock, item.Cantidad)
			return cambiarEstadoOrden(id, EstadoNoStock)
		}

		detalles = append(detalles, facturasdb.ItemFactura{
			ProductoID:     item.ProductoID,
			Cantidad:       item.Cantidad,
			PrecioUnitario: producto.PrecioUnitario,
		})
	}

	for _, d := range detalles {
		if err := decrementarStockDeAPI(d.ProductoID, d.Cantidad); err != nil {
			return err
		}
	}

	fmt.Printf("Orden %d con stock: marcada como lista para entrega\n", id)
	if err := cambiarEstadoOrden(id, EstadoReadyToDelivery); err != nil {
		return err
	}

	return generarFactura(id, detalles)
}

func generarFactura(ordenID int64, items []facturasdb.ItemFactura) error {
	var total float64
	for _, item := range items {
		total += float64(item.Cantidad) * item.PrecioUnitario
	}

	id, err := facturaRepo.Guardar(ordenID, total, items)
	if err != nil {
		return fmt.Errorf("no se pudo generar la factura de la orden %d: %v", ordenID, err)
	}

	fmt.Printf("Factura %d generada para la orden %d (total %.2f)\n", id, ordenID, total)
	return nil
}

func peticion(method, url string, body []byte) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return httpClient.Do(req)
}

func obtenerOrdenDeAPI(id int64) (*Orden, error) {
	resp, err := peticion(http.MethodGet, ordenesAPIURL+"/api/ordenes/"+strconv.FormatInt(id, 10), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ordenes-api respondió HTTP %d", resp.StatusCode)
	}

	var o Orden
	if err := json.NewDecoder(resp.Body).Decode(&o); err != nil {
		return nil, err
	}
	return &o, nil
}

func obtenerProductoDeAPI(id int64) (*Producto, error) {
	resp, err := peticion(http.MethodGet, productosAPIURL+"/api/productos/"+strconv.FormatInt(id, 10), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("el producto %d no existe", id)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("productos-api respondió HTTP %d", resp.StatusCode)
	}

	var p Producto
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func reclamarOrden(id int64, hasta string) (bool, error) {
	body, err := json.Marshal(struct {
		Estado string   `json:"estado"`
		Desde  []string `json:"desde"`
	}{Estado: hasta, Desde: estadosProcesablesList})
	if err != nil {
		return false, err
	}

	resp, err := peticion(http.MethodPatch, ordenesAPIURL+"/api/ordenes/"+strconv.FormatInt(id, 10), body)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("no se pudo reclamar la orden %d (HTTP %d)", id, resp.StatusCode)
	}

	return true, nil
}

func cambiarEstadoOrden(id int64, estado string) error {
	body, err := json.Marshal(struct {
		Estado string `json:"estado"`
	}{Estado: estado})
	if err != nil {
		return err
	}

	resp, err := peticion(http.MethodPatch, ordenesAPIURL+"/api/ordenes/"+strconv.FormatInt(id, 10), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("no se pudo actualizar la orden %d a %s (HTTP %d)", id, estado, resp.StatusCode)
	}

	fmt.Printf("Orden %d actualizada a %s\n", id, estado)
	return nil
}

func decrementarStockDeAPI(id int64, cantidad int) error {
	body, err := json.Marshal(struct {
		Cantidad int `json:"cantidad"`
	}{Cantidad: cantidad})
	if err != nil {
		return err
	}

	resp, err := peticion(http.MethodPost, productosAPIURL+"/api/productos/"+strconv.FormatInt(id, 10)+"/decrementar-stock", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("no se pudo decrementar el stock del producto %d (HTTP %d)", id, resp.StatusCode)
	}

	fmt.Printf("Stock del producto %d decrementado en %d\n", id, cantidad)
	return nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func iniciarServidorHTTP() {
	router := gin.Default()

	router.GET("/api/facturas", handlerListarFacturas)
	router.GET("/api/facturas/:id", handlerObtenerFactura)
	router.GET("/api/ordenes/:ordenId/factura", handlerFacturaPorOrden)

	addr := ":" + getEnv("PORT", "8082")
	fmt.Println("Servidor de facturas en http://localhost" + addr)
	if err := router.Run(addr); err != nil {
		fmt.Println("Error en el servidor HTTP de facturas:", err)
	}
}

func handlerListarFacturas(c *gin.Context) {
	facturas, err := facturaRepo.ListarFacturas()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"mensaje": "Error al listar las facturas: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, facturas)
}

func handlerObtenerFactura(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"mensaje": "El identificador de la factura es inválido"})
		return
	}

	factura, err := facturaRepo.ObtenerFactura(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"mensaje": "Error al obtener la factura: " + err.Error()})
		return
	}

	if factura == nil {
		c.JSON(http.StatusNotFound, gin.H{"mensaje": "La factura no existe"})
		return
	}

	c.JSON(http.StatusOK, factura)
}

func handlerFacturaPorOrden(c *gin.Context) {
	ordenID, err := strconv.ParseInt(c.Param("ordenId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"mensaje": "El identificador de la orden es inválido"})
		return
	}

	factura, err := facturaRepo.ObtenerFacturaPorOrden(ordenID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"mensaje": "Error al obtener la factura: " + err.Error()})
		return
	}

	if factura == nil {
		c.JSON(http.StatusNotFound, gin.H{"mensaje": "La orden no tiene factura asociada"})
		return
	}

	c.JSON(http.StatusOK, factura)
}