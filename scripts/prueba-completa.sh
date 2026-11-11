#!/usr/bin/env bash
# End-to-end check of the whole assignment (docs/PLAN.md §10, phase A8) against a running
# app. Every step uses the app the way the browser does (same routes, same Origin), and
# every effect is checked where it happens: the DNS inside ns, the site in Apache, the
# VM and the base disk in VirtualBox.
#
# Usage (from the repository root, app running):
#   scripts/prueba-completa.sh                  # real mode: VMs, ns and the template disk
#   SIMULADO=1 scripts/prueba-completa.sh       # against `make simular`: only the app's side
# Environment: BASE (default http://127.0.0.1:8080), CONFIG (default config.json),
#              NS_IP (default 192.168.10.10), LLAVE (default keys/httpaas_ed25519)
set -uo pipefail

BASE="${BASE:-http://127.0.0.1:8080}"
CONFIG="${CONFIG:-config.json}"
NS_IP="${NS_IP:-192.168.10.10}"
LLAVE="${LLAVE:-keys/httpaas_ed25519}"
SIMULADO="${SIMULADO:-0}"
DOMINIO="cloud.local"
ESPERA=300 # seconds a VM may take to be created or deleted

ok=0; fallas=0; omitidas=0
verificar() { # name, result (0 = OK)
  if [ "$2" -eq 0 ]; then echo "  OK      $1"; ok=$((ok + 1)); else echo "  FALLA   $1"; fallas=$((fallas + 1)); fi
}
omitir() { echo "  omitido $1 (modo simulado)"; omitidas=$((omitidas + 1)); }
real() { [ "$SIMULADO" != "1" ]; }

# --- the app's API (no jq needed: the JSON comes from encoding/json, one object per instance)
api() { curl -s "$BASE/api/instancias"; }
campo() { # instance, field: prints the field of that instance, or nothing
  api | grep -o "{[^{}]*\"Nombre\":\"$1\"[^{}]*}" | sed -n "s/.*\"$2\":\"\{0,1\}\([^\",}]*\).*/\1/p"
}
existe() { api | grep -q "\"Nombre\":\"$1\""; }
esperar() { # instance, state: waits until it reaches the state (or "ausente" = deleted)
  local fin=$((SECONDS + ESPERA))
  while [ $SECONDS -lt $fin ]; do
    if [ "$2" = ausente ]; then existe "$1" || return 0
    else
      local e; e=$(campo "$1" Estado)
      [ "$e" = "$2" ] && [ -z "$(campo "$1" Error)" ] && return 0
      case "$e" in error|perdida) return 1 ;; esac
    fi
    sleep 2
  done
  return 1
}
post() { # path, curl form args...: prints the HTTP status
  local ruta=$1; shift
  curl -s -o /dev/null -w '%{http_code}' -H "Origin: $BASE" "$@" "$BASE$ruta"
}

# --- the real side
OPC=(-i "$LLAVE" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new
     -o UserKnownHostsFile=keys/known_hosts -o ConnectTimeout=5)
dns() { ssh "${OPC[@]}" "httpaas@$NS_IP" "dig @127.0.0.1 +short $1.$DOMINIO" 2>/dev/null; }
sitio() { curl -s --max-time 10 -H "Host: $1.$DOMINIO" "http://$2/"; }
disco_base() { sed -n 's/^ *"disco_base": *"\(.*\)",\{0,1\}$/\1/p' "$CONFIG"; }

curl -s -o /dev/null "$BASE/salud" || { echo "La app no responde en $BASE" >&2; exit 2; }
if real; then
  for f in "$CONFIG" "$LLAVE"; do [ -f "$f" ] || { echo "Falta $f" >&2; exit 2; }; done
fi
echo "Prueba completa contra $BASE (simulado=$SIMULADO)"

echo "== 1. Crear paisajes y negocios"
for h in paisajes negocios; do
  verificar "Crear $h (303)" "$([ "$(post /instancias -F nombre=$h)" = 303 ]; echo $?)"
