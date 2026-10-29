# Parte C — Plantilla Apache y publicación

**Responsable:** Wiliam Carmona · **Rama:** `wiliam`
**Lee antes:** [plan general](../PLAN.md), §2 (arquitectura), §3 (D3, D7, D8, D11, D16) y §4.3–4.4 (contrato).

---

## Para el asistente de IA (copiar tal cual)

```text
Eres el asistente de Wiliam en el Parcial 3 de Computación en la Nube: un servicio HTTPaaS local
(webapp en Go que crea VMs Apache en VirtualBox desde un disco multiconexión, las registra en Bind9 y
publica un .zip del usuario). Repositorio: httpaas-paginas-web. Lee docs/PLAN.md y docs/parte-c/PLAN.md.

Tu parte (C) es dueña de: la VM "apache-plantilla" (Debian 13 CLI + Apache) y su disco ApacheServer.vdi en
modo multiconexión, deploy/plantilla/, internal/publicar/, testdata/zips/, docs/parte-c/ y el diagrama.
No edites carpetas de A (resto de internal/, cmd/) ni de B (internal/dns, deploy/ns, dashboard).

Reglas obligatorias:
- Los clones se configuran solos al arrancar leyendo guest properties /httpaas/{hostname,ip,dominio,dns}
  y escriben /httpaas/estado = configurando | listo | error:<detalle> (docs/PLAN.md §4.3). No cambies esos nombres.
- Scripts bash con `set -euo pipefail`; todo argumento se valida con regex antes de usarse; nunca eval ni sh -c
  con datos externos. Los cambios de configuración se prueban (apache2ctl configtest, sshd -t, visudo -cf)
  ANTES de aplicarlos.
- El zip es dato hostil: se rechazan rutas absolutas, "..", enlaces simbólicos, más de 2000 archivos,
  más de 200 MB descomprimidos y zips sin index.html (en la raíz o dentro de una única carpeta).
- En Go: implementa puertos.Publicador EXACTAMENTE como en docs/PLAN.md §4.2. Comandos remotos solo por
  executor.Executor con argumentos separados. ValidarZip es una función pura con pruebas de tabla.
- Antes de probar algo que modifica la plantilla, toma un snapshot. Nunca conviertas el disco a
  multiconexión sin un respaldo (clonemedium).
- Trabaja por fases (C0..C8) en orden. Una fase termina cuando su "Verificación" pasa. No avances antes.
- Antes de cada commit, `make check` en verde. Commits convencionales en inglés y descriptivos.
- Antes de escribir código o configuración, explica en 3-5 líneas qué vas a hacer y por qué.
```

---

## Qué entrega esta parte

La "imagen dorada" de la nube: una VM Apache que, al clonarse, se configura sola con su nombre, su IP y su sitio. También el procedimiento exacto de clonación (que A automatiza), la publicación segura del `.zip` y el diagrama de arquitectura.

---

## Fases

