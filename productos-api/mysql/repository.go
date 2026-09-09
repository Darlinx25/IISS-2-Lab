package mysql

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

type Producto struct {
	ID             int64
	Nombre         string
	Descripcion    string
	PrecioUnitario float64
	Stock          int
	Imagenes       []string
}

type ProductoPatch struct {
	Nombre         *string
	Descripcion    *string
	PrecioUnitario *float64
	Stock          *int
	Imagenes       *[]string
}

type ProductoRepository interface {
	ObtenerTodos() ([]Producto, error)
	ObtenerPorID(id int64) (*Producto, error)
	Crear(p Producto) (int64, error)
	Actualizar(id int64, p Producto) error
	Modificar(id int64, patch ProductoPatch) error
}

type mysqlProductoRepository struct {
	db *sql.DB
}

func NewProductoRepository(db *sql.DB) ProductoRepository {
	return &mysqlProductoRepository{db: db}
}

func (r *mysqlProductoRepository) ObtenerTodos() ([]Producto, error) {
	rows, err := r.db.Query("SELECT id, nombre, descripcion, precio_unitario, stock FROM productos")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var productos []Producto
	for rows.Next() {
		var p Producto
		if err := rows.Scan(&p.ID, &p.Nombre, &p.Descripcion, &p.PrecioUnitario, &p.Stock); err != nil {
			return nil, err
		}
		imagenes, err := r.obtenerImagenes(p.ID)
		if err != nil {
			return nil, err
		}
		p.Imagenes = imagenes
		productos = append(productos, p)
	}
	return productos, nil
}

func (r *mysqlProductoRepository) ObtenerPorID(id int64) (*Producto, error) {
	var p Producto
	err := r.db.QueryRow(
		"SELECT id, nombre, descripcion, precio_unitario, stock FROM productos WHERE id = ?", id,
	).Scan(&p.ID, &p.Nombre, &p.Descripcion, &p.PrecioUnitario, &p.Stock)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	imagenes, err := r.obtenerImagenes(p.ID)
	if err != nil {
		return nil, err
	}
	p.Imagenes = imagenes
	return &p, nil
}

func (r *mysqlProductoRepository) Crear(p Producto) (int64, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		"INSERT INTO productos (nombre, descripcion, precio_unitario, stock) VALUES (?, ?, ?, ?)",
		p.Nombre, p.Descripcion, p.PrecioUnitario, p.Stock,
	)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	if len(p.Imagenes) > 0 {
		if err := r.insertarImagenesTx(tx, id, p.Imagenes); err != nil {
			return 0, err
		}
	}

	return id, tx.Commit()
}

func (r *mysqlProductoRepository) Actualizar(id int64, p Producto) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		"UPDATE productos SET nombre = ?, descripcion = ?, precio_unitario = ?, stock = ? WHERE id = ?",
		p.Nombre, p.Descripcion, p.PrecioUnitario, p.Stock, id,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}

	if _, err := tx.Exec("DELETE FROM producto_imagenes WHERE producto_id = ?", id); err != nil {
		return err
	}

	if len(p.Imagenes) > 0 {
		if err := r.insertarImagenesTx(tx, id, p.Imagenes); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *mysqlProductoRepository) Modificar(id int64, patch ProductoPatch) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	setClauses := []string{}
	args := []interface{}{}

	if patch.Nombre != nil {
		setClauses = append(setClauses, "nombre = ?")
		args = append(args, *patch.Nombre)
	}
	if patch.Descripcion != nil {
		setClauses = append(setClauses, "descripcion = ?")
		args = append(args, *patch.Descripcion)
	}
	if patch.PrecioUnitario != nil {
		setClauses = append(setClauses, "precio_unitario = ?")
		args = append(args, *patch.PrecioUnitario)
	}
	if patch.Stock != nil {
		setClauses = append(setClauses, "stock = ?")
		args = append(args, *patch.Stock)
	}

	if len(setClauses) > 0 {
		query := "UPDATE productos SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
		args = append(args, id)
		result, err := tx.Exec(query, args...)
		if err != nil {
			return err
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return sql.ErrNoRows
		}
	}

	if patch.Imagenes != nil {
		if _, err := tx.Exec("DELETE FROM producto_imagenes WHERE producto_id = ?", id); err != nil {
			return err
		}
		if len(*patch.Imagenes) > 0 {
			if err := r.insertarImagenesTx(tx, id, *patch.Imagenes); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (r *mysqlProductoRepository) obtenerImagenes(productoID int64) ([]string, error) {
	rows, err := r.db.Query(
		"SELECT imagen_url FROM producto_imagenes WHERE producto_id = ? ORDER BY imagen_url", productoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var imagenes []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		imagenes = append(imagenes, url)
	}
	return imagenes, nil
}

func (r *mysqlProductoRepository) insertarImagenesTx(tx *sql.Tx, productoID int64, imagenes []string) error {
	stmt, err := tx.Prepare("INSERT INTO producto_imagenes (producto_id, imagen_url) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("error preparing insert imagenes: %w", err)
	}
	defer stmt.Close()

	for _, url := range imagenes {
		if _, err := stmt.Exec(productoID, url); err != nil {
			return err
		}
	}
	return nil
}
