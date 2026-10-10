// Interfaz web - Requisito 2.a (tabla de ordenes) y 2.b (detalle de productos)
// La fila se expande en el lugar al hacer click.

const ENDPOINT = '/api/ordenes';
const COLUMNAS = 5;

const cuerpoTabla = document.getElementById('tabla-ordenes');
const sinDatos = document.getElementById('sin-datos');
const mensajeError = document.getElementById('mensaje-error');
const btnRecargar = document.getElementById('btn-recargar');

let ordenActual = 'desc';

async function cargarOrdenes() {
    mostrarError(null);
    try {
        const respuesta = await fetch(ENDPOINT);
        if (!respuesta.ok) {
            throw new Error('La API respondio con HTTP ' + respuesta.status);
        }

        // OJO: /api/ordenes devuelve las claves en PascalCase (ID, Email, Estado...).
        const ordenes = await respuesta.json();
        renderizar(ordenar(ordenes, ordenActual));
    } catch (error) {
        mostrarError('No se pudieron cargar las ordenes: ' + error.message);
        renderizar([]);
    }
}

function ordenar(ordenes, sentido) {
    const copia = [...ordenes];
    copia.sort((a, b) => {
        const fa = new Date(a.FechaCreacion).getTime();
        const fb = new Date(b.FechaCreacion).getTime();
        return sentido === 'desc' ? fb - fa : fa - fb;
    });
    return copia;
}

function renderizar(ordenes) {
    cuerpoTabla.innerHTML = '';
    sinDatos.hidden = ordenes.length > 0;

    for (const orden of ordenes) {
        const fila = document.createElement('tr');
        fila.className = 'fila-clic';
        fila.innerHTML =
            '<td>' + escapar(orden.ID) + '</td>' +
            '<td>' +escapar(orden.Email) + '</td>' +
            '<td>' + escapar(orden.Telefono) + '</td>' +
            '<td><span class="estado ' + claseEstado(orden.Estado) + '">' +
                escapar(orden.Estado) + '</span></td>' +
            '<td>' + escapar(formatearFecha(orden.FechaCreacion)) + '</td>';

        fila.addEventListener('click', () => alternarDetalle(fila, orden.ID));
        cuerpoTabla.appendChild(fila);
    }
}



function alternarDetalle(fila, id) {
    // Si ya estaba abierta, se cierra.
    const abierta = fila.nextElementSibling;
    if (abierta && abierta.classList.contains('fila-detalle')) {
        abierta.remove();
        fila.classList.remove('fila-abierta');
        return;
    }

    // Crea la fila de detalle de la orden.
    const detalle = document.createElement('tr');
    detalle.className = 'fila-detalle';

    const celda = document.createElement('td');
    celda.colSpan = COLUMNAS;
    celda.innerHTML = '<p class="cargando">Cargando detalle...</p>';

    detalle.appendChild(celda);
    fila.after(detalle);
    fila.classList.add('fila-abierta');

    cargarDetalle(celda, id);
}

async function cargarDetalle(celda, id) {
    try {
        const respuesta = await fetch('/api/ordenes/' + id + '/detalle');
        if (!respuesta.ok) {
            throw new Error('La API respondio con HTTP ' + respuesta.status);
        }

        //devuelve las claves en camelCase (id, email, productos, productoId...). Es al reves que /api/ordenes
        const detalle = await respuesta.json();
        celda.innerHTML = construirDetalle(detalle);

        // solo las ordenes "Ready to Delivery" tienen factura.
        if (detalle.estado === 'Ready to Delivery') {
            const bloque = document.createElement('div');
            bloque.className = 'detalle-factura';
            bloque.innerHTML = '<p class="cargando">Cargando facturacion...</p>';
            celda.appendChild(bloque);
            cargarFactura(bloque, id);
        }
    } catch (error) {
        celda.innerHTML = '<p class="detalle-error">No se pudo cargar el detalle de la orden ' +
            escapar(id) + ': ' + escapar(error.message) + '</p>';
    }
}

async function cargarFactura(bloque, id) {
    try {
        const respuesta = await fetch('/api/ordenes/' + id + '/factura');

        // La factura la genera procesador-api de forma asincrona (via MQTT).
        // Si todavia no esta lista, la API responde 404.
        if (respuesta.status === 404) {
            bloque.innerHTML = '<p class="detalle-error">La orden todavia no tiene factura generada.</p>';
            return;
        }
        if (!respuesta.ok) {
            throw new Error('La API respondio con HTTP ' + respuesta.status);
        }

        bloque.innerHTML = construirFactura(await respuesta.json());
    } catch (error) {
        bloque.innerHTML = '<p class="detalle-error">No se pudo cargar la facturacion: ' +
            escapar(error.message) + '</p>';
    }
}

