#!/bin/bash
# Prepares the apache-plantilla VM (Debian 13) as the HTTPaaS template (C1.4-C4.2).
# Run it inside the VM, as root, from any folder:
#   sudo bash /ruta/al/repo/deploy/plantilla/instalar.sh
# Idempotent: running it twice leaves the same result. Check it afterwards with
#   sudo bash /ruta/al/repo/deploy/plantilla/verificar-plantilla.sh
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
    echo "Error: corre este script como root (sudo)." >&2
    exit 1
fi
# Files are taken from the script's own folder, so it works from any working directory.
DIR="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"
LLAVES="$DIR/../llaves"

# 1. Actualizar e instalar paquetes obligatorios (Punto 9)
apt-get update -y
apt-get install -y sudo curl unzip apache2 openssh-server

# 2. Asegurar existencia del usuario httpaas
if ! id -u httpaas >/dev/null 2>&1; then
    useradd -m -s /bin/bash httpaas
fi

# 3. Crear directorio de subidas con permisos estrictos 0700 (C1.4 - Punto 9)
install -d -o httpaas -g httpaas -m 0700 /home/httpaas/subidas

# 4. Llaves SSH del equipo (C1.4): every deploy/llaves/*.pub, added once each.
# They go in BEFORE password login is turned off in step 5, so nobody is locked out.
SSH_DIR="/home/httpaas/.ssh"
AUTH_KEYS="$SSH_DIR/authorized_keys"
install -d -o httpaas -g httpaas -m 0700 "$SSH_DIR"
touch "$AUTH_KEYS"
for pub in "$LLAVES"/*.pub; do
    [ -f "$pub" ] || continue
    grep -qxF "$(cat "$pub")" "$AUTH_KEYS" || cat "$pub" >> "$AUTH_KEYS"
done
chown httpaas:httpaas "$AUTH_KEYS"
chmod 0600 "$AUTH_KEYS"
if [ ! -s "$AUTH_KEYS" ]; then
    echo "Error: no hay llaves en $LLAVES; sin ellas nadie podría entrar por SSH." >&2
    exit 1
fi

# 5. Endurecimiento de SSH (C1.5): keys only, no root login.
# Tested with sshd -t before it is applied; a broken file is removed, never left in place.
mkdir -p /etc/ssh/sshd_config.d/
cat << 'SSHEOF' > /etc/ssh/sshd_config.d/10-httpaas.conf
PubkeyAuthentication yes
PasswordAuthentication no
PermitRootLogin no
SSHEOF
if ! sshd -t; then
    rm -f /etc/ssh/sshd_config.d/10-httpaas.conf
    echo "Error: la configuración de sshd no es válida; no se aplicó." >&2
    exit 1
fi
systemctl reload ssh 2>/dev/null || true

# 6. Scripts and template, owned by root.
# `install` replaces the file: a plain `cp` over an existing file keeps its old owner, so
# a script owned by httpaas would stay writable by httpaas while root runs it at boot.
mkdir -p /etc/httpaas
install -o root -g root -m 0755 "$DIR/httpaas-firstboot" /usr/local/bin/httpaas-firstboot
install -o root -g root -m 0755 "$DIR/httpaas-publicar" /usr/local/sbin/httpaas-publicar
install -o root -g root -m 0644 "$DIR/vhost.conf.tmpl" /etc/httpaas/vhost.conf.tmpl

# 7. Configurar sudoers para httpaas-publicar sin contraseña (validado antes de dejarlo)
cat << 'SUDOEOF' > /etc/sudoers.d/httpaas-publicar.tmp
httpaas ALL=(root) NOPASSWD: /usr/local/sbin/httpaas-publicar
SUDOEOF
chmod 0440 /etc/sudoers.d/httpaas-publicar.tmp
if visudo -cf /etc/sudoers.d/httpaas-publicar.tmp; then
    mv /etc/sudoers.d/httpaas-publicar.tmp /etc/sudoers.d/httpaas-publicar
else
    rm -f /etc/sudoers.d/httpaas-publicar.tmp
    echo "Error: la regla de sudoers no es válida; no se aplicó." >&2
    exit 1
fi

# 8. Servicio systemd de primer arranque: the unit from the repository, the single source
# (After=vboxadd-service.service, so Guest Additions are up before it reads properties).
install -o root -g root -m 0644 "$DIR/httpaas-firstboot.service" /etc/systemd/system/httpaas-firstboot.service
systemctl daemon-reload
# A masked unit cannot be enabled (and never runs): undo a `systemctl mask` left from testing.
systemctl unmask httpaas-firstboot.service
systemctl enable httpaas-firstboot.service

echo "Instalacion de la plantilla HTTPaaS completada con exito."
echo "Siguiente paso: sudo bash $DIR/verificar-plantilla.sh"
