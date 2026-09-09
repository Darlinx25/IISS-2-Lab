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

	ordenesdb "ordenes-api/mysql"
)

const productosAPIURLDefault = "http://localhost:8080"

var productosAPIURL = func() string {
	if url := os.Getenv("PRODUCTOS_API_URL"); url != "" {
		return url
	}
	return productosAPIURLDefault
}()

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

type IdResponse struct {
	ID int64 `json:"id"`
}

type ErrorResponse struct {
	Mensaje string `json:"mensaje"`
}

const (
	EstadoCreated    = "Created"
	EstadoConfirmed  = "Confirmed"
	EstadoProcessing = "Processing"
	EstadoShipped    = "Shipped"
	EstadoDelivered  = "Delivered"
	EstadoCancelled  = "Cancelled"
)

type Handlers struct {
	repo ordenesdb.OrdenRepository
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
	h := &Handlers{repo: repo}

	router := gin.Default()

	router.GET("/api/ordenes", h.obtenerOrdenes)
	router.POST("/api/ordenes", h.crearOrden)
	router.GET("/api/ordenes/:id", h.obtenerOrdenPorID)
	router.GET("/api/ordenes/:id/detalle", h.obtenerDetalleOrden)

	fmt.Println("======================================")
	fmt.Println("API de Órdenes")
	fmt.Println("======================================")
	fmt.Println("Servidor escuchando en:")
	fmt.Println("http://localhost:8081")
	fmt.Println("======================================")

	puerto := os.Getenv("PORT")
	if puerto == "" {
		puerto = "8081"
	}

	if err := router.Run(":" + puerto); err != nil {
		panic(err)
	}
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

	orden := ordenesdb.Orden{
		Email:          request.Email,
		DireccionEnvio: request.DireccionEnvio,
		Telefono:       request.Telefono,
		Estado:         EstadoCreated,
		FechaCreacion:  time.Now().Format("2006-01-02 15:04:05"),
		Productos:      request.Productos,
	}

	id, err := h.repo.Crear(orden)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Mensaje: "Error al crear la orden",
		})
		return
	}

	c.JSON(http.StatusCreated, IdResponse{ID: id})
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
