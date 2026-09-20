package mysql

import (
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type ItemFactura struct {
	ProductoID     int64   `json:"productoId"`
	Cantidad       int     `json:"cantidad"`
	PrecioUnitario float64 `json:"precioUnitario"`
}

type Factura struct {
	ID           int64         `json:"id"`
	OrdenID      int64         `json:"ordenId"`
	MontoTotal   float64       `json:"montoTotal"`
	FechaEmision time.Time     `json:"fechaEmision"`
	Items        []ItemFactura `json:"items"`
}

type FacturaRepository interface {
	Guardar(ordenID int64, montoTotal float64, items []ItemFactura) (int64, error)
	ListarFacturas() ([]Factura, error)
	ObtenerFactura(id int64) (*Factura, error)
	ObtenerFacturaPorOrden(ordenID int64) (*Factura, error)
}

type mysqlFacturaRepository struct {
	db *sql.DB
}

func NewFacturaRepository(db *sql.DB) FacturaRepository {
	return &mysqlFacturaRepository{db: db}
}

func (r *mysqlFacturaRepository) Guardar(ordenID int64, montoTotal float64, items []ItemFactura) (int64, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		"INSERT INTO facturas (orden_id, monto_total) VALUES (?, ?)",
		ordenID, montoTotal,
	)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	stmt, err := tx.Prepare(
		"INSERT INTO factura_items (factura_id, producto_id, cantidad, precio_unitario) VALUES (?, ?, ?, ?)",
	)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	for _, item := range items {
		if _, err := stmt.Exec(id, item.ProductoID, item.Cantidad, item.PrecioUnitario); err != nil {
			return 0, err
		}
	}

	return id, tx.Commit()
}

func (r *mysqlFacturaRepository) ListarFacturas() ([]Factura, error) {
	rows, err := r.db.Query("SELECT id, orden_id, monto_total, fecha_emision FROM facturas ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facturas []Factura
	for rows.Next() {
		var f Factura
		if err := rows.Scan(&f.ID, &f.OrdenID, &f.MontoTotal, &f.FechaEmision); err != nil {
			return nil, err
		}
		items, err := r.cargarItems(f.ID)
		if err != nil {
			return nil, err
		}
		f.Items = items
		facturas = append(facturas, f)
	}
	return facturas, rows.Err()
}

func (r *mysqlFacturaRepository) ObtenerFactura(id int64) (*Factura, error) {
	var f Factura
	err := r.db.QueryRow(
		"SELECT id, orden_id, monto_total, fecha_emision FROM facturas WHERE id = ?", id,
	).Scan(&f.ID, &f.OrdenID, &f.MontoTotal, &f.FechaEmision)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items, err := r.cargarItems(f.ID)
	if err != nil {
		return nil, err
	}
	f.Items = items
	return &f, nil
}

func (r *mysqlFacturaRepository) ObtenerFacturaPorOrden(ordenID int64) (*Factura, error) {
	var f Factura
	err := r.db.QueryRow(
		"SELECT id, orden_id, monto_total, fecha_emision FROM facturas WHERE orden_id = ?", ordenID,
	).Scan(&f.ID, &f.OrdenID, &f.MontoTotal, &f.FechaEmision)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items, err := r.cargarItems(f.ID)
	if err != nil {
		return nil, err
	}
	f.Items = items
	return &f, nil
}

func (r *mysqlFacturaRepository) cargarItems(facturaID int64) ([]ItemFactura, error) {
	rows, err := r.db.Query(
		"SELECT producto_id, cantidad, precio_unitario FROM factura_items WHERE factura_id = ?",
		facturaID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ItemFactura
	for rows.Next() {
		var it ItemFactura
		if err := rows.Scan(&it.ProductoID, &it.Cantidad, &it.PrecioUnitario); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}