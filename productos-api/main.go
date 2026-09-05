package main

import (
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
)

type Producto struct {
	ID             int64    `json:"id"`
	Nombre         string   `json:"nombre"`
	Descripcion    string   `json:"descripcion"`
	PrecioUnitario float64  `json:"precioUnitario"`
	Stock          int      `json:"stock"`
	Imagenes       []string `json:"imagenes"`
}

type ProductoRequest struct {
	Nombre         string   `json:"nombre"`
	Descripcion    string   `json:"descripcion"`
	PrecioUnitario float64  `json:"precioUnitario"`
	Stock          int      `json:"stock"`
	Imagenes       []string `json:"imagenes"`
}

type ProductoPatch struct {
	Nombre         *string   `json:"nombre"`
	Descripcion    *string   `json:"descripcion"`
	PrecioUnitario *float64  `json:"precioUnitario"`
	Stock          *int      `json:"stock"`
	Imagenes       *[]string `json:"imagenes"`
}

type IdResponse struct {
	ID int64 `json:"id"`
}

type MessageResponse struct {
	Mensaje string `json:"mensaje"`
}

type ErrorResponse struct {
	Mensaje string `json:"mensaje"`
}

var productos = []Producto{
	{
		ID:             1,
		Nombre:         "Notebook Lenovo ThinkPad",
		Descripcion:    "Notebook Intel i7 16GB RAM",
		PrecioUnitario: 1250.50,
		Stock:          15,
		Imagenes: []string{
			"https://ejemplo.com/imagenes/notebook1.jpg",
			"https://ejemplo.com/imagenes/notebook2.jpg",
		},
	},
	{
		ID:             2,
		Nombre:         "Mouse Logitech",
		Descripcion:    "Mouse inalámbrico Logitech",
		PrecioUnitario: 35.90,
		Stock:          50,
		Imagenes: []string{
			"https://ejemplo.com/imagenes/mouse1.jpg",
		},
	},
}

var mutex sync.RWMutex

var siguienteID int64 = 3

func main() {
	router := gin.Default()

	router.GET("/api/productos", obtenerProductos)

	router.POST("/api/productos", crearProducto)

	router.GET("/api/productos/:id", obtenerProductoPorID)

	router.PUT("/api/productos/:id", actualizarProducto)

	router.PATCH("/api/productos/:id", modificarProducto)

	puerto := os.Getenv("PORT")
	if puerto == "" {
		puerto = "8080"
	}

	if err := router.Run(":" + puerto); err != nil {
		panic(err)
	}
}

func obtenerProductos(c *gin.Context) {
	mutex.RLock()
	defer mutex.RUnlock()

	c.JSON(http.StatusOK, productos)
}

func crearProducto(c *gin.Context) {
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

	mutex.Lock()
	defer mutex.Unlock()

	producto := Producto{
		ID:             siguienteID,
		Nombre:         request.Nombre,
		Descripcion:    request.Descripcion,
		PrecioUnitario: request.PrecioUnitario,
		Stock:          request.Stock,
		Imagenes:       request.Imagenes,
	}

	productos = append(productos, producto)
	siguienteID++

	c.JSON(http.StatusCreated, IdResponse{
		ID: producto.ID,
	})
}

func obtenerProductoPorID(c *gin.Context) {
	id, ok := obtenerID(c)

	if !ok {
		return
	}

	mutex.RLock()
	defer mutex.RUnlock()

	for _, producto := range productos {
		if producto.ID == id {
			c.JSON(http.StatusOK, producto)
			return
		}
	}

	c.JSON(http.StatusNotFound, ErrorResponse{
		Mensaje: "No existe un producto con el identificador " + strconv.FormatInt(id, 10),
	})
}

func actualizarProducto(c *gin.Context) {
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

	mutex.Lock()
	defer mutex.Unlock()

	for i := range productos {
		if productos[i].ID == id {
			productos[i].Nombre = request.Nombre
			productos[i].Descripcion = request.Descripcion
			productos[i].PrecioUnitario = request.PrecioUnitario
			productos[i].Stock = request.Stock
			productos[i].Imagenes = request.Imagenes

			c.JSON(http.StatusOK, MessageResponse{
				Mensaje: "Producto actualizado correctamente",
			})
			return
		}
	}

	c.JSON(http.StatusNotFound, ErrorResponse{
		Mensaje: "No existe un producto con el identificador " + strconv.FormatInt(id, 10),
	})
}

func modificarProducto(c *gin.Context) {
	id, ok := obtenerID(c)

	if !ok {
		return
	}

	var request ProductoPatch

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

	mutex.Lock()
	defer mutex.Unlock()

	for i := range productos {
		if productos[i].ID == id {

			if request.Nombre != nil {
				productos[i].Nombre = *request.Nombre
			}

			if request.Descripcion != nil {
				productos[i].Descripcion = *request.Descripcion
			}

			if request.PrecioUnitario != nil {
				productos[i].PrecioUnitario = *request.PrecioUnitario
			}

			if request.Stock != nil {
				productos[i].Stock = *request.Stock
			}

			if request.Imagenes != nil {
				productos[i].Imagenes = *request.Imagenes
			}

			c.JSON(http.StatusOK, MessageResponse{
				Mensaje: "Producto modificado correctamente",
			})
			return
		}
	}

	c.JSON(http.StatusNotFound, ErrorResponse{
		Mensaje: "No existe un producto con el identificador " + strconv.FormatInt(id, 10),
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

func validarProductoPatch(request ProductoPatch) error {
	if request.PrecioUnitario != nil && *request.PrecioUnitario < 0 {
		return errorString("El precio unitario no puede ser negativo")
	}

	if request.Stock != nil && *request.Stock < 0 {
		return errorString("El stock no puede ser negativo")
	}

	return nil
}

type errorString string

func (e errorString) Error() string {
	return string(e)
}