### C0. Preparación ⬜ (H0)
Hacer P1–P4 del [plan general §7](../PLAN.md#7-preparación-de-cada-pc-los-tres-h0). Revisar y aprobar el PR del contrato (A1).

### C1. VM `apache-plantilla` base ⬜ (H1)
**Meta:** Debian 13 sin interfaz gráfica, con Apache y acceso por SSH con llaves.
1. VM `apache-plantilla`: 1 CPU, 512 MB, disco **dinámico de 6 GB llamado `ApacheServer.vdi`**, **BIOS**, controlador **SATA (AHCI)** (D16). `nic1` host-only `vboxnet0`, `nic2` NAT.
2. Instalar Debian 13: sin entorno gráfico, con "SSH server" y "standard system utilities".
3. Red: confirmar los nombres con `ip -br link`. Cada interfaz va en **su propio archivo** dentro de `interfaces.d/`, porque el primer arranque de los clones reescribe solo el de `enp0s3`:
   - `/etc/network/interfaces`: **solo** `source /etc/network/interfaces.d/*` y la interfaz `lo`. Nada de `enp0s3` ni de `enp0s8`.
   - `/etc/network/interfaces.d/enp0s3`: `auto enp0s3`, `iface enp0s3 inet static`, `address 192.168.10.20/24` (IP de mantenimiento), **sin** `gateway`.
   - `/etc/network/interfaces.d/enp0s8`: `auto enp0s8`, `iface enp0s8 inet dhcp` (salida a internet por NAT).
   - Verificación: `grep -c enp0s3 /etc/network/interfaces` → `0`, y `ip -br addr` muestra `.20` en `enp0s3`.
4. `apt install apache2 unzip sudo curl`. Usuario `httpaas` sin contraseña, con las tres llaves de `deploy/llaves/*.pub`. Crear `/home/httpaas/subidas` (dueño `httpaas`, 0700).
5. `sshd_config.d/10-httpaas.conf`: `PasswordAuthentication no` y `PermitRootLogin no` (`sshd -t` antes de reiniciar).
6. Snapshot `c1-base`.

**Verificación:** desde el host, `ssh httpaas@192.168.10.20 hostname` sin contraseña. `curl -s http://192.168.10.20/ | grep -i apache` muestra la página por defecto.
📸 **C-01** — configuración de la VM (`VBoxManage showvminfo apache-plantilla | grep -E 'Firmware|Storage|NIC'`). 📸 **C-02** — página por defecto de Apache desde el navegador del host.
**Commit:** `docs(plantilla): record base template vm setup`

### C2. Guest Additions ⬜ (H1)
**Meta:** que la VM pueda leer los datos que le deja el host.
1. `apt install build-essential linux-headers-$(uname -r) bzip2`.
2. Host: `VBoxManage storageattach apache-plantilla --storagectl SATA --port 1 --device 0 --type dvddrive --medium additions`. VM: `mount /dev/sr0 /mnt && sh /mnt/VBoxLinuxAdditions.run --nox11` → reiniciar → quitar el disco de las Guest Additions.
3. Si falla la compilación, **avisar al equipo el mismo día**: se activa el plan B del §11 del plan general.

**Verificación:**
```bash
VBoxManage guestproperty set apache-plantilla /httpaas/hostname prueba     # en el host
VBoxControl --nologo guestproperty get /httpaas/hostname                   # en la VM → "Value: prueba"
VBoxControl --nologo guestproperty set /httpaas/estado hola                # en la VM
VBoxManage guestproperty get apache-plantilla /httpaas/estado              # en el host → "Value: hola"
VBoxManage guestproperty unset apache-plantilla /httpaas/hostname; VBoxManage guestproperty unset apache-plantilla /httpaas/estado
```
📸 **C-03** — lectura y escritura de guest properties en ambas direcciones.
Snapshot `c2-additions`.

### C3. Servicio de primer arranque `httpaas-firstboot` ⬜ (H1) — la pieza más importante
**Meta:** que un clon se convierta solo en `paisajes` con la IP `.30`.
1. `deploy/plantilla/httpaas-firstboot` (bash):
   1. `get()` lee una propiedad con `VBoxControl --nologo guestproperty get` y reintenta hasta 30 s (el driver puede tardar en cargar).
   2. Si `/httpaas/hostname` está vacío, **sale con 0 sin hacer nada**: es la plantilla misma.
   3. Valida: hostname (regex D10), IP `192.168.10.N/24`, dominio y DNS. Si algo es inválido → `estado=error:datos-invalidos` y sale.
   4. Si `/etc/httpaas/aplicado` contiene `"<host> <ip>"`, ya estaba configurada (es un reinicio): `estado=listo` y sale.
   5. `estado=configurando`. `trap` en ERR que escribe `estado=error:linea-$LINENO`.
   6. `hostnamectl set-hostname H` y `/etc/hosts` (`127.0.1.1 H.cloud.local H`).
   7. Identidad única: `rm -f /etc/ssh/ssh_host_*` → `ssh-keygen -A`; vaciar `/etc/machine-id` → `systemd-machine-id-setup`.
   8. Red, **en este orden**: `ifdown enp0s3` (mientras el archivo todavía tiene la `.20`, para que ifupdown sepa qué quitar) → reescribir `/etc/network/interfaces.d/enp0s3` con la IP nueva, sin gateway → `ifup enp0s3`. Si `ifdown` falla, ejecutar `ip addr flush dev enp0s3` antes del `ifup`. Solo se toca ese archivo; `/etc/network/interfaces` no se modifica (C1.3). Después, `/etc/resolv.conf` con `nameserver 192.168.10.10` y `search cloud.local`.
   9. Apache: `/var/www/H/index.html` provisional ("Instancia H lista. Aún no hay contenido publicado.") → `/etc/apache2/sites-available/H.conf` desde la plantilla `deploy/plantilla/vhost.conf.tmpl` (`ServerName H.cloud.local`, `DocumentRoot /var/www/H`, `ErrorLog`/`CustomLog` propios) → `a2ensite H` → `a2dissite 000-default` → `apache2ctl configtest` → `systemctl reload apache2`.
   10. `systemctl restart ssh`. Escribir `/etc/httpaas/aplicado`. Por último, `estado=listo`.
2. `httpaas-firstboot.service`: `Type=oneshot`, `RemainAfterExit=yes`, `After=vboxadd-service.service network.target`, `Wants=vboxadd-service.service`, `WantedBy=multi-user.target`. `systemctl enable httpaas-firstboot`.
3. **Probarlo en la plantilla sin dañarla:** snapshot `c3-antes` → en el host, `guestproperty set` de las 4 propiedades (`prueba`, `192.168.10.29/24`…) → reiniciar → verificar → **restaurar el snapshot `c3-antes`** y borrar las propiedades.

**Verificación (durante la prueba):** `VBoxManage guestproperty get apache-plantilla /httpaas/estado` → `listo`. `ssh httpaas@192.168.10.29 hostname` → `prueba` (y SSH avisa que es una llave nueva). `curl -s -H 'Host: prueba.cloud.local' http://192.168.10.29/` → la página provisional. Con datos inválidos (`/httpaas/hostname = "X;rm"`) → `estado=error:datos-invalidos`.
📸 **C-04** — `systemctl status httpaas-firstboot`. 📸 **C-05** — estado `listo` visto desde el host. 📸 **C-06** — página provisional con el `Host` correcto.
**Commit:** `feat(plantilla): self-configure clones from guest properties on boot`

### C4. Script de publicación `httpaas-publicar` ⬜ (H2)
**Meta:** desplegar un zip de forma segura y sin dejar el sitio a medias.
1. `deploy/plantilla/httpaas-publicar ZIP`:
   1. El argumento debe coincidir con `^/home/httpaas/subidas/[a-zA-Z0-9-]{8,64}\.zip$` y existir. Si no, código 2.
   2. El nombre de la instancia `H` **no llega como argumento**: se lee del primer campo de `/etc/httpaas/aplicado`, que escribe `httpaas-firstboot` (C3.1.10), y se valida con la regex de D10. Si el archivo no existe o el nombre es inválido, código 3: la instancia no terminó su primer arranque. Así nadie puede publicar en la carpeta de otro sitio.
   3. `unzip -Z1` lista las entradas: si alguna empieza con `/` o contiene `..`, código 2. `zipinfo` sin enlaces simbólicos (`^l`). Máximo 2000 entradas y 200 MB (suma de `unzip -Zt`).
   4. Descomprimir en `mktemp -d` (en `/var/www/.tmp-H`). Si no hay `index.html` en la raíz y hay **una sola** carpeta, usar esa carpeta como raíz. Si aun así no hay `index.html`, código 2.
   5. `chown -R www-data:www-data`, permisos 0755/0644 → `mv /var/www/H /var/www/.old-H && mv <nuevo> /var/www/H` → `apache2ctl configtest && systemctl reload apache2` (si falla, se restaura `.old-H` y código 3) → borrar `.old-H` y el zip.
   6. Imprimir `publicado <n> archivos`.
2. `/etc/sudoers.d/httpaas`: `httpaas ALL=(root) NOPASSWD: /usr/local/sbin/httpaas-publicar` (`visudo -cf`).
3. `testdata/zips/`: `paisajes.zip` (un `index.html` con una imagen de paisaje), `negocios.zip` (una carpeta `negocios/` con `index.html` y `css/`, para probar el caso de la carpeta única), `sin-index.zip`, `zipslip.zip` (entrada `../../tmp/x`) y `symlink.zip`. Agregar el script `testdata/zips/generar.sh` que los crea, para que se puedan reconstruir.

**Verificación:** repetir la prueba de C3.3 (snapshot `c3-antes` → propiedades de `prueba` → reiniciar → `estado=listo`) y, **antes de restaurar el snapshot**: `scp` de cada zip a `subidas/` → `sudo httpaas-publicar …`. `paisajes` y `negocios` → código 0 y `curl` muestra el contenido. Los tres maliciosos → código 2, y el sitio anterior **sigue** funcionando. `sudo cat /etc/shadow` → rechazado.
📸 **C-07** — publicación correcta. 📸 **C-08** — rechazo de `zipslip.zip`.
**Commit:** `feat(plantilla): publish user zips atomically with validation`

### C5. Script `instalar.sh` y disco multiconexión ⬜ (H2)
**Meta:** congelar la plantilla y dejar el procedimiento reproducible.
1. `deploy/plantilla/instalar.sh`: aplica C1.4–C4.2 (paquetes, usuario, llaves, sshd, scripts, unit, sudoers) en una Debian 13 recién instalada. Puede correrse dos veces. Las Guest Additions (C2) quedan como paso manual documentado.
2. Limpiar la plantilla: `apt clean`, borrar `/etc/httpaas/aplicado` si existe, `history -c` → `poweroff`.
3. Desconectar y mover el disco (procedimiento del Lab #4, por CLI):
   ```bash
   VBoxManage storageattach apache-plantilla --storagectl SATA --port 0 --device 0 --medium none
   mkdir -p "$HOME/VirtualBox VMs/httpaas-base"
   VBoxManage modifymedium disk "<ruta actual>/ApacheServer.vdi" --move "$HOME/VirtualBox VMs/httpaas-base/ApacheServer.vdi"
   VBoxManage clonemedium disk "$HOME/VirtualBox VMs/httpaas-base/ApacheServer.vdi" "$HOME/httpaas-respaldo/ApacheServer-respaldo.vdi"
   VBoxManage modifymedium disk "$HOME/VirtualBox VMs/httpaas-base/ApacheServer.vdi" --type multiattach
   ```
   Si la plantilla tiene snapshots, primero hay que borrarlos (`VBoxManage snapshot apache-plantilla delete …`), porque el disco no se puede mover con hijos.
4. Para cambiar la plantilla más adelante: no puede haber clones → `--type normal` → conectar → editar → repetir el paso 3.

**Verificación:** `VBoxManage showmediuminfo disk "$HOME/VirtualBox VMs/httpaas-base/ApacheServer.vdi"` → `Type: multiattach`. El respaldo existe.
📸 **C-09** — `showmediuminfo` con `Type: multiattach`.
**Commit:** `feat(plantilla): add reproducible install script for the template`

### C6. Clonación manual y especificación para A ⬜ (H2, lo primero del día) — **bloquea la prueba real de A6**
> Avisar a Cristhian en cuanto `clon-manual.md` esté en `dev`: A6 no puede probar con VMs reales sin este documento.
**Meta:** probar a mano la secuencia exacta que A va a automatizar.
1. `docs/parte-c/clon-manual.md`: los comandos uno por uno (`createvm`, `modifyvm`, `storagectl`, `storageattach`, `guestproperty set` ×4, `startvm --type headless`, esperar `listo`, `controlvm poweroff`, `unregistervm --delete`) y la salida esperada de cada uno.
2. Ejecutarlos para `paisajes` (`.30`) y `negocios` (`.31`) **al mismo tiempo**.
3. Eliminar las dos y comprobar que `ApacheServer.vdi` sigue existiendo (es la misma prueba crítica de A6; si falla, avisar a A).
4. Exportar el `.vdi` base comprimido a la carpeta compartida del equipo, junto con su checksum (`sha256sum`).

**Verificación:** las dos instancias responden por SSH con su propio hostname y con llaves de host **distintas** (`ssh-keyscan` de las dos IPs y comparar). `curl -H 'Host: paisajes.cloud.local' http://192.168.10.30/` funciona. Después de eliminarlas, `VBoxManage list hdds` sigue mostrando el disco base.
📸 **C-10** — las dos VMs corriendo, con sus discos diferenciales en `VBoxManage list hdds`. 📸 **C-11** — después de eliminar, el disco base intacto.
**Commit:** `docs(plantilla): specify the manual clone procedure`

### C7. Adaptador Go `internal/publicar` ⬜ (H3)
**Meta:** que la webapp valide y publique el zip.
1. `ValidarZip(ruta string, lim Limites) (Resumen, error)`, una función pura con `archive/zip`. Aplica las mismas reglas que C4.1.3–4, y además rechaza las entradas cuyo `filepath.IsLocal` sea falso. `Resumen{Archivos int, Bytes int64, Raiz string}`. Los errores son legibles para el usuario ("El zip contiene rutas peligrosas: ../../etc/passwd").
2. `Publicador.Publicar(host, ip, zip)`: `id := hex aleatorio` → `ex.CopyTo(zip, "/home/httpaas/subidas/<id>.zip")` → `ex.Run(sudo /usr/local/sbin/httpaas-publicar <ruta>)`. El código 2 se convierte en un error de validación y el 3 en un error de servidor → verificación `GET http://<ip>/` con `Host: <host>.cloud.local` → 200.
3. Pruebas de tabla de `ValidarZip` con los zips de `testdata/zips` y con zips generados en memoria (rutas absolutas, `..`, symlink, 2001 entradas, tamaño declarado enorme, vacío, no es un zip). Pruebas de `Publicar` con `executor.Fake` (argumentos exactos, mapeo de códigos). Prueba `vm` contra una instancia real.

**Verificación:** `make check` y `make vm-test` en verde.
**Commit:** `feat(publicar): validate and deploy user zips to instances`

### C8. Diagrama y material de demostración ⬜ (H4)
1. Diagrama de arquitectura (draw.io o Excalidraw) con lo mismo del §2 del plan general: host, webapp, red, `ns`, instancias, disco multiconexión y flechas de los tres flujos. Exportar `docs/arquitectura.png` y guardar también el archivo fuente.
2. Escribir para B la sección "Plantilla Apache y disco multiconexión" (½ página + capturas C-xx).

📸 **C-12** — el diagrama.
**Commit:** `docs: add architecture diagram`

---

## Errores comunes en esta parte

- **El clon arranca con la IP `.20`:** `httpaas-firstboot` corrió antes de que el driver de Guest Additions leyera las propiedades. Revisar los reintentos de `get()` y `journalctl -u httpaas-firstboot`.
- **Dos clones con la misma llave SSH:** faltó el paso C3.1.7. A lo detecta como `REMOTE HOST IDENTIFICATION HAS CHANGED`.
- **"Cannot change type for medium … it has child media":** el disco tiene snapshots o clones. Primero borrarlos.
- **Apache muestra la página de Debian y no la del sitio:** falta `a2dissite 000-default` o el `ServerName` no coincide con el nombre pedido.
- **`unzip` deja archivos con dueño root:** faltó el `chown -R www-data:www-data` antes del `mv`.
