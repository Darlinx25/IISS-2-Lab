package mysql

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

type ProductoOrden struct {
	ProductoID int64
	Cantidad   int
}

type Orden struct {
	ID             int64
	Email          string
	DireccionEnvio string
	Telefono       string
	Estado         string
	FechaCreacion  string
	Productos      []ProductoOrden
}

type OrdenRepository interface {
	ObtenerTodas() ([]Orden, error)
	ObtenerPorID(id int64) (*Orden, error)
	Crear(o Orden) (int64, error)
}

type mysqlOrdenRepository struct {
	db *sql.DB
}

func NewOrdenRepository(db *sql.DB) OrdenRepository {
	return &mysqlOrdenRepository{db: db}
}

func (r *mysqlOrdenRepository) ObtenerTodas() ([]Orden, error) {
	rows, err := r.db.Query(
		"SELECT id, email, direccion_envio, telefono, estado, fecha_creacion FROM ordenes",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ordenes []Orden
	for rows.Next() {
		var o Orden
		if err := rows.Scan(&o.ID, &o.Email, &o.DireccionEnvio, &o.Telefono, &o.Estado, &o.FechaCreacion); err != nil {
			return nil, err
		}
		productos, err := r.obtenerProductosOrden(o.ID)
		if err != nil {
			return nil, err
		}
		o.Productos = productos
		ordenes = append(ordenes, o)
	}
	return ordenes, nil
}

func (r *mysqlOrdenRepository) ObtenerPorID(id int64) (*Orden, error) {
	var o Orden
	err := r.db.QueryRow(
		"SELECT id, email, direccion_envio, telefono, estado, fecha_creacion FROM ordenes WHERE id = ?", id,
	).Scan(&o.ID, &o.Email, &o.DireccionEnvio, &o.Telefono, &o.Estado, &o.FechaCreacion)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	productos, err := r.obtenerProductosOrden(o.ID)
	if err != nil {
		return nil, err
	}
	o.Productos = productos
	return &o, nil
}

func (r *mysqlOrdenRepository) Crear(o Orden) (int64, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		"INSERT INTO ordenes (email, direccion_envio, telefono, estado, fecha_creacion) VALUES (?, ?, ?, ?, ?)",
		o.Email, o.DireccionEnvio, o.Telefono, o.Estado, o.FechaCreacion,
	)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	if len(o.Productos) > 0 {
		if err := r.insertarProductosOrdenTx(tx, id, o.Productos); err != nil {
			return 0, err
		}
	}

	return id, tx.Commit()
}

func (r *mysqlOrdenRepository) obtenerProductosOrden(ordenID int64) ([]ProductoOrden, error) {
	rows, err := r.db.Query(
		"SELECT producto_id, cantidad FROM orden_productos WHERE orden_id = ?", ordenID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var productos []ProductoOrden
	for rows.Next() {
		var p ProductoOrden
		if err := rows.Scan(&p.ProductoID, &p.Cantidad); err != nil {
			return nil, fmt.Errorf("error scanning orden producto: %w", err)
		}
		productos = append(productos, p)
	}
	return productos, nil
}

func (r *mysqlOrdenRepository) insertarProductosOrdenTx(tx *sql.Tx, ordenID int64, productos []ProductoOrden) error {
	stmt, err := tx.Prepare("INSERT INTO orden_productos (orden_id, producto_id, cantidad) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, p := range productos {
		if _, err := stmt.Exec(ordenID, p.ProductoID, p.Cantidad); err != nil {
			return err
		}
	}
	return nil
}
