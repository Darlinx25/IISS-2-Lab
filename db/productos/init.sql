CREATE TABLE IF NOT EXISTS productos (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    nombre VARCHAR(255) NOT NULL,
    descripcion TEXT NOT NULL,
    precio_unitario DECIMAL(10,2) NOT NULL DEFAULT 0,
    stock INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS producto_imagenes (
    producto_id BIGINT NOT NULL,
    imagen_url VARCHAR(500) NOT NULL,
    PRIMARY KEY (producto_id, imagen_url),
    FOREIGN KEY (producto_id) REFERENCES productos(id) ON DELETE CASCADE
);

INSERT INTO productos (id, nombre, descripcion, precio_unitario, stock) VALUES
(1, 'Notebook Lenovo ThinkPad', 'Notebook Intel i7 16GB RAM', 1250.50, 15),
(2, 'Mouse Logitech', 'Mouse inalámbrico Logitech', 35.90, 50);

INSERT INTO producto_imagenes (producto_id, imagen_url) VALUES
(1, 'https://ejemplo.com/imagenes/notebook1.jpg'),
(1, 'https://ejemplo.com/imagenes/notebook2.jpg'),
(2, 'https://ejemplo.com/imagenes/mouse1.jpg');

ALTER TABLE productos AUTO_INCREMENT = 3;
