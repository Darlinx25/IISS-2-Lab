// Interfaz web - Requisito 2.a
// Tabla de ordenes de compra: Id, Email, Telefono, Status, Fecha creacion.
// Ordenada por defecto de la mas reciente a la mas antigua.

const ENDPOINT = '/api/ordenes';

const cuerpoTabla = document.getElementById('tabla-ordenes');
const sinDatos = document.getElementById('sin-datos');
const mensajeError = document.getElementById('mensaje-error');
const btnRecargar = document.getElementById('btn-recargar');

// 'desc' = mas reciente primero. Es el orden por defecto que pide el enunciado.
let ordenActual = 'desc';

async function cargarOrdenes() {
    mostrarError(null);
    try {
        const respuesta = await fetch(ENDPOINT);

        if (!respuesta.ok) {
            throw new Error('La API respondio con HTTP ' + respuesta.status);
        }

        // OJO: las claves vienen en PascalCase (ID, Email, Estado...),
        // NO en camelCase como el resto de la API. Esto es lo que devuelve
        // de verdad GET /api/ordenes.
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
        fila.innerHTML =
            '<td>' + escapar(orden.ID) + '</td>' +
            '<td>' + escapar(orden.Email) + '</td>' +
            '<td>' + escapar(orden.Telefono) + '</td>' +
            '<td><span class="estado ' + claseEstado(orden.Estado) + '">' +
                escapar(orden.Estado) + '</span></td>' +
            '<td>' + escapar(formatearFecha(orden.FechaCreacion)) + '</td>';
        cuerpoTabla.appendChild(fila);
    }
}

// MySQL guarda el DATETIME sin zona horaria y corre en UTC, asi que la fecha
// llega como ISO-8601 terminado en Z. La pasamos a hora local para mostrarla.
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

// Evita que un dato que traiga HTML se ejecute como codigo.
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