done
for h in paisajes negocios; do
  esperar "$h" lista; verificar "$h queda lista" $?
done
IP_P=$(campo paisajes IP); IP_N=$(campo negocios IP)

echo "== 2. DNS"
if real; then
  verificar "paisajes.$DOMINIO -> $IP_P" "$([ -n "$IP_P" ] && [ "$(dns paisajes)" = "$IP_P" ]; echo $?)"
  verificar "negocios.$DOMINIO -> $IP_N" "$([ -n "$IP_N" ] && [ "$(dns negocios)" = "$IP_N" ]; echo $?)"
else omitir "resolución en ns"; fi

echo "== 3. Publicar"
for h in paisajes negocios; do
  verificar "Publicar testdata/zips/$h.zip (303)" "$([ "$(post /contenido -F nombre=$h -F archivo=@testdata/zips/$h.zip)" = 303 ]; echo $?)"
done
for h in paisajes negocios; do
  esperar "$h" lista; verificar "$h vuelve a lista sin error" $?
done

echo "== 4. Apache sirve exactamente el index.html de cada zip"
if real; then
  for par in "paisajes $IP_P" "negocios $IP_N"; do
    set -- $par
    esperado=$(unzip -p "testdata/zips/$1.zip" '*index.html' | md5sum)
    servido=$(sitio "$1" "$2" | md5sum)
    verificar "http://$1.$DOMINIO/ (Host en $2) = index.html del zip" "$([ "$esperado" = "$servido" ]; echo $?)"
  done
else omitir "contenido servido por Apache"; fi

echo "== 5. Rechazos"
verificar "Nombre repetido (422)" "$([ "$(post /instancias -F nombre=paisajes)" = 422 ]; echo $?)"
verificar "Zip con zip slip (422)" "$([ "$(post /contenido -F nombre=paisajes -F archivo=@testdata/zips/zipslip.zip)" = 422 ]; echo $?)"
verificar "Zip con enlace simbólico (422)" "$([ "$(post /contenido -F nombre=paisajes -F archivo=@testdata/zips/symlink.zip)" = 422 ]; echo $?)"
verificar "Nombre malicioso (422)" "$([ "$(post /instancias -F 'nombre=a;rm')" = 422 ]; echo $?)"
verificar "Otro origen (403)" "$([ "$(curl -s -o /dev/null -w '%{http_code}' -H 'Origin: http://evil.example' -F nombre=otro "$BASE/instancias")" = 403 ]; echo $?)"

echo "== 6. Eliminar paisajes"
verificar "Eliminar paisajes (303)" "$([ "$(post /instancias/paisajes/eliminar -X POST)" = 303 ]; echo $?)"
esperar paisajes ausente; verificar "paisajes ya no está en la app" $?
if real; then
  verificar "La VM paisajes ya no existe" "$(! VBoxManage list vms | grep -q '"paisajes"'; echo $?)"
  verificar "paisajes.$DOMINIO ya no resuelve" "$([ -z "$(dns paisajes)" ]; echo $?)"
  verificar "El disco base sigue existiendo" "$([ -f "$(disco_base)" ]; echo $?)"
else omitir "VM, DNS y disco base"; fi

echo "== 7. Eliminar negocios"
verificar "Eliminar negocios (303)" "$([ "$(post /instancias/negocios/eliminar -X POST)" = 303 ]; echo $?)"
esperar negocios ausente; verificar "negocios ya no está en la app" $?
if real; then
  verificar "La VM negocios ya no existe" "$(! VBoxManage list vms | grep -q '"negocios"'; echo $?)"
  verificar "El disco base sigue existiendo" "$([ -f "$(disco_base)" ]; echo $?)"
fi

echo
echo "Resultado: $ok OK, $fallas FALLA, $omitidas omitidos"
[ "$fallas" -eq 0 ]
