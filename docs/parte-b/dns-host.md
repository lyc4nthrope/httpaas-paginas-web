# Resolver `*.cloud.local` desde el host (B5)

`ns` ya responde `cloud.local`, pero el sistema operativo del host no le pregunta:
su resolver usa el DNS del ISP. Esta fase conecta los dos **sin romper internet**.

El objetivo es que el navegador abra `http://paisajes.cloud.local`, no que todo el
tráfico del host pase por `ns`.

---

## Windows — regla NRPT

PowerShell **como administrador**:

```powershell
Add-DnsClientNrptRule -Namespace ".cloud.local" -NameServers "192.168.10.10" -Comment "httpaas-b5"
```

### Por qué NRPT y no el archivo `hosts`

| | `hosts` | NRPT |
|---|---|---|
| Host nuevo | Hay que agregarlo a mano | Se resuelve solo |
| Alcance | Solo los nombres escritos | Todo el sufijo `.cloud.local` |
| Deshacer | Editar el archivo | Un comando |

NRPT (*Name Resolution Policy Table*) es la tabla de políticas de resolución de
nombres de Windows: "para este sufijo, pregunta a este servidor". Es DNS dividido,
igual que hace `resolvectl` en Linux.

### Verificación

```powershell
Resolve-DnsName ns.cloud.local
ping ns.cloud.local
```

Esperado: `192.168.10.10` en ambos.

### Deshacer

```powershell
Remove-DnsClientNrptRule -Namespace ".cloud.local"
```

Para ver qué reglas hay: `Get-DnsClientNrptRule`.

### Gotchas

- **`nslookup` no usa NRPT.** Hace sus propias consultas al servidor que le digas.
  Para probar usa `Resolve-DnsName` o `ping`.
- **Firefox con "DNS sobre HTTPS" activo se salta el resolver del sistema.** Para
  la prueba, desactívalo en *Ajustes → Privacidad y seguridad*.
- **La regla cubre `.cloud.local`, no la zona inversa.** `Resolve-DnsName 192.168.10.10`
  consulta `10.168.192.in-addr.arpa`, que no coincide con el sufijo y sigue yendo al
  DNS del ISP. No afecta al navegador; el `PTR` existe y responde si se pregunta a
  `ns` directamente: `Resolve-DnsName 192.168.10.10 -Server 192.168.10.10 -Type PTR`.
- Requiere que `ns` esté encendida y `192.168.10.10` sea alcanzable.

---

## Linux — `scripts/dns-host-linux.sh`

```bash
sudo ./scripts/dns-host-linux.sh
```

El script pide `sudo` solo, es idempotente y hace dos cosas independientes.

### 1. `nsswitch.conf`: `dns` antes de mDNS

Debian trae:

```text
hosts:          files mdns4_minimal [NOTFOUND=return] dns
```

`mdns4_minimal` atiende los nombres `.local`, y `[NOTFOUND=return]` corta la
búsqueda ahí. Como `cloud.local` termina en `.local`, mDNS se queda con la consulta
y **nunca llega a DNS**. El script mueve `dns` delante:

```text
hosts:          files dns mdns4_minimal [NOTFOUND=return]
```

No agrega ni quita módulos. Si tu distro lista `mymachines`, `resolve` o
`myhostname`, los conserva en su posición original:

```text
hosts: files myhostname mdns4_minimal [NOTFOUND=return] dns
  ↓
hosts: files myhostname dns mdns4_minimal [NOTFOUND=return]
```

La única regla es: **`dns` tiene que existir y estar antes de `mdns[46]_minimal`**.
Todo lo demás se respeta tal cual.

### 2. El resolver

Dos caminos según la máquina:

| Caso | Qué hace | Alcance |
|---|---|---|
| `systemd-resolved` activo | `resolvectl dns vboxnet0 192.168.10.10` + `resolvectl domain vboxnet0 '~cloud.local'` | Solo `cloud.local` va a `ns`; el resto sigue normal |
| Sin `systemd-resolved` | `nameserver 192.168.10.10` como primera línea de `/etc/resolv.conf` | Todo pasa por `ns`, que reenvía a internet |

El `~` en `~cloud.local` marca un **dominio de enrutamiento**: resolved manda ese
sufijo a ese servidor y no lo usa para nada más.

### Verificación

```bash
getent hosts ns.cloud.local
ping -c1 ns.cloud.local
curl -sI https://debian.org >/dev/null && echo "internet ok"
```

### Deshacer

```bash
sudo ./scripts/dns-host-linux.sh --undo
```

Restaura `nsswitch.conf` y `resolv.conf` desde los respaldos `.httpaas.bak` que
crea la primera vez, y revierte la configuración de `systemd-resolved`.

### Gotchas

- El adaptador host-only en Linux se llama `vboxnet0`. Si tu VirtualBox usa otro
  nombre, pásalo con `VBOX_IFACE=vboxnet1 sudo -E ./scripts/dns-host-linux.sh`.
- **`resolvectl` no persiste tras reiniciar.** Vuelve a correr el script.
- Si `/etc/resolv.conf` es un enlace simbólico, el script escribe a través de él
  para no romper la cadena.

---

## Problemas

| Síntoma | Causa | Solución |
|---|---|---|
| `ping paisajes.cloud.local` no resuelve en Linux | mDNS se queda con `.local` | `nsswitch.conf`: `dns` antes de `mdns4_minimal` |
| `Resolve-DnsName` funciona y `nslookup` no | `nslookup` no usa NRPT | Probar con `Resolve-DnsName` |
| La terminal resuelve y el navegador no | DNS sobre HTTPS del navegador | Desactivar DoH |
| `resolvectl` dice *Unknown device* | El adaptador no se llama `vboxnet0` | `VBOX_IFACE=<nombre>` |
| Todo falla tras reiniciar en Linux | `resolvectl` no persiste | Volver a correr el script |
