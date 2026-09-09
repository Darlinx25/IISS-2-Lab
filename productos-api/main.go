package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	productosdb "productos-api/mysql"
)

type ProductoRequest struct {
	Nombre         string   `json:"nombre"`
	Descripcion    string   `json:"descripcion"`
	PrecioUnitario float64  `json:"precioUnitario"`
	Stock          int      `json:"stock"`
	Imagenes       []string `json:"imagenes"`
}

type IdResponse struct {
	ID int64 `json:"id"`
}

type ErrorResponse struct {
	Mensaje string `json:"mensaje"`
}

type MessageResponse struct {
	Mensaje string `json:"mensaje"`
}

type Handlers struct {
	repo productosdb.ProductoRepository
}

func main() {
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "3306")
	dbName := getEnv("DB_NAME", "productos")
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

	repo := productosdb.NewProductoRepository(db)
	h := &Handlers{repo: repo}

	router := gin.Default()

	router.GET("/api/productos", h.obtenerProductos)
	router.POST("/api/productos", h.crearProducto)
	router.GET("/api/productos/:id", h.obtenerProductoPorID)
	router.PUT("/api/productos/:id", h.actualizarProducto)
	router.PATCH("/api/productos/:id", h.modificarProducto)

	puerto := os.Getenv("PORT")
	if puerto == "" {
		puerto = "8080"
	}

	if err := router.Run(":" + puerto); err != nil {
		panic(err)
	}
}

func (h *Handlers) obtenerProductos(c *gin.Context) {
	productos, err := h.repo.ObtenerTodos()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Mensaje: "Error al obtener los productos",
		})
		return
	}
	c.JSON(http.StatusOK, productos)
}

func (h *Handlers) crearProducto(c *gin.Context) {
	var request ProductoRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Mensaje: "Los datos del producto son inválidos",
		})
		return
	}

	if err := validarProductoRequest(request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Mensaje: err.Error(),
		})
		return
	}

	producto := productosdb.Producto{
		Nombre:         request.Nombre,
		Descripcion:    request.Descripcion,
		PrecioUnitario: request.PrecioUnitario,
		Stock:          request.Stock,
		Imagenes:       request.Imagenes,
	}

	id, err := h.repo.Crear(producto)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Mensaje: "Error al crear el producto",
		})
		return
	}

	c.JSON(http.StatusCreated, IdResponse{ID: id})
}

func (h *Handlers) obtenerProductoPorID(c *gin.Context) {
	id, ok := obtenerID(c)
	if !ok {
		return
	}

	producto, err := h.repo.ObtenerPorID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Mensaje: "Error al obtener el producto",
		})
		return
	}

	if producto == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Mensaje: "No existe un producto con el identificador " + strconv.FormatInt(id, 10),
		})
		return
	}

	c.JSON(http.StatusOK, producto)
}

func (h *Handlers) actualizarProducto(c *gin.Context) {
	id, ok := obtenerID(c)
	if !ok {
		return
	}

	var request ProductoRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Mensaje: "Los datos del producto son inválidos",
		})
		return
	}

	if err := validarProductoRequest(request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Mensaje: err.Error(),
		})
		return
	}

	producto := productosdb.Producto{
		Nombre:         request.Nombre,
		Descripcion:    request.Descripcion,
		PrecioUnitario: request.PrecioUnitario,
		Stock:          request.Stock,
		Imagenes:       request.Imagenes,
	}

	err := h.repo.Actualizar(id, producto)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Mensaje: "No existe un producto con el identificador " + strconv.FormatInt(id, 10),
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Mensaje: "Error al actualizar el producto",
		})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{
		Mensaje: "Producto actualizado correctamente",
	})
}

func (h *Handlers) modificarProducto(c *gin.Context) {
	id, ok := obtenerID(c)
	if !ok {
		return
	}

	var request struct {
		Nombre         *string   `json:"nombre"`
		Descripcion    *string   `json:"descripcion"`
		PrecioUnitario *float64  `json:"precioUnitario"`
		Stock          *int      `json:"stock"`
		Imagenes       *[]string `json:"imagenes"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Mensaje: "Los datos del producto son inválidos",
		})
		return
	}

	if err := validarProductoPatch(request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Mensaje: err.Error(),
		})
		return
	}

	patch := productosdb.ProductoPatch{
		Nombre:         request.Nombre,
		Descripcion:    request.Descripcion,
		PrecioUnitario: request.PrecioUnitario,
		Stock:          request.Stock,
		Imagenes:       request.Imagenes,
	}

	err := h.repo.Modificar(id, patch)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Mensaje: "No existe un producto con el identificador " + strconv.FormatInt(id, 10),
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Mensaje: "Error al modificar el producto",
		})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{
		Mensaje: "Producto modificado correctamente",
	})
}

func obtenerID(c *gin.Context) (int64, bool) {
	idString := c.Param("id")

	id, err := strconv.ParseInt(idString, 10, 64)

	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Mensaje: "El identificador del producto es inválido",
		})
		return 0, false
	}

	return id, true
}

func validarProductoRequest(request ProductoRequest) error {
	if request.Nombre == "" {
		return errorString("El nombre del producto es obligatorio")
	}

	if request.Descripcion == "" {
		return errorString("La descripción del producto es obligatoria")
	}

	if request.PrecioUnitario < 0 {
		return errorString("El precio unitario no puede ser negativo")
	}

	if request.Stock < 0 {
		return errorString("El stock no puede ser negativo")
	}

	if request.Imagenes == nil {
		return errorString("El campo imagenes es obligatorio")
	}

	return nil
}

func validarProductoPatch(request struct {
	Nombre         *string   `json:"nombre"`
	Descripcion    *string   `json:"descripcion"`
	PrecioUnitario *float64  `json:"precioUnitario"`
	Stock          *int      `json:"stock"`
	Imagenes       *[]string `json:"imagenes"`
}) error {
	if request.PrecioUnitario != nil && *request.PrecioUnitario < 0 {
		return errorString("El precio unitario no puede ser negativo")
	}

	if request.Stock != nil && *request.Stock < 0 {
		return errorString("El stock no puede ser negativo")
	}

	return nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

type errorString string

func (e errorString) Error() string {
	return string(e)
}
