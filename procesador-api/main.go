package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	mqttBrokerDefault    = "tcp://localhost:1883"
	mqttTopicOrdenes     = "ordenes/para-procesar"
	ordenesAPIURLDefault  = "http://localhost:8081"
	productosAPIURLDefault = "http://localhost:8080"
)

const (
	EstadoCreated         = "Created"
	EstadoReadyToDelivery = "Ready to Delivery"
	EstadoNoStock         = "No Stock"
)

var ordenesAPIURL = getEnv("ORDENES_API_URL", ordenesAPIURLDefault)
var productosAPIURL = getEnv("PRODUCTOS_API_URL", productosAPIURLDefault)
var mqttBrokerURL = getEnv("MQTT_BROKER_URL", mqttBrokerDefault)

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
	client := conectarMQTT()
	suscribirse(client)

	fmt.Println("======================================")
	fmt.Println("Procesador de órdenes")
	fmt.Println("======================================")
	fmt.Println("Suscripto a:", mqttTopicOrdenes)
	fmt.Println("Ordenes API:", ordenesAPIURL)
	fmt.Println("Productos API:", productosAPIURL)
	fmt.Println("======================================")

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
		SetConnectRetryInterval(2 * time.Second)

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

func suscribirse(client mqtt.Client) {
	for i := 0; i < 30; i++ {
		token := client.Subscribe(mqttTopicOrdenes, 1, onMessage)
		token.WaitTimeout(3 * time.Second)
		if token.Error() == nil {
			fmt.Println("Suscripto al topic:", mqttTopicOrdenes)
			return
		}
		fmt.Printf("Reintentando suscripción... (%d/30): %v\n", i+1, token.Error())
		time.Sleep(2 * time.Second)
	}
	panic("No se pudo suscribir al topic " + mqttTopicOrdenes)
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

	if orden.Estado != EstadoCreated {
		fmt.Printf("Orden %d ignorada: ya procesada (estado=%s)\n", id, orden.Estado)
		return nil
	}

	for _, item := range orden.Productos {
		producto, err := obtenerProductoDeAPI(item.ProductoID)
		if err != nil {
			return err
		}

		if producto.Stock < item.Cantidad {
			fmt.Printf("Orden %d sin stock: producto %d (stock %d < %d)\n", id, item.ProductoID, producto.Stock, item.Cantidad)
			return cambiarEstadoOrden(id, EstadoNoStock)
		}
	}

	for _, item := range orden.Productos {
		if err := decrementarStockDeAPI(item.ProductoID, item.Cantidad); err != nil {
			return err
		}
	}

	fmt.Printf("Orden %d con stock: marcada como lista para entrega\n", id)
	return cambiarEstadoOrden(id, EstadoReadyToDelivery)
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