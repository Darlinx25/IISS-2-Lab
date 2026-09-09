CREATE TABLE IF NOT EXISTS ordenes (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    direccion_envio VARCHAR(500) NOT NULL,
    telefono VARCHAR(50) NOT NULL,
    estado VARCHAR(50) NOT NULL DEFAULT 'Created',
    fecha_creacion DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS orden_productos (
    orden_id BIGINT NOT NULL,
    producto_id BIGINT NOT NULL,
    cantidad INT NOT NULL DEFAULT 1,
    PRIMARY KEY (orden_id, producto_id),
    FOREIGN KEY (orden_id) REFERENCES ordenes(id) ON DELETE CASCADE
);

INSERT INTO ordenes (id, email, direccion_envio, telefono, estado, fecha_creacion) VALUES
(1001, 'cliente@email.com', 'Av. Italia 3333, Maldonado', '+59899111222', 'Created', NOW());

INSERT INTO orden_productos (orden_id, producto_id, cantidad) VALUES
(1001, 1, 2);

ALTER TABLE ordenes AUTO_INCREMENT = 1002;
