#!/usr/bin/env bash
# Checks the "ns" VM against the verifications of phases B1-B3 (docs/parte-b/PLAN.md).
# Everything runs over SSH inside ns, so it works the same on Linux and in Git Bash on
# Windows without installing dig on the host.
#
# Usage: scripts/verificar-ns.sh          (run from the repository root, ns powered on)
# Environment: NS_IP (default 192.168.10.10), LLAVE (default keys/httpaas_ed25519)
set -uo pipefail

NS_IP="${NS_IP:-192.168.10.10}"
LLAVE="${LLAVE:-keys/httpaas_ed25519}"
PRUEBA="verificacion"   # test name; it is removed at the end
IP_PRUEBA="192.168.10.250"

[[ "$NS_IP" =~ ^192\.168\.10\.[0-9]{1,3}$ ]] || { echo "NS_IP inválida: $NS_IP" >&2; exit 2; }
[ -f "$LLAVE" ] || { echo "No existe la llave $LLAVE (plan general §7, P3)" >&2; exit 2; }

OPC=(-i "$LLAVE" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new
     -o UserKnownHostsFile=keys/known_hosts -o ConnectTimeout=5)
ok=0; fallas=0
# The remote commands are fixed strings of this script: no user data goes into them.
# Only the remote command's standard output is read: an SSH error message must never
# count as an answer.
vm() { ssh "${OPC[@]}" "httpaas@$NS_IP" "$1" 2>/dev/null; }
# rechazado: the command ran in ns and ns refused it. SSH returns 255 when it cannot
# connect, so 255 is "no answer", never "refused".
rechazado() { vm "$1" >/dev/null; local rc=$?; [ "$rc" -ne 0 ] && [ "$rc" -ne 255 ]; }
verificar() { # name, condition result (0 = OK)
  if [ "$2" -eq 0 ]; then echo "  OK    $1"; ok=$((ok + 1)); else echo "  FALLA $1"; fallas=$((fallas + 1)); fi
}
es() { [ "$1" = "$2" ]; echo $?; }

echo "== B1: acceso"
verificar "SSH con la llave como httpaas (hostname = ns)" "$(es "$(vm hostname)" ns)"
sin_clave=$(ssh -o BatchMode=yes -o PubkeyAuthentication=no -o StrictHostKeyChecking=no \
  -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 "httpaas@$NS_IP" true 2>&1)
[[ "$sin_clave" == *"Permission denied"* ]]; verificar "La contraseña está desactivada" $?
reglas=$(vm "sudo -n -l" | grep -c 'NOPASSWD')
[ "$reglas" -eq 1 ] && vm "sudo -n -l" | grep -q '/usr/local/sbin/httpaas-dns'
verificar "sudo permite solo httpaas-dns" $?

echo "== B2: zona"
verificar "named está activo" "$(es "$(vm 'systemctl is-active named')" active)"
verificar "ns.cloud.local -> $NS_IP" "$(es "$(vm 'dig @127.0.0.1 +short ns.cloud.local')" "$NS_IP")"
verificar "Zona inversa: $NS_IP -> ns.cloud.local." "$(es "$(vm "dig @127.0.0.1 +short -x $NS_IP")" ns.cloud.local.)"
[ -n "$(vm 'dig @127.0.0.1 +short +time=3 debian.org')" ]; verificar "Resuelve internet (recursión)" $?

echo "== B3: actualización dinámica (httpaas-dns)"
vm "sudo /usr/local/sbin/httpaas-dns agregar $PRUEBA $IP_PRUEBA" >/dev/null
verificar "Agregar $PRUEBA -> $IP_PRUEBA" "$(es "$(vm "dig @127.0.0.1 +short $PRUEBA.cloud.local")" "$IP_PRUEBA")"
vm "sudo /usr/local/sbin/httpaas-dns listar" | grep -q "^$PRUEBA $IP_PRUEBA$"; verificar "Listar muestra el registro" $?
rechazado "sudo /usr/local/sbin/httpaas-dns agregar 'x;id' $IP_PRUEBA"; verificar "Rechaza un nombre malicioso" $?
rechazado "sudo /usr/local/sbin/httpaas-dns agregar otro 10.0.0.5"; verificar "Rechaza una IP fuera de la red" $?
vm "sudo /usr/local/sbin/httpaas-dns quitar $PRUEBA" >/dev/null
# "fin" proves ns answered: an empty answer without it would also match an unreachable VM.
verificar "Quitar $PRUEBA" "$(es "$(vm "dig @127.0.0.1 +short $PRUEBA.cloud.local; echo fin")" fin)"
vm "sudo /usr/local/sbin/httpaas-dns quitar $PRUEBA" >/dev/null; verificar "Quitar algo que no existe no es error" $?
rechazado "sudo -n cat /etc/shadow"; verificar "sudo para otra cosa es rechazado" $?
vm "ls /var/lib/bind/db.cloud.local.jnl" >/dev/null; verificar "La zona es dinámica (existe el .jnl)" $?

echo
echo "Resultado: $ok OK, $fallas FALLA"
[ "$fallas" -eq 0 ]
