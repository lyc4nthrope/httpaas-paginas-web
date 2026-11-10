#!/usr/bin/env bash
# Imports the "ns" VM (Bind9, part B) from ns.ova and attaches its first network card to
# THIS computer's host-only network. An exported .ova keeps the host-only adapter name of
# the computer it came from (on Windows "VirtualBox Host-Only Ethernet Adapter #2", on
# Linux "vboxnet0"), so without this step the VM starts without the project network.
#
# Usage:
#   scripts/importar-ns.sh ruta/a/ns.ova                 # Linux: uses vboxnet0
#   scripts/importar-ns.sh ruta/a/ns.ova "<adaptador>"   # Windows (Git Bash): see docs/IMPORTAR-NS.md
#   scripts/importar-ns.sh "" "<adaptador>"              # VM already imported: only fix the network
set -euo pipefail

OVA="${1:-}"
ADAPTADOR="${2:-vboxnet0}"
VM="ns"
IP_HOST="192.168.10.1"

fallar() { echo "ERROR: $*" >&2; exit 1; }

command -v VBoxManage >/dev/null || fallar "VBoxManage no está en el PATH."

# 1. The host-only network must exist on this computer with the host at 192.168.10.1 (P2).
ip_red=$(VBoxManage list hostonlyifs | awk -v n="$ADAPTADOR" '
  /^Name:/ { sub(/^Name:[ \t]+/, ""); nombre = $0 }
  /^IPAddress:/ && nombre == n { print $2 }')
[ -n "$ip_red" ] || fallar "no existe la red host-only '$ADAPTADOR'. Créala (plan general §7, P2) o pasa su nombre exacto como segundo argumento (VBoxManage list hostonlyifs)."
[ "$ip_red" = "$IP_HOST" ] || fallar "la red '$ADAPTADOR' tiene la IP $ip_red; debe ser $IP_HOST (plan general §7, P2)."

# 2. Import only if the VM is not registered yet.
if VBoxManage showvminfo "$VM" >/dev/null 2>&1; then
  echo "La VM '$VM' ya está importada; solo se revisa su red."
else
  [ -n "$OVA" ] && [ -f "$OVA" ] || fallar "no se encuentra el archivo .ova: '$OVA'."
  echo "Importando $OVA (puede tardar unos minutos)..."
  VBoxManage import "$OVA"
fi

# 3. The network card can only be changed while the VM is powered off.
estado=$(VBoxManage showvminfo "$VM" --machinereadable | sed -n 's/^VMState="\(.*\)"$/\1/p')
actual=$(VBoxManage showvminfo "$VM" --machinereadable | sed -n 's/^hostonlyadapter1="\(.*\)"$/\1/p')
if [ "$actual" != "$ADAPTADOR" ]; then
  case "$estado" in
    poweroff|aborted) ;;
    *) fallar "la VM está en estado '$estado'. Apágala (VBoxManage controlvm $VM acpipowerbutton) y vuelve a correr el script." ;;
  esac
  VBoxManage modifyvm "$VM" --nic1 hostonly --host-only-adapter1 "$ADAPTADOR"
  echo "Adaptador 1 cambiado: '$actual' -> '$ADAPTADOR'."
fi

echo "Listo: '$VM' usa la red host-only '$ADAPTADOR' (adaptador 1) y NAT (adaptador 2)."
echo "Siguiente paso: VBoxManage startvm $VM --type headless && scripts/verificar-ns.sh"
