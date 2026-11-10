#!/bin/bash
set -euo pipefail

# 1. Actualizar e instalar paquetes obligatorios (Punto 9)
apt-get update -y
apt-get install -y sudo curl unzip apache2

# 2. Asegurar existencia del usuario httpaas
if ! id -u httpaas >/dev/null 2>&1; then
    useradd -m -s /bin/bash httpaas
fi

# 3. Crear directorio de subidas con permisos estrictos 0700 (C1.4 - Punto 9)
mkdir -p /home/httpaas/subidas
chmod 0700 /home/httpaas/subidas
chown httpaas:httpaas /home/httpaas/subidas

# 4. Configurar SSH para el usuario httpaas (C1.4 - Punto 9)
SSH_DIR="/home/httpaas/.ssh"
AUTH_KEYS="$SSH_DIR/authorized_keys"

mkdir -p "$SSH_DIR"
chmod 0700 "$SSH_DIR"

if [ ! -f "$AUTH_KEYS" ]; then
    touch "$AUTH_KEYS"
fi
chmod 0600 "$AUTH_KEYS"
chown -R httpaas:httpaas "$SSH_DIR"

# 5. Configurar endurecimiento de SSH en sshd_config.d (C1.5 - Punto 9)
mkdir -p /etc/ssh/sshd_config.d/
cat << 'SSHEOF' > /etc/ssh/sshd_config.d/10-httpaas.conf
PubkeyAuthentication yes
PasswordAuthentication yes
SSHEOF

# Validar sintaxis de SSH antes de continuar (Punto 9)
sshd -t

# 6. Copiar scripts a sus rutas ejecutables y asignar permisos
mkdir -p /etc/httpaas

cp scripts/httpaas-firstboot /usr/local/bin/httpaas-firstboot
chmod 0755 /usr/local/bin/httpaas-firstboot

cp scripts/httpaas-publicar /usr/local/sbin/httpaas-publicar
chmod 0755 /usr/local/sbin/httpaas-publicar

cp deploy/plantilla/vhost.conf.tmpl /etc/httpaas/vhost.conf.tmpl

# 7. Configurar sudoers para httpaas-publicar sin contraseña
cat << 'SUDOEOF' > /etc/sudoers.d/httpaas-publicar
httpaas ALL=(ALL) NOPASSWD: /usr/local/sbin/httpaas-publicar
SUDOEOF
chmod 0440 /etc/sudoers.d/httpaas-publicar

# Validar sintaxis del archivo sudoers
visudo -cf /etc/sudoers.d/httpaas-publicar

# 8. Instalar y habilitar servicio systemd de primer arranque
cat << 'SERVICEEOF' > /etc/systemd/system/httpaas-firstboot.service
[Unit]
Description=HTTPaaS First Boot Service
After=network.target VirtualBoxGuestAdditions.service
Wants=network.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/httpaas-firstboot
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
SERVICEEOF

systemctl daemon-reload
systemctl enable httpaas-firstboot.service

echo "Instalacion de la plantilla HTTPaaS completada con exito."