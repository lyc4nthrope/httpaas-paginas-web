#!/bin/bash
# Runs, on the HOST, the same thing the app does with every clone, but on the template VM
# itself and inside a snapshot: it writes the guest properties, boots the VM and checks
# that httpaas-firstboot reaches "listo" with the new name and IP, and that SSH and Apache
# answer. Whatever happens, the VM is powered off and restored to the snapshot, and the
# snapshot and the properties are removed: the template is left exactly as it was.
#
# Usage (template powered off, its disk in NORMAL mode, attached to the VM):
#   deploy/plantilla/probar-clon.sh                       # VM apache-plantilla, test IP .29
#   VM=mi-plantilla IP=192.168.10.28 deploy/plantilla/probar-clon.sh
# Environment: VM (apache-plantilla), IP (192.168.10.29), ESPERA (seconds, 180)
set -uo pipefail

VM="${VM:-apache-plantilla}"
IP="${IP:-192.168.10.29}"
ESPERA="${ESPERA:-180}"
NOMBRE="prueba"
SNAP="antes-de-probar-clon"

fallar() { echo "ERROR: $*" >&2; exit 2; }
command -v VBoxManage >/dev/null || fallar "VBoxManage no está en el PATH."
[[ "$IP" =~ ^192\.168\.10\.([1-9][0-9]?|1[0-9][0-9]|2[0-4][0-9]|25[0-4])$ ]] || fallar "IP inválida: $IP"
info() { VBoxManage showvminfo "$VM" --machinereadable 2>/dev/null; }
estado_vm() { info | sed -n 's/^VMState="\(.*\)"$/\1/p'; }
prop() { VBoxManage guestproperty get "$VM" "$1" 2>/dev/null | sed -n 's/^Value: //p'; }
# VirtualBox keeps the VM locked for a moment after a state change: retry only on a lock.
vbox() {
  local i out
  for i in $(seq 1 15); do
    out=$(VBoxManage "$@" 2>&1) && return 0
    grep -q -E 'locked|VBOX_E_INVALID_OBJECT_STATE' <<<"$out" || { echo "$out" >&2; return 1; }
    sleep 2
  done
  echo "$out" >&2; return 1
}

info >/dev/null || fallar "no existe la VM '$VM'."
case "$(estado_vm)" in poweroff|aborted) ;; *) fallar "la VM '$VM' debe estar apagada (está en '$(estado_vm)')." ;; esac
VBoxManage snapshot "$VM" list --machinereadable 2>/dev/null | grep -q "=\"$SNAP\"$" && fallar "ya existe el snapshot '$SNAP': bórralo primero (VBoxManage snapshot $VM delete $SNAP)."
if ! info | grep -q -E '^"[^"]*-0-0"="/'; then fallar "la VM '$VM' no tiene disco conectado."; fi

limpiar() {
  echo "== Dejando la plantilla como estaba"
  if [ "$(estado_vm)" != poweroff ] && [ "$(estado_vm)" != aborted ]; then
    vbox controlvm "$VM" poweroff || true
    for _ in $(seq 1 60); do case "$(estado_vm)" in poweroff|aborted) break ;; esac; sleep 1; done
  fi
  vbox snapshot "$VM" restore "$SNAP" && echo "   restaurado el snapshot"
  vbox snapshot "$VM" delete "$SNAP" && echo "   borrado el snapshot"
  for p in hostname ip dominio dns estado; do VBoxManage guestproperty unset "$VM" "/httpaas/$p" 2>/dev/null; done
  echo "   propiedades /httpaas/* quitadas"
}

vbox snapshot "$VM" take "$SNAP" >/dev/null || fallar "no se pudo tomar el snapshot."
trap limpiar EXIT
for par in "hostname $NOMBRE" "ip $IP/24" "dominio cloud.local" "dns 192.168.10.10"; do
  set -- $par
  VBoxManage guestproperty set "$VM" "/httpaas/$1" "$2" || fallar "no se pudo escribir /httpaas/$1"
done
VBoxManage guestproperty unset "$VM" /httpaas/estado 2>/dev/null
echo "== Arrancando '$VM' como si fuera el clon '$NOMBRE' ($IP)"
vbox startvm "$VM" --type headless || fallar "no arrancó la VM."

ok=0; fallas=0
verificar() { if [ "$2" -eq 0 ]; then echo "  OK    $1"; ok=$((ok + 1)); else echo "  FALLA $1"; fallas=$((fallas + 1)); fi; }

echo "== Esperando /httpaas/estado (hasta $ESPERA s)"
estado=""; fin=$((SECONDS + ESPERA))
while [ $SECONDS -lt $fin ]; do
  estado=$(prop /httpaas/estado)
  case "$estado" in listo|error:*) break ;; esac
  sleep 3
done
echo "   último estado: '${estado:-ninguno}'"
[ "$estado" = listo ]; verificar "httpaas-firstboot llegó a 'listo'" $?
if [ -z "$estado" ]; then
  echo "   Pista: el primer arranque no escribió nada. ¿Está vacío, sin permisos o 'masked'? Dentro de la VM:"
  echo "   systemctl is-enabled httpaas-firstboot; md5sum /usr/local/bin/httpaas-firstboot"
fi

if [ "$estado" = listo ]; then
  sleep 5 # Guest Additions refresh the reported IP every few seconds
  [ "$(prop /VirtualBox/GuestInfo/Net/0/V4/IP)" = "$IP" ]; verificar "La VM tomó la IP $IP" $?
  timeout 5 bash -c "echo > /dev/tcp/$IP/22" 2>/dev/null; verificar "SSH responde en $IP" $?
  pagina=$(curl -s --max-time 5 -H "Host: $NOMBRE.cloud.local" "http://$IP/")
  grep -q "$NOMBRE" <<<"$pagina"; verificar "Apache sirve la página de '$NOMBRE.cloud.local'" $?
fi

echo
echo "Resultado: $ok OK, $fallas FALLA"
if [ "$fallas" -eq 0 ]; then echo "El clon funciona: ya se puede subir el disco."; else echo "NO subas el disco: el clon no quedó bien."; fi
[ "$fallas" -eq 0 ]