function construirDetalle(detalle) {
    let html = '';

    html += '<div class="detalle-cabecera">';
    html += '<div><span>Email</span><strong>' + escapar(detalle.email) + '</strong></div>';
    html += '<div><span>Telefono</span><strong>' + escapar(detalle.telefono) + '</strong></div>';
    html += '<div><span>Direccion</span><strong>' + escapar(detalle.direccionEnvio) + '</strong></div>';
    html += '<div><span>Fecha</span><strong>' + escapar(formatearFecha(detalle.fechaCreacion)) + '</strong></div>';
    html += '<div><span>Status</span><span class="estado ' + claseEstado(detalle.estado) + '">' +
            escapar(detalle.estado) + '</span></div>';
    html += '</div>';

    if (!detalle.productos || detalle.productos.length === 0) {
        return html + '<p class="sin-datos">La orden no tiene productos.</p>';
    }

    let total = 0;
    html += '<table class="tabla tabla-detalle"><thead><tr>' +
            '<th>Id</th><th>Producto</th><th>Descripcion</th>' +
            '<th>Cantidad</th><th>Precio unitario</th><th>Subtotal</th>' +
            '</tr></thead><tbody>';

    for (const p of detalle.productos) {
        const subtotal = p.cantidad * p.precioUnitario;
        total += subtotal;
        html += '<tr>' +
            '<td>' + escapar(p.productoId) + '</td>' +
            '<td>' + escapar(p.nombre) + '</td>' +
            '<td>' + escapar(p.descripcion) + '</td>' +
            '<td>' + escapar(p.cantidad) + '</td>' +
            '<td>$' + formatearPrecio(p.precioUnitario) + '</td>' +
            '<td><strong>$' + formatearPrecio(subtotal) + '</strong></td>' +
            '</tr>';
    }

    html += '</tbody><tfoot><tr>' +
            '<td colspan="5" class="total-etiqueta">Total</td>' +
            '<td class="total-valor"><strong>$' + formatearPrecio(total) + '</strong></td>' +
            '</tr></tfoot></table>';

    return html;
}

function construirFactura(factura) {
    let html = '';

    html += '<h3 class="factura-titulo">Facturacion</h3>';
    html += '<div class="detalle-cabecera">';
    html += '<div><span>Factura</span><strong>#' + escapar(factura.id) + '</strong></div>';
    html += '<div><span>Emitida</span><strong>' + escapar(formatearFecha(factura.fechaEmision)) + '</strong></div>';
    html += '<div><span>Monto total</span><strong>$' + formatearPrecio(factura.montoTotal) + '</strong></div>';
    html += '</div>';

    if (!factura.items || factura.items.length === 0) {
        return html + '<p class="sin-datos">La factura no tiene items.</p>';
    }

    html += '<table class="tabla tabla-detalle"><thead><tr>' +
            '<th>Producto</th><th>Cantidad</th><th>Precio unitario</th><th>Subtotal</th>' +
            '</tr></thead><tbody>';

    for (const item of factura.items) {
        const subtotal = item.cantidad * item.precioUnitario;
        html += '<tr>' +
            '<td>' + escapar(item.productoId) + '</td>' +
            '<td>' + escapar(item.cantidad) + '</td>' +
            '<td>$' + formatearPrecio(item.precioUnitario) + '</td>' +
            '<td><strong>$' + formatearPrecio(subtotal) + '</strong></td>' +
            '</tr>';
    }

    html += '</tbody><tfoot><tr>' +
            '<td colspan="3" class="total-etiqueta">Total</td>' +
            '<td class="total-valor"><strong>$' + formatearPrecio(factura.montoTotal) + '</strong></td>' +
            '</tr></tfoot></table>';

    return html;
}

function formatearPrecio(numero) {
    return numero.toLocaleString('es-UY', {
        minimumFractionDigits: 2,
        maximumFractionDigits: 2
    });
}

function formatearFecha(iso) {
    const fecha = new Date(iso);
    if (isNaN(fecha.getTime())) {
        return iso;
    }
    return fecha.toLocaleString('es-UY', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit'
    });
}

function claseEstado(estado) {
    const mapa = {
        'Created': 'gris',
        'Confirmed': 'azul',
        'Processing': 'ambar',
        'Ready to Delivery': 'verde',
        'No Stock': 'rojo',
        'Shipped': 'violeta',
        'Delivered': 'verde-oscuro',
        'Cancelled': 'rojo'
    };
    return mapa[estado] || 'gris';
}

function escapar(valor) {
    if (valor === null || valor === undefined) {
        return '';
    }
    return String(valor)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

function mostrarError(texto) {
    if (texto) {
        mensajeError.textContent = texto;
        mensajeError.hidden = false;
    } else {
        mensajeError.hidden = true;
    }
}

btnRecargar.addEventListener('click', () => {
    ordenActual = 'desc';
    cargarOrdenes();
});

cargarOrdenes();