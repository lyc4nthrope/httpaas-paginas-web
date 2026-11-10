#!/bin/bash
# Checks, inside apache-plantilla and as root, that the template is ready to be turned into
# the multiattach disk: the right script versions, owned by root, SSH with keys only, and
# no leftovers of a test clone. Run it right before powering the VM off to upload the disk.
#   sudo bash /ruta/al/repo/deploy/plantilla/verificar-plantilla.sh
set -uo pipefail

if [ "$(id -u)" -ne 0 ]; then
    echo "Error: corre este script como root (sudo)." >&2
    exit 2
fi
DIR="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"
ok=0; fallas=0
verificar() { # name, result (0 = OK)
    if [ "$2" -eq 0 ]; then echo "  OK    $1"; ok=$((ok + 1)); else echo "  FALLA $1"; fallas=$((fallas + 1)); fi
}
# Command output is captured first and searched afterwards: with pipefail, `cmd | grep -q`
# fails when grep stops early and cmd gets SIGPIPE, even though the line was there.
tiene() { grep -qx -- "$2" <<<"$1"; } # text, exact line

de_root_sin_escritura_ajena() { # file: owned by root and writable by nobody else
    [ -f "$1" ] && [ "$(stat -c %U:%G "$1")" = root:root ] && [ $(( 0$(stat -c %a "$1") & 022 )) -eq 0 ]
}

echo "== Scripts"
F=/usr/local/bin/httpaas-firstboot
de_root_sin_escritura_ajena "$F"; verificar "$F es de root y nadie más puede modificarlo" $?
grep -q '/httpaas/dominio' "$F" 2>/dev/null && ! grep -q '/httpaas/domain"' "$F"; verificar "$F lee /httpaas/dominio (contrato §4.3)" $?
cmp -s "$F" "$DIR/httpaas-firstboot"; verificar "$F es igual al del repositorio" $?
P=/usr/local/sbin/httpaas-publicar
de_root_sin_escritura_ajena "$P"; verificar "$P es de root y nadie más puede modificarlo" $?
cmp -s "$P" "$DIR/httpaas-publicar"; verificar "$P es igual al del repositorio" $?
[ ! -e /usr/local/sbin/httpaas-firstboot ]; verificar "No hay un httpaas-firstboot viejo en /usr/local/sbin" $?
bash -n "$F" 2>/dev/null && bash -n "$P" 2>/dev/null; verificar "Los dos scripts no tienen errores de sintaxis (bash -n)" $?

echo "== Arranque"
systemctl is-enabled --quiet httpaas-firstboot.service; verificar "httpaas-firstboot.service está habilitado" $?
[ "$(systemctl show -p ExecStart --value httpaas-firstboot.service | sed -n 's/.*path=\([^ ;]*\).*/\1/p')" = "$F" ]
verificar "La unidad ejecuta $F" $?
unidad=$(systemctl cat httpaas-firstboot.service 2>/dev/null)
grep -q '^After=.*vboxadd-service.service' <<<"$unidad"; verificar "La unidad espera a vboxadd-service" $?
command -v VBoxControl >/dev/null; verificar "Guest Additions instaladas (VBoxControl)" $?

echo "== Acceso"
sshd_conf=$(sshd -T 2>/dev/null)
tiene "$sshd_conf" 'passwordauthentication no'; verificar "SSH sin contraseña" $?
tiene "$sshd_conf" 'permitrootlogin no'; verificar "SSH sin login de root" $?
[ -s /home/httpaas/.ssh/authorized_keys ] && [ "$(stat -c %a /home/httpaas/.ssh/authorized_keys)" = 600 ]
verificar "authorized_keys de httpaas tiene llaves (0600)" $?
faltan=0
for pub in "$DIR"/../llaves/*.pub; do
    [ -f "$pub" ] && ! grep -qxF "$(cat "$pub")" /home/httpaas/.ssh/authorized_keys 2>/dev/null && faltan=1
done
verificar "Están todas las llaves de deploy/llaves/" "$faltan"
permisos=$(sudo -l -U httpaas 2>/dev/null)
[ "$(grep -c 'NOPASSWD' <<<"$permisos")" -eq 1 ] && grep -q 'NOPASSWD: /usr/local/sbin/httpaas-publicar$' <<<"$permisos"
verificar "sudo de httpaas permite solo httpaas-publicar" $?
[ "$(stat -c %U:%a /home/httpaas/subidas 2>/dev/null)" = httpaas:700 ]; verificar "/home/httpaas/subidas es de httpaas (0700)" $?

echo "== Apache y limpieza (la plantilla no debe parecer un clon ya configurado)"
systemctl is-enabled --quiet apache2; verificar "apache2 está habilitado" $?
command -v unzip >/dev/null; verificar "unzip instalado" $?
[ -f /etc/httpaas/vhost.conf.tmpl ]; verificar "Existe /etc/httpaas/vhost.conf.tmpl" $?
[ ! -e /etc/httpaas/aplicado ]; verificar "No existe /etc/httpaas/aplicado (C5.2)" $?
sitios=$(ls /etc/apache2/sites-enabled 2>/dev/null | grep -v -x '000-default.conf')
[ -z "$sitios" ]; verificar "Apache sin sitios de instancias habilitados (quedaron: ${sitios//$'\n'/ })" $?
[ "$(hostname)" = apache-plantilla ]; verificar "El hostname es apache-plantilla" $?

echo
echo "Resultado: $ok OK, $fallas FALLA"
[ "$fallas" -eq 0 ] && echo "Listo: apaga la VM (poweroff) y sube ApacheServer.vdi."
[ "$fallas" -eq 0 ]
