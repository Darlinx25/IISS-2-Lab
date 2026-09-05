package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const productosAPIURLDefault = "http://localhost:8080"

var productosAPIURL = func() string {
	if url := os.Getenv("PRODUCTOS_API_URL"); url != "" {
		return url
	}
	return productosAPIURLDefault
}()

type ProductoOrden struct {
	ProductoID int64 `json:"productoId"`
	Cantidad   int   `json:"cantidad"`
}

type Orden struct {
	ID             int64           `json:"id"`
	Email          string          `json:"email"`
	DireccionEnvio string          `json:"direccionEnvio"`
	Telefono       string          `json:"telefono"`
	Estado         string          `json:"estado"`
	FechaCreacion  time.Time       `json:"fechaCreacion"`
	Productos      []ProductoOrden `json:"productos"`
}

type OrdenRequest struct {
	Email          string          `json:"email"`
	DireccionEnvio string          `json:"direccionEnvio"`
	Telefono       string          `json:"telefono"`
	Productos      []ProductoOrden `json:"productos"`
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
	FechaCreacion  time.Time         `json:"fechaCreacion"`
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

var ordenes = []Orden{
	{
		ID:             1001,
		Email:          "cliente@email.com",
		DireccionEnvio: "Av. Italia 3333, Maldonado",
		Telefono:       "+59899111222",
		Estado:         EstadoCreated,
		FechaCreacion:  time.Now(),
		Productos: []ProductoOrden{
			{
				ProductoID: 1,
				Cantidad:   2,
			},
		},
	},
}

var siguienteID int64 = 1002

var mutex sync.RWMutex

func main() {
	router := gin.Default()

	router.GET("/api/ordenes", obtenerOrdenes)

	router.POST("/api/ordenes", crearOrden)

	router.GET("/api/ordenes/:id", obtenerOrdenPorID)

	router.GET("/api/ordenes/:id/detalle", obtenerDetalleOrden)

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

func obtenerOrdenes(c *gin.Context) {
	mutex.RLock()
	defer mutex.RUnlock()

	c.JSON(http.StatusOK, ordenes)
}

func crearOrden(c *gin.Context) {
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

	mutex.Lock()
	defer mutex.Unlock()

	orden := Orden{
		ID:             siguienteID,
		Email:          request.Email,
		DireccionEnvio: request.DireccionEnvio,
		Telefono:       request.Telefono,
		Estado:         EstadoCreated,
		FechaCreacion:  time.Now(),
		Productos:      request.Productos,
	}

	ordenes = append(ordenes, orden)

	siguienteID++

	c.JSON(http.StatusCreated, IdResponse{
		ID: orden.ID,
	})
}

func obtenerOrdenPorID(c *gin.Context) {
	id, ok := obtenerID(c)

	if !ok {
		return
	}

	mutex.RLock()
	defer mutex.RUnlock()

	for _, orden := range ordenes {
		if orden.ID == id {
			c.JSON(http.StatusOK, orden)
			return
		}
	}

	c.JSON(http.StatusNotFound, ErrorResponse{
		Mensaje: "No existe una orden con el identificador " +
			strconv.FormatInt(id, 10),
	})
}

func obtenerDetalleOrden(c *gin.Context) {
	id, ok := obtenerID(c)

	if !ok {
		return
	}

	mutex.RLock()

	var ordenEncontrada *Orden

	for i := range ordenes {
		if ordenes[i].ID == id {
			orden := ordenes[i]
			ordenEncontrada = &orden
			break
		}
	}

	mutex.RUnlock()

	if ordenEncontrada == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Mensaje: "No existe una orden con el identificador " +
				strconv.FormatInt(id, 10),
		})
		return
	}

	productosDetalle := make([]ProductoDetalle, 0)

	for _, item := range ordenEncontrada.Productos {

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
		ID:             ordenEncontrada.ID,
		Email:          ordenEncontrada.Email,
		DireccionEnvio: ordenEncontrada.DireccionEnvio,
		Telefono:       ordenEncontrada.Telefono,
		Estado:         ordenEncontrada.Estado,
		FechaCreacion:  ordenEncontrada.FechaCreacion,
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