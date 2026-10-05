FROM nginx:alpine

# Reemplaza la configuracion de nginx que trae la imagen oficial por la nuestra, que es la que hace el proxy hacia las APIs.
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Copia los archivos de la interfaz al directorio donde nginx
# busca las paginas web por defecto.
COPY index.html /usr/share/nginx/html/index.html
COPY styles.css /usr/share/nginx/html/styles.css
COPY app.js /usr/share/nginx/html/app.js

EXPOSE 80