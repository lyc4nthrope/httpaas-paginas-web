# Parte B — DNS y experiencia de usuario

**Responsable:** Daniel Pérez · **Rama:** `daniel`
**Lee antes:** [plan general](../PLAN.md), §2 (arquitectura), §3 (D5, D6, D7, D9) y §4 (contrato).

---

## Para el asistente de IA (copiar tal cual)

```text
Eres el asistente de Daniel en el Parcial 3 de Computación en la Nube: un servicio HTTPaaS local
(webapp en Go que crea VMs Apache en VirtualBox, las registra en Bind9 y publica un .zip del usuario).
Repositorio: httpaas-paginas-web. Lee docs/PLAN.md y docs/parte-b/PLAN.md.

Tu parte (B) es dueña de: la VM "ns" (Debian 13 CLI, Bind9, 192.168.10.10), deploy/ns/, internal/dns/,
scripts/dns-host-linux.sh, internal/web/templates/dashboard.html, internal/web/static/ y el documento final.
No edites carpetas de A (resto de internal/, cmd/) ni de C (internal/publicar, deploy/plantilla).

Reglas obligatorias:
- La zona es dinámica: update-policy local; archivos en /var/lib/bind/; NUNCA editar la zona a mano con named
  encendido (usar rndc freeze/thaw). Siempre named-checkconf y named-checkzone antes de recargar.
- El script httpaas-dns valida host (^[a-z][a-z0-9-]{1,30}[a-z0-9]$) e IP (192.168.10.N) antes de usarlos.
  Los datos nunca se interpolan sin validar.
- En Go: implementa la interfaz puertos.DNS EXACTAMENTE como en docs/PLAN.md §4.2. Comandos remotos solo a
  través de executor.Executor con argumentos separados. Funciones puras con pruebas de tabla; Fakes para el resto.
- El contrato de docs/PLAN.md §4 NO se cambia sin acuerdo del equipo.
- Trabaja por fases (B0..B8) en orden. Una fase termina cuando su "Verificación" pasa. No avances antes.
- Antes de cada commit, `make check` en verde. Commits convencionales en inglés y descriptivos.
- Antes de escribir código o configuración, explica en 3-5 líneas qué vas a hacer y por qué.
```

---

## Qué entrega esta parte

El servidor DNS que da nombre a cada instancia, la pieza de Go que lo actualiza, la configuración para que el navegador del host resuelva `*.cloud.local`, el dashboard que ve el usuario y el documento final.

---

## Fases

### B0. Preparación ⬜ (H0)
Hacer P1–P4 del [plan general §7](../PLAN.md#7-preparación-de-cada-pc-los-tres-h0). Revisar y aprobar el PR del contrato (A1).

### B1. VM `ns` ⬜ (H1)
**Meta:** una VM Debian 13 sin interfaz gráfica, con IP fija 192.168.10.10 y acceso por SSH con llaves.
1. VM `ns`: 1 CPU, 512 MB, disco de 8 GB, BIOS. `nic1` host-only `vboxnet0`, `nic2` NAT. Instalar Debian 13 sin entorno gráfico, con "SSH server" y "standard system utilities".
2. Comprobar el nombre de las interfaces con `ip -br link`: normalmente `enp0s3` es host-only y `enp0s8` es NAT.
3. `/etc/network/interfaces`: `enp0s3` estática `192.168.10.10/24` **sin gateway**, y `enp0s8` por DHCP (ahí está la salida a internet).
4. `hostnamectl set-hostname ns` y la línea `127.0.1.1 ns.cloud.local ns` en `/etc/hosts`.
5. `apt install bind9 bind9-dnsutils sudo`.
6. Usuario `httpaas` sin contraseña (`adduser --disabled-password`). En `~httpaas/.ssh/authorized_keys`, las tres llaves de `deploy/llaves/*.pub`.
7. `/etc/ssh/sshd_config.d/10-httpaas.conf`: `PasswordAuthentication no` y `PermitRootLogin no`. Probar con `sshd -t` **antes** de `systemctl restart ssh`.
8. Snapshot `ns-base`.

**Verificación:** desde el host, `ssh -i keys/httpaas_ed25519 httpaas@192.168.10.10 hostname` → `ns`, sin pedir contraseña. Desde `ns`, `ping -c1 deb.debian.org` responde.
📸 **B-01** — `ip -br addr` en `ns`. 📸 **B-02** — SSH desde el host sin contraseña.
**Commit:** `docs(ns): record base vm setup` (con los pasos en `deploy/ns/README.md`)

### B2. Zona `cloud.local` estática ⬜ (H1)
**Meta:** que Bind9 responda por la zona y por la inversa.
1. `deploy/ns/named.conf.options`: `listen-on { 127.0.0.1; 192.168.10.10; };`, `allow-query { localhost; 192.168.10.0/24; };`, `recursion yes;`, `allow-recursion { localhost; 192.168.10.0/24; };`, `forwarders { 1.1.1.1; 8.8.8.8; };`, `dnssec-validation auto;`.
2. `deploy/ns/named.conf.local`:
   ```
   zone "cloud.local" { type master; file "/var/lib/bind/db.cloud.local"; update-policy local; };
   zone "10.168.192.in-addr.arpa" { type master; file "/var/lib/bind/db.192.168.10"; update-policy local; };
   ```
3. `db.cloud.local`: `$TTL 300`, SOA `ns.cloud.local. admin.cloud.local.` con serial `2026110201`, `NS ns.cloud.local.` y `ns A 192.168.10.10`. `db.192.168.10`: SOA, NS y `10 PTR ns.cloud.local.`
4. Copiar a `/var/lib/bind/` con dueño `bind:bind`. `named-checkconf`, `named-checkzone cloud.local /var/lib/bind/db.cloud.local` → `systemctl restart named`.

**Verificación (desde el host):** `dig @192.168.10.10 ns.cloud.local +short` → `192.168.10.10`. `dig @192.168.10.10 -x 192.168.10.10 +short` → `ns.cloud.local.` `dig @192.168.10.10 debian.org +short` → una IP (la recursión funciona).
📸 **B-03** — `named-checkconf` y `named-checkzone` sin errores. 📸 **B-04** — los tres `dig`.
**Commit:** `feat(ns): serve cloud.local and its reverse zone`

### B3. Actualización dinámica con `httpaas-dns` ⬜ (H1)
**Meta:** registrar y quitar hosts con un solo comando, que es lo que llamará la webapp.
1. `deploy/ns/httpaas-dns` (bash, `set -euo pipefail`). Valida el host con `^[a-z][a-z0-9-]{1,30}[a-z0-9]$` y la IP con `^192\.168\.10\.([1-9][0-9]?|1[0-9]{2}|2[0-4][0-9]|25[0-4])$`. Rechaza `ns`.
   - `agregar H IP`: una sola llamada a `nsupdate -l` con **dos bloques, uno por zona, cada uno con su propio `send`**. Un mensaje de actualización solo puede tocar una zona; si se mezclan, `nsupdate` falla con `NOTZONE`. `N` es el último octeto de la IP:
     ```
     zone cloud.local
     update delete H.cloud.local A
     update add H.cloud.local 300 A IP
     send
     zone 10.168.192.in-addr.arpa
     update delete N.10.168.192.in-addr.arpa PTR
     update add N.10.168.192.in-addr.arpa 300 PTR H.cloud.local.
     send
     ```
     Después confirma con `dig @127.0.0.1 +short H.cloud.local` = IP y `dig @127.0.0.1 +short -x IP` = `H.cloud.local.`. Si no coincide, sale con código 1.
   - `quitar H`: primero obtiene la IP actual con `dig +short H.cloud.local` (para saber qué PTR borrar). Después, dos bloques igual que arriba: `update delete H.cloud.local A` + `send`, y `update delete N.10.168.192.in-addr.arpa PTR` + `send`. No falla si no existía.
   - `listar`: `dig @127.0.0.1 cloud.local AXFR` filtrado a `host ip`. Para que funcione, agregar `allow-transfer { localhost; };` a la zona.
2. Instalar en `/usr/local/sbin/httpaas-dns` (root, 0755). `/etc/sudoers.d/httpaas`: `httpaas ALL=(root) NOPASSWD: /usr/local/sbin/httpaas-dns`, validado con `visudo -cf`.
3. `deploy/ns/instalar.sh`: hace B1.5 a B3.2 en una VM Debian recién instalada (D14). Debe poder correrse dos veces sin romper nada.

**Verificación (desde el host):**
```bash
ssh httpaas@192.168.10.10 sudo httpaas-dns agregar prueba 192.168.10.99   # código 0
dig @192.168.10.10 prueba.cloud.local +short                              # 192.168.10.99
dig @192.168.10.10 -x 192.168.10.99 +short                                # prueba.cloud.local.
ssh httpaas@192.168.10.10 sudo httpaas-dns agregar 'x;id' 192.168.10.99   # rechazado, código ≠ 0
ssh httpaas@192.168.10.10 sudo httpaas-dns quitar prueba && dig @192.168.10.10 prueba.cloud.local +short   # vacío
ssh httpaas@192.168.10.10 sudo cat /etc/shadow                            # rechazado por sudo
```
📸 **B-05** — agregar + `dig`. 📸 **B-06** — rechazo del nombre malicioso y de `sudo cat`. 📸 **B-07** — archivo `.jnl` en `/var/lib/bind/` (prueba de que la actualización fue dinámica).
**Commit:** `feat(ns): add httpaas-dns for dynamic a and ptr records`

### B4. Exportar `ns` para el equipo ⬜ (H2)
`VBoxManage export ns -o ns.ova` y subirlo a la carpeta compartida del equipo (no al repositorio). A y C lo importan con `VBoxManage import ns.ova`.
**Verificación:** A o C confirman B3 desde su PC.

### B5. Resolver `*.cloud.local` desde el host ⬜ (H2)
**Meta:** que el navegador abra `http://paisajes.cloud.local`.
1. **Linux:**
   - `/etc/nsswitch.conf`, línea `hosts:` → `files myhostname dns mdns4_minimal [NOTFOUND=return]` (`dns` antes de mDNS; ver §1 del plan general).
   - Si `systemctl is-active systemd-resolved` está activo: `resolvectl dns vboxnet0 192.168.10.10` y `resolvectl domain vboxnet0 '~cloud.local'` (DNS dividido: solo `cloud.local` va a `ns`).
   - Si no: agregar `nameserver 192.168.10.10` como **primera** línea de `/etc/resolv.conf`. `ns` también resuelve internet por la recursión de B2.
   - Todo esto en `scripts/dns-host-linux.sh` (idempotente; pide `sudo`).
2. **Windows (PowerShell como administrador):** `Add-DnsClientNrptRule -Namespace ".cloud.local" -NameServers "192.168.10.10"`.
3. Documentar los dos procedimientos en `docs/parte-b/dns-host.md`, incluyendo cómo deshacerlos.
4. Firefox: si no resuelve, desactivar "DNS sobre HTTPS" para la prueba.

**Verificación:** `getent hosts ns.cloud.local` (Linux) o `Resolve-DnsName ns.cloud.local` (Windows) → `192.168.10.10`. `ping -c1 ns.cloud.local` responde. Internet sigue funcionando (`curl -sI https://debian.org`).
📸 **B-08** — `getent`/`Resolve-DnsName` y `ping` desde el host.
**Commit:** `feat(scripts): resolve cloud.local from the host`

### B6. Adaptador Go `internal/dns` ⬜ (H2)
**Meta:** que la webapp maneje el DNS a través de la interfaz `puertos.DNS`.
1. `Nuevo(ex executor.Executor, cfg)`.
2. `Agregar(host, ip)` → `ex.Run(Command{Name:"sudo", Args:["/usr/local/sbin/httpaas-dns","agregar",host,ip]})`. `Quitar` es igual. Antes de ejecutar, valida con `dominio.ValidarHostname` y con la IP dentro del rango.
3. `Resolver(host)`: `net.Resolver{PreferGo: true, Dial: …}` que **siempre** dialoga con `192.168.10.10:53`. Así no depende de cómo esté configurado el host.
4. Pruebas: con `executor.Fake`, comprobar los argumentos exactos; `Agregar` con un nombre inválido no llama al executor; un código de salida distinto de 0 se convierte en un error con el stderr incluido. Prueba `vm`: `Agregar` → `Resolver` → `Quitar` contra el `ns` real.

**Verificación:** `make check` y `make vm-test` en verde.
**Commit:** `feat(dns): implement dns port over ssh and httpaas-dns`

### B7. Dashboard ⬜ (H3)
**Meta:** la pantalla de la figura del profesor (página 11 de `Imagenes_enunciado_parcial3.pdf`).
1. `internal/web/templates/dashboard.html` (`html/template`, con escape automático):
   - Título "Gestión de páginas y aplicaciones web".
   - Un formulario con el campo "Nombre del host" y dos botones: **Aceptar** (`formaction="/instancias"`) y **Publicar**, con el campo "Contenido web (comprimido)" (`accept=".zip"`, `formaction="/instancias/{nombre}/contenido"`, armado con un poco de JS a partir del nombre escrito). Los nombres de los campos son las constantes de A5.
   - Tabla "Instancias disponibles": enlace `http://<host>.cloud.local` (`target="_blank" rel="noopener"`), `IP/24`, nombre de la VM, fecha y hora de creación, estado y el botón **Eliminar** con `confirm()`.
   - Estado vacío: "Aún no hay instancias. Escribe un nombre y pulsa Aceptar".
   - Mensajes flash y errores por campo (llegan de A5).
2. `static/app.js`: mientras haya una instancia en `creando`, `publicando` o `eliminando`, consulta `GET /api/instancias` cada 3 s y actualiza la tabla. Después deja de consultar.
3. `static/estilos.css`: legible, que funcione con un ancho de 360 px, y un estado con color **y** texto (no solo color).
4. Revisar con la lista de `web-design-guidelines`: etiquetas en los campos, foco visible y botones con texto.

**Verificación:** con `make simular`, crear, ver el estado cambiar solo, publicar y eliminar. El enlace abre en una pestaña nueva.
📸 **B-09** — dashboard vacío. 📸 **B-10** — dos instancias, como en la figura. 📸 **B-11** — vista en el ancho de un celular.
**Commit:** `feat(web): build the instances dashboard`

### B8. Documento final ⬜ (H5)
**Meta:** el documento que pide el enunciado, con todo el equipo.
1. `docs/DOCUMENTO.md`, que se exporta a PDF. Estructura: portada → introducción y objetivos → arquitectura (diagrama de C) → secciones 2 a 5 según la tabla del §9 del plan general, con las capturas en orden → problemas encontrados, mejoras y trabajo futuro (desde `docs/problemas.md`) → conocimientos aprendidos (un párrafo de cada uno) → uso de IA → referencias.
2. Pedir a A y a C su sección en H4. Cada captura lleva un pie que dice qué se ve y por qué importa.
3. Revisión cruzada: A y C leen el documento completo antes de H5.

**Verificación:** las capturas P, A, B y C de la tabla del §9 están todas y en orden. Los tres aprobaron.
**Commit:** `docs: add final report`

---

## Errores comunes en esta parte

- **`named` no arranca después de editar:** siempre `named-checkconf` y `named-checkzone`, y revisar `journalctl -u named -n 30`.
- **"permission denied" al escribir el `.jnl`:** la zona no está en `/var/lib/bind` o no es de `bind:bind` (AppArmor).
- **Cambios a mano que "desaparecen":** se editó la zona con el journal activo. Usar `rndc freeze cloud.local` → editar y subir el serial → `rndc thaw cloud.local`.
- **`nsupdate -l` dice "no session key":** falta `update-policy local;` en la zona o no se reinició `named`.
- **`ns` sin internet:** se puso gateway en `enp0s3`. El gateway va solo por `enp0s8` (NAT).
