#!/bin/bash
set -e

# 1. Actualizar repositorios e instalar paquetes requeridos (C1.4)
apt-get update
apt-get install -y apache2 unzip python3

# 2. Crear usuario httpaas si no existe
if ! id -u httpaas >/dev/null 2>&1; then
    useradd -m -s /bin/bash httpaas
fi

# 3. Crear directorio de configuración de httpaas
mkdir -p /etc/httpaas

# 4. Obtener ruta del script e instalar componentes
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Copiar plantilla vhost si existe
if [ -f "$SCRIPT_DIR/vhost.conf.tmpl" ]; then
    cp "$SCRIPT_DIR/vhost.conf.tmpl" /etc/httpaas/vhost.conf.tmpl
fi

# Instalar script firstboot
if [ -f "$SCRIPT_DIR/httpaas-firstboot" ]; then
    cp "$SCRIPT_DIR/httpaas-firstboot" /usr/local/bin/httpaas-firstboot
    chmod +x /usr/local/bin/httpaas-firstboot
fi

# Instalar y habilitar servicio systemd
if [ -f "$SCRIPT_DIR/httpaas-firstboot.service" ]; then
    cp "$SCRIPT_DIR/httpaas-firstboot.service" /etc/systemd/system/httpaas-firstboot.service
    systemctl daemon-reload
    systemctl enable httpaas-firstboot
fi

# Instalar script de publicación segura
if [ -f "$SCRIPT_DIR/httpaas-publicar" ]; then
    cp "$SCRIPT_DIR/httpaas-publicar" /usr/local/sbin/httpaas-publicar
    chmod 0755 /usr/local/sbin/httpaas-publicar
    chown root:root /usr/local/sbin/httpaas-publicar
fi

# 5. Configurar sudoers exclusivo
echo "httpaas ALL=(root) NOPASSWD: /usr/local/sbin/httpaas-publicar" > /etc/sudoers.d/httpaas
chmod 0440 /etc/sudoers.d/httpaas
visudo -cf /etc/sudoers.d/httpaas

echo "Instalación de la plantilla completada con éxito."
