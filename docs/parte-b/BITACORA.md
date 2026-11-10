# Bitácora de la parte B

Registro de lo ejecutado en la parte B (DNS y experiencia de usuario), fase por fase.
El plan vive en [`PLAN.md`](PLAN.md), los problemas del proyecto se anotan en
[`docs/problemas.md`](../problemas.md) y las capturas en `docs/capturas/<código>.png`.

Cada fase se marca ✅ cuando su sección **Verificación** del plan pasa contra la VM real.

**Shells:** los comandos del host se ejecutan en **`cmd`** (no se usa PowerShell en
esta parte); los de la VM, en la consola Debian (bash). Por eso las consultas desde
el host se documentan con `nslookup`, no con `Resolve-DnsName`.

---

## B0. Prerrequisitos del host — ✅ 2026-11-07

Preparar el PC que corre VirtualBox y la webapp. No toca ninguna VM.

| Paso | Qué se hizo |
|---|---|
| P1 | **No aplica en Windows.** La restricción de rangos de `/etc/vbox/networks.conf` solo existe en Linux y macOS |
| P2 | Adaptador host-only `VirtualBox Host-Only Ethernet Adapter #2` con `192.168.10.1/24` y **DHCP desactivado**. Convive con un segundo adaptador `192.168.56.1` de otro curso, en un conmutador distinto |
| P3 | Par de llaves `keys/httpaas_ed25519` y `keys/httpaas_ed25519.pub`. La pública se versiona como `deploy/llaves/daniel.pub`; `keys/` está en `.gitignore` |
| P4 | Herramientas: Go 1.27.0, OpenSSH 9.5p2, VBoxManage 7.2.16 |

📸 `P-01` — `ipconfig` mostrando el adaptador host-only en `192.168.10.1`.

**Por qué dos adaptadores y no uno.** El host necesita *iniciar* conexiones hacia
las VMs (SSH, SCP, HTTP) con direcciones predecibles, y eso solo lo da host-only.
Las VMs necesitan salir a internet para `apt`, y eso solo lo da NAT. Un único
adaptador no cumple las dos cosas.

---

## B1. VM `ns` — ✅ 2026-11-07

Una Debian 13 sin entorno gráfico, con IP fija `192.168.10.10` y acceso por SSH con
llaves. Los pasos reproducibles están en [`deploy/ns/README.md`](../../deploy/ns/README.md).

### Lo que se hizo

1. VM `ns` con `nic1` host-only (adaptador de `192.168.10.1`) y `nic2` NAT.
2. Interfaces confirmadas con `ip -br link`: `enp0s3` = host-only, `enp0s8` = NAT.
3. `/etc/network/interfaces`: `enp0s3` estática `192.168.10.10/24` **sin gateway**;
   `enp0s8` por DHCP.
4. `hostnamectl set-hostname ns` y `127.0.1.1 ns.cloud.local ns` en `/etc/hosts`.
5. `apt install bind9 bind9-dnsutils sudo`.
6. Usuario `httpaas` sin contraseña y las tres llaves de `deploy/llaves/*.pub` en su
   `authorized_keys`.
7. `/etc/ssh/sshd_config.d/10-httpaas.conf` con `PasswordAuthentication no` y
   `PermitRootLogin no`, validado con `sshd -t`.
8. Instantánea `ns-base` con la VM apagada.

### Verificación (salida real)

```text
ns                                    <- hostname
ns.cloud.local                        <- hostname -f
enp0s3  UP  192.168.10.10/24
enp0s8  UP  10.0.3.15/24              <- DHCP del NAT
1 packets transmitted, 1 received, 0% packet loss   <- ping deb.debian.org
```

Y desde el host, sin contraseña:

```bat
ssh -i keys/httpaas_ed25519 httpaas@192.168.10.10 hostname   # -> ns
```

📸 `B-01` — `ip -br addr` en `ns`. 📸 `B-02` — SSH desde el host sin contraseña.

### Decisiones

- **`ns` no se aprovisiona desde la webapp.** Es plataforma previa (D14). El
  producto la consume y, si no responde, falla con un mensaje claro (B6.5).
  Automatizarlo es trabajo futuro M2.
- **Se mantiene EFI** aunque el plan diga BIOS: el disco ya estaba instalado así y
  cambiarlo obliga a reinstalar el cargador. La restricción de BIOS del proyecto es
  del disco multiconexión (plantilla e instancias), no de `ns`.
- **El endurecimiento de `sshd` va después de probar la llave.** Al revés, un error
  en `authorized_keys` deja la VM solo accesible por consola.
- **La instantánea se toma con la VM apagada.** Una instantánea con RAM guardada es
  más frágil como imagen base.
- **La VM se resuelve a sí misma.** El DHCP de `enp0s8` entregó un DNS que la VM no
  alcanzaba (`192.168.1.1`, 100 % de pérdida), y con eso `apt` y `dig` sin `@servidor`
  fallaban. Se antepuso `127.0.0.1` (el propio `named`) y `10.0.3.3` (DNS del NAT)
  en `/etc/dhcp/dhclient.conf`. Se corrigió en B3, pero es un paso de B1 que ahora
  aplica `instalar.sh` antes de instalar paquetes.

### Problemas encontrados

Detalle completo en [`docs/problemas.md`](../problemas.md).

| Síntoma | Causa | Solución |
|---|---|---|
| `ns` no arrancaba: *no bootable option or device* | El NVRAM de la VM estaba vacío: la entrada de arranque se pierde al crear una VM copiando el VDI en vez de usar `VBoxManage clonevm` | `grub-install --removable`, que crea `\EFI\BOOT\BOOTX64.EFI` |
| `grub-install: command not found` | Solo estaba el GRUB que arranca, no el que reinstala | `apt install grub-efi-amd64` |
| `apt` falla con *Temporary failure resolving* | `enp0s8` (NAT) estaba `DOWN` y `resolv.conf` traía DNS del ISP heredados del clon | `/etc/network/interfaces` con `enp0s3` estática sin gateway y `enp0s8` DHCP |

---

## B2. Zona `cloud.local` estática — ✅ 2026-11-08

`ns` pasa a ser autoritativo de `cloud.local` y de su zona inversa. Los cuatro
archivos viven versionados en [`deploy/ns/`](../../deploy/ns/) y se copian a la VM;
el repo es la fuente de verdad, no la VM.

### Archivos

| Archivo en el repo | Destino en `ns` | Qué hace |
|---|---|---|
| `named.conf.options` | `/etc/bind/named.conf.options` | Escucha en `127.0.0.1` y `192.168.10.10`, atiende solo la red del proyecto, recursión con reenvío a `1.1.1.1`/`8.8.8.8` |
| `named.conf.local` | `/etc/bind/named.conf.local` | Declara las dos zonas como `type primary` con `update-policy local` |
| `db.cloud.local` | `/var/lib/bind/db.cloud.local` | Zona directa: `ns A 192.168.10.10` |
| `db.192.168.10` | `/var/lib/bind/db.192.168.10` | Zona inversa: `10 PTR ns.cloud.local.` |

### Shells usados

- **Host (Windows): `cmd`.** No se está usando PowerShell en esta parte.
- **VM `ns`: consola Debian (bash).**

Por eso todas las consultas desde el host se documentan con `nslookup`, que viene
en cualquier Windows, y no con `Resolve-DnsName`, que es de PowerShell.

### Lo que se hizo

1. Escribir los cuatro archivos en `deploy/ns/`.
2. Copiarlos a `ns` con `scp` como `httpaas` (a `/tmp`).
3. Instalarlos con `install` fijando dueño y modo: `root:bind` en `/etc/bind`,
   `bind:bind` en `/var/lib/bind`.
4. Validar con `named-checkconf` y `named-checkzone` **antes** de reiniciar.
5. `systemctl restart named` y verificar con `dig` dentro de la VM y `nslookup`
   desde `cmd`.

### Qué hace cada comando

Desde `cmd`, en el host:

| Comando | Qué hace |
|---|---|
| `scp -i keys/httpaas_ed25519 ... httpaas@192.168.10.10:/tmp/` | Copia por SSH. `-i` indica la llave privada; el destino `usuario@ip:/ruta` escribe en `/tmp` de la VM |
| `-o BatchMode=yes` | Si la llave falla, aborta en vez de pedir contraseña (la cuenta no tiene) |
| `-o UserKnownHostsFile=keys/known_hosts` | Guarda/lee la huella del host en el archivo que usará la webapp, no en el perfil de Windows |
| `nslookup <nombre> <servidor>` | Consulta DNS. El **segundo argumento** es el servidor a preguntar; sin él, Windows usa el DNS de la interfaz |
| `nslookup <ip> <servidor>` | Si el primer argumento es una IP, `nslookup` hace una consulta **PTR** (inversa) |

Dentro de la VM:

| Comando | Qué hace |
|---|---|
| `install -o root -g bind -m 0644 origen destino` | Copia y fija dueño, grupo y permisos en un solo paso (equivale a `cp` + `chown` + `chmod`, pero sin ventana intermedia) |
| `named-checkconf` | Valida la sintaxis de todo el árbol `named.conf*`. **No imprime nada si está bien** |
| `named-checkzone <zona> <archivo>` | Parsea un archivo de zona, comprueba SOA/NS y reporta el serial cargado |
| `systemctl restart named` | Detiene y arranca el servicio: relee configuración **y todos los archivos de zona** desde disco |
| `systemctl is-active named` | Devuelve `active` o `inactive`; sirve para confirmar que no quedó caído |
| `dig @127.0.0.1 <nombre> +short` | Consulta al servidor indicado (`@127.0.0.1` evita salir por la red) y `+short` deja solo la respuesta |
| `dig @127.0.0.1 -x <ip> +short` | `-x` hace la consulta inversa (PTR) |

### Verificación (salida real)

Dentro de `ns`:

```text
$ dig @127.0.0.1 ns.cloud.local +short
192.168.10.10

$ dig @127.0.0.1 SOA cloud.local +short
ns.cloud.local. admin.cloud.local. 2026110702 3600 900 604800 300

$ dig @127.0.0.1 -x 192.168.10.10 +short
ns.cloud.local.

$ dig @127.0.0.1 debian.org +short
151.101.2.132
151.101.194.132
```

Desde el host, en **cmd**:

```bat
nslookup ns.cloud.local 192.168.10.10
```

```text
Servidor:  ns.cloud.local
Address:  192.168.10.10

Nombre:  ns.cloud.local
Address:  192.168.10.10
```

```bat
nslookup 192.168.10.10 192.168.10.10
```

```text
Servidor:  ns.cloud.local
Address:  192.168.10.10

Nombre:  ns.cloud.local
Address:  192.168.10.10
```

```bat
nslookup debian.org 192.168.10.10
```

```text
Respuesta no autoritativa:
Servidor:  ns.cloud.local
Address:  192.168.10.10

Nombre:  debian.org
Addresses:  2a04:4e42:200::644
            2a04:4e42::644
            2a04:4e42:600::644
            2a04:4e42:400::644
            151.101.194.132
            151.101.2.132
            151.101.66.132
            151.101.130.132
```

Cómo leer la salida de `nslookup`:

- El bloque **`Servidor` / `Address`** de arriba es **quién respondió**, no la
  respuesta. Confirma que le preguntamos a `192.168.10.10`.
- El bloque **`Nombre` / `Address`** de abajo es **la respuesta**.
- **`Respuesta no autoritativa`** en `debian.org` es correcta: significa que `ns`
  no es dueño de ese dominio y la obtuvo por recursión. Para `cloud.local` no
  aparece porque `ns` sí es autoritativo.

Las tres pruebas cubren cosas distintas: la primera la zona directa, la segunda la
inversa (opcional en el plan) y la tercera la recursión. Si la tercera fallara sola,
el problema estaría en la salida a internet de `ns` (`enp0s8`), no en las zonas.

📸 `B-03` — `named-checkconf` + `named-checkzone`. 📸 `B-04` — las tres consultas.

### Decisiones

- **Los archivos de zona van en `/var/lib/bind/`, no en `/etc/bind/`.** `named`
  corre como usuario `bind` y necesita escribir el *journal* de las actualizaciones
  dinámicas. `/etc/bind` es de `root` y no es escribible por ese usuario (D6).
- **`update-policy local` en vez de una llave TSIG repartida a mano.** BIND genera
  una llave de sesión en `/run/named/session.key`; `nsupdate -l` la usa. Es lo que
  permite que B3 actualice sin gestionar secretos.
- **`forwarders` + `recursion`:** las instancias necesitan resolver `deb.debian.org`
  para `apt` a través de `ns`. Sin recursión, cada instancia tendría que apuntar a
  un DNS externo.
- **Serial `YYYYMMDDnn`.** Hoy `2026110702` en la zona directa y `2026110701` en la
  inversa. Cada zona tiene el suyo; no tienen que coincidir. En B3 no se toca a
  mano: `nsupdate` lo incrementa solo.
- **`/var/lib/bind` quedó `root:bind` con `drwxrwxr-x`**, así que `bind` puede crear
  el journal. Los `db.*` son `bind:bind` para que un volcado del journal no cambie
  de dueño.

### Problemas encontrados

Detalle completo en [`docs/problemas.md`](../problemas.md).

| Síntoma | Causa | Solución |
|---|---|---|
| `named-checkconf`: `unknown option 'update-privacy'` | El archivo se escribió a mano y `policy` quedó como `privacy` | Corregir a `update-policy` y volver a validar |
| `dig ns.cloud.local` devolvía `192.168.10.0` | El registro `A` se escribió con la dirección de red en vez de la del servidor | Corregir a `192.168.10.10` y subir el serial |
| `rndc reload cloud.local` falla con *dynamic zone* | BIND no recarga desde archivo una zona dinámica: descartaría el journal | `systemctl restart named` (no había journal, no se perdió nada) |

La lección del tercer punto es la que importa para B3: con zonas dinámicas el
archivo en disco y la memoria del demonio son **dos estados distintos**. Editar el
archivo no cambia lo que responde `named`. La vía manual correcta es
`rndc freeze` → editar → `rndc thaw`; la vía normal, y la que usará el producto, es
`nsupdate -l` contra el demonio vivo.

## B3. Actualización dinámica con `httpaas-dns` — ✅ 2026-11-09

`ns` deja de editarse a mano: un script recibe altas y bajas y las aplica contra el
demonio vivo con `nsupdate -l`. Es exactamente lo que la webapp llamará por SSH en B6.

### Archivos

| Archivo en el repo | Destino en `ns` | Dueño / modo | Qué hace |
|---|---|---|---|
| `httpaas-dns` | `/usr/local/sbin/httpaas-dns` | `root:root 755` | `agregar`, `quitar`, `listar` |
| `sudoers` | `/etc/sudoers.d/httpaas` | `root:root 440` | `NOPASSWD` **solo** para ese script |
| `instalar.sh` | (se ejecuta) | — | Converge B1.3–B3.2; idempotente |

### Lo que se hizo

1. Escribir `httpaas-dns` con `set -euo pipefail` y tres subcomandos.
2. Validar host (`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`) e IP (`192.168.10.1–254`) antes de
   ejecutar nada; rechazar `ns` y las IPs reservadas `.1`, `.10` y `.20`.
3. `agregar`: dos bloques de `nsupdate` (uno por zona, cada uno con su `send`) y
   confirmación con `dig`.
4. `quitar`: leer la IP actual, borrar el `A` y el `PTR`; no falla si no existía.
5. `listar`: `AXFR` filtrado a `host ip` (requiere `allow-transfer { localhost; };`).
6. `sudoers` validado con `visudo -cf` antes de instalarse.
7. `instalar.sh` aplica todo, incluida la resolución de nombres de B1.3.

### Verificación (salida real)

```text
$ ssh ... sudo httpaas-dns agregar prueba 192.168.10.99
added prueba.cloud.local -> 192.168.10.99                       # exit 0

$ dig @192.168.10.10 prueba.cloud.local +short
192.168.10.99

$ dig @192.168.10.10 -x 192.168.10.99 +short
prueba.cloud.local.

$ sudo httpaas-dns listar
ns 192.168.10.10
prueba 192.168.10.99

$ ssh ... sudo httpaas-dns agregar 'x;id' 192.168.10.99
httpaas-dns: invalid host: 'x;id'                               # exit 1

$ ssh ... sudo httpaas-dns agregar otra 192.168.10.10
httpaas-dns: ip '192.168.10.10' is reserved for the infrastructure  # exit 1

$ ssh ... sudo -n cat /etc/shadow
sudo: a password is required                                    # exit 1

$ ssh ... sudo -n -l
User httpaas may run the following commands on ns:
    (root) NOPASSWD: /usr/local/sbin/httpaas-dns                # exit 0

$ sudo httpaas-dns quitar prueba
removed prueba.cloud.local                                      # exit 0

$ dig @192.168.10.10 prueba.cloud.local +short
                                                                # vacío

$ sudo httpaas-dns quitar prueba
not found: prueba.cloud.local                                   # exit 0, idempotente

$ ls -l /var/lib/bind/
-rw-r--r-- 1 bind bind db.192.168.10
-rw-r--r-- 1 bind bind db.192.168.10.jnl
-rw-r--r-- 1 bind bind db.cloud.local
-rw-r--r-- 1 bind bind db.cloud.local.jnl
```

La presencia de los `.jnl` es la prueba de que la actualización fue **dinámica**
(RFC 2136), no una reescritura del archivo.

📸 `B-05` — alta + `dig`. 📸 `B-06` — rechazo del nombre malicioso y de `sudo cat`.
📸 `B-07` — los archivos `.jnl`.

### Decisiones

- **`nsupdate -l` en vez de una llave TSIG repartida.** `-l` usa la llave de sesión
  que `named` genera en `/run/named/session.key`, la contraparte de
  `update-policy local`. No hay secreto que distribuir.
- **Dos bloques con dos `send`.** Un mensaje DNS UPDATE pertenece a una sola zona;
  mezclar el `A` (`cloud.local`) y el `PTR` (`10.168.192.in-addr.arpa`) falla con
  `NOTZONE`.
- **`update delete` antes de `update add`.** Reagregar un host existente no falla.
- **Confirmar con `dig` dentro del script.** El código de salida significa "el
  registro está", no "nsupdate no se quejó". Sin eso la webapp reportaría éxito falso.
- **Rechazar `ns`.** Sobrescribir el registro del servidor DNS tumbaría la plataforma.
- **Rechazar también las IPs de infraestructura `.1`, `.10` y `.20`.** Escribir `.10`
  borraría el `PTR` `10 PTR ns.cloud.local.` de la zona inversa, y la confirmación con
  `dig` seguiría pasando porque el `A` sí queda: sería una falla silenciosa. El
  contrato está fijado en `internal/dns/httpaas_dns_test.go`.
- **Sudoers de mínimo privilegio.** `httpaas` puede ejecutar un solo binario como
  root. La cuenta tiene la contraseña bloqueada (`passwd -S` → `L`), así que cualquier
  otro comando falla sin siquiera poder autenticarse: `sudo -n cat /etc/shadow` →
  `sudo: a password is required`. La prueba positiva es `sudo -n -l`, que sin
  contraseña lista la única regla: `(root) NOPASSWD: /usr/local/sbin/httpaas-dns`.
- **`instalar.sh` no sobrescribe zonas existentes.** Después de B3 las zonas tienen
  registros dinámicos; pisarlas borraría las instancias. `--forzar-zonas` es la
  excepción deliberada.

### Problemas encontrados

Detalle completo en [`docs/problemas.md`](../problemas.md).

| Síntoma | Causa | Solución |
|---|---|---|
| `listar` devolvía `prueba.cloud.local.` en vez de `prueba` | El `sed` anclaba `$` al final de la línea, que termina en la IP, no en el sufijo | Separar el campo con `awk` y quitar el sufijo ahí |
| `agregar ns` decía *invalid host* en vez de *reserved* | El regex de longitud (mínimo 3) rechazaba `ns` antes de llegar al chequeo de reservado | Mover el chequeo de `ns` antes del regex |
| `apt` y `dig` sin `@servidor` fallaban | El DHCP entregó un DNS inalcanzable | `prepend` de `127.0.0.1, 10.0.3.3` en `dhclient.conf` (ver B1) |

## B4. Imagen `ns.ova` — ✅ verificada 2026-11-09

`ns` deja de vivir en un solo PC: se exporta como un archivo único que A y C
importan con un comando, sin repetir B1–B3 a mano.

### Lo que se hizo

1. Apagar la VM. `VBoxManage export` exige `poweroff`: no acepta una máquina
   encendida ni en estado guardado.
2. `VBoxManage export ns -o ns.ova`.
3. Validar el resultado **sin importarlo**: `VBoxManage import ns.ova --dry-run`.
4. Comprobar que git lo ignora (`.gitignore` → `*.ova`).
5. Mover el archivo fuera del repositorio y subirlo a la carpeta compartida.

### Verificación

```text
$ VBoxManage import ns.ova --dry-run
Interpreting ns.ova...
OK.
  vmdisk1  18874368000  ...  streamOptimized  ns-disk001.vmdk
  Suggested OS type: "Debian13_64"
  Number of CPUs: 2
  Guest memory: 1024 MB
  Network adapter: orig HostOnly, config 3, extra slot=0
  Network adapter: orig NAT, config 3, extra slot=1
```

| Comprobación | Resultado |
|---|---|
| Archivo | `ns.ova`, 1.14 GB |
| Disco virtual | 18 GB, `streamOptimized` (solo se empaqueta lo escrito) |
| Tipo de SO | `Debian13_64` |
| Recursos | 2 vCPU, 1024 MB |
| Redes | HostOnly + NAT |
| Ignorado por git | `.gitignore:14:*.ova` |

**Verificación externa:** Cristhian importó `ns.ova` en su PC con
`scripts/importar-ns.sh` (PR #4) y `scripts/verificar-ns.sh` pasó los chequeos
de B1–B3: Bind9 responde, la zona directa e inversa están bien y
`httpaas-dns` registra y quita sin tocar la zona a mano. B4 queda cerrada.

### Decisiones

- **`.ova` y no `.ovf`.** Un solo archivo se copia por Drive o USB sin perder
  piezas; `.ovf` deja el descriptor y los discos sueltos.
- **Fuera del repositorio.** `*.ova` está en `.gitignore` y el archivo pesa
  1.14 GB. Además un `git clean -xdf` lo borraría sin preguntar.
- **`--dry-run` antes de compartir.** Lee el descriptor y no crea ninguna VM:
  detecta un OVA corrupto antes de subirlo.
- **`--vmname ns` al importar.** Sin eso, si en la máquina ya existe una VM `ns`,
  VirtualBox sugiere `ns 1`.
- **El OVA no lleva la instantánea `ns-base`.** Exporta el estado actual, no el
  historial de snapshots.

### Problemas encontrados

Ninguno. El export se ejecutó desde la raíz del repositorio y el archivo quedó
ahí; git lo ignora, pero hay que sacarlo antes de cualquier `git clean -xdf`.

## B5. Resolución de `*.cloud.local` en el host — ✅ 2026-11-09

`ns` ya respondía `cloud.local`, pero el sistema operativo del host no le
preguntaba: usaba el DNS del ISP. Esta fase conecta los dos **sin romper
internet**, con DNS dividido: solo `*.cloud.local` va a `ns`.

### Lo que se hizo

1. **Windows:** regla NRPT para el sufijo `.cloud.local` apuntando a
   `192.168.10.10`.
2. **Linux:** `scripts/dns-host-linux.sh`, idempotente y con `--undo`.
   - Reubica `dns` **antes** de `mdns[46]_minimal` en `/etc/nsswitch.conf`
     conservando los demás módulos de la línea.
   - Con `systemd-resolved` activo: `resolvectl dns` + `resolvectl domain '~cloud.local'`.
   - Sin él: `nameserver 192.168.10.10` como primera línea de `/etc/resolv.conf`.
3. Documentar ambos procedimientos y cómo deshacerlos en
   [`dns-host.md`](dns-host.md).

### Verificación (salida real)

```text
PS> Get-DnsClientNrptRule | Format-List Namespace, NameServers, Comment
Namespace   : {.cloud.local}
NameServers : 192.168.10.10
Comment     : httpaas-b5

PS> Resolve-DnsName ns.cloud.local
Name             Type IPAddress
ns.cloud.local   A    192.168.10.10

PS> ping ns.cloud.local
Paquetes: enviados = 4, recibidos = 4, perdidos = 0 (0% perdidos)

PS> Invoke-WebRequest https://debian.org -Method Head
debian.org -> HTTP 200
```

| Comprobación | Resultado |
|---|---|
| Regla NRPT | `.cloud.local` → `192.168.10.10` |
| `Resolve-DnsName ns.cloud.local` | `192.168.10.10` |
| `ping ns.cloud.local` | 4/4, 0% perdidos |
| Internet | `debian.org` HTTP 200, resuelve a IP de Fastly |
| Registro dinámico de B3 | `prueba.cloud.local` → `192.168.10.99` |
| Nombre inexistente | `noexiste.cloud.local` → NXDOMAIN (correcto) |

### Decisiones

- **NRPT y no `hosts`.** `hosts` obliga a agregar cada VM a mano y no cubre
  instancias nuevas. NRPT resuelve todo el sufijo solo y se deshace con un comando.
- **DNS dividido, no cambiar el DNS global.** Solo `cloud.local` va a `ns`; el
  resto sigue por el ISP. Cambiar el DNS global dejaría internet dependiendo de la VM.
- **El script preserva módulos, no impone una línea.** La versión inicial
  reescribía la línea `hosts:` completa y perdía `mymachines`, `resolve` o
  `myhostname` en distros que los usan. Ahora solo **reubica** `dns`: la única
  regla es que exista y esté antes de `mdns[46]_minimal`.
- **`getent hosts` como verificación final del script.** Consulta por NSS, el
  mismo camino que usa el navegador. `dig` habla DNS directo y se salta
  `nsswitch.conf`, así que diría "funciona" con el sistema roto.

### Problemas encontrados

| Síntoma | Causa | Solución |
|---|---|---|
| `ping 192.168.10.10` daba 100% perdidos justo tras `startvm` | La VM aún estaba en el arranque EFI; tarda más que un BIOS | Esperar a que `named` esté activo antes de concluir que la red falla |
| `Resolve-DnsName 192.168.10.10` no devuelve el `PTR` | La regla NRPT cubre `.cloud.local`, no la zona inversa `10.168.192.in-addr.arpa` | No afecta al navegador; consultar con `-Server 192.168.10.10` |
| `nslookup ns.cloud.local` falla con la regla puesta | `nslookup` no usa NRPT | Probar con `Resolve-DnsName` o `ping` |
| `Resolve-DnsName ns.cloud.local -Server 8.8.8.8` devuelve el registro | NRPT se aplica antes que `-Server` cuando el nombre cae en el sufijo | Usar `nslookup` para consultar otro servidor de verdad |

## B6. Adaptador `internal/dns` — ✅ 2026-11-09

La webapp deja de hablar DNS a mano: usa `puertos.DNS`, que es el contrato que ya
consumía `internal/instancias`. El adaptador no sabe de HTTP ni de VirtualBox.

### Lo que se hizo

`internal/dns/dns.go`, cuatro piezas:

1. `Nuevo(ex executor.Executor, cfg config.Config) *DNS`. No conecta: el canal SSH
   y el servidor DNS solo se tocan cuando se llama a un método.
2. `Agregar` / `Quitar`: validan **antes** de construir el comando y ejecutan
   `sudo /usr/local/sbin/httpaas-dns <sub> <args>` con `executor.Command`, nunca
   con `sh -c`.
3. `Resolver`: `net.Resolver` con `PreferGo: true` y un `Dial` fijo a
   `192.168.10.10:53`, así no depende de NRPT ni de `nsswitch.conf`.
4. `ejecutar`: revisa `ExitCode`, porque el executor solo devuelve `error` cuando
   el comando **no se pudo entregar**.

### Verificación (salida real)

```text
$ go test -count=1 ./internal/dns/
ok  	github.com/lyc4nthrope/httpaas-paginas-web/internal/dns	5.480s

$ go test -tags vm -count=1 -v -run TestVMRegistrarResolverQuitar ./internal/dns/
=== RUN   TestVMRegistrarResolverQuitar
--- PASS: TestVMRegistrarResolverQuitar (2.90s)
```

La prueba `vm` hace `Agregar` → `Resolver` → `Quitar` contra el `ns` real y deja
la zona limpia:

```text
$ sudo httpaas-dns listar
ns 192.168.10.10
prueba 192.168.10.99
```

`gofmt -l .` limpio, `go vet ./...` limpio, `go build ./cmd/httpaas` ok. Los
cuatro fallos de `go test ./...` en Windows son los de siempre por entorno
(`almacen`, `executor`, `oplog`, `instancias.TestCrearTimeout`); CI corre en Linux.

### Decisiones

- **`Resolver` recibe el label, no el FQDN.** `internal/instancias:242` ya llama
  `Resolver(ctx, nombre)` con `paisajes`, no con `paisajes.cloud.local`. El
  adaptador agrega el dominio, y así nadie puede construir
  `paisajes.cloud.local.cloud.local`.
- **`Resolver` no aplica `dominio.ValidarHostname`.** B6.5 necesita resolver `ns`,
  que `ValidarHostname` rechaza por reservado. El nombre solo llega a
  `net.Resolver`, nunca a una línea de comandos, así que no hay superficie de
  inyección.
- **El `Dial` se fija en `Nuevo`, no en cada consulta.** Un solo lugar decide a
  quién se le pregunta; `servidor` se calcula una vez con `net.JoinHostPort`.
- **Un `exit != 0` es error con el `stderr`.** El mensaje útil lo escribe el
  script (`ip '192.168.10.10' is reserved`); perderlo obligaría a leer el log de
  la VM para entender un fallo.
- **`lookup` es un campo, no una llamada directa.** Permite probar el FQDN y el
  manejo de errores sin red; el camino real lo cubre la prueba `vm`.

### Problemas encontrados

| Síntoma | Causa | Solución |
|---|---|---|
| `make vm-test` se saltaba la prueba | No existe `config.json` (está en `.gitignore`) | Crear una config temporal y pasarla con `HTTPAAS_CONFIG` |
| El adaptador podía reportar éxito con un comando fallido | `executor.SSH.Run` solo devuelve `error` para fallos de transporte | Revisar `res.ExitCode` y devolver el `stderr` |

## B6.5. Validación de `ns` al arrancar — ✅ código y prueba VM; banner pendiente

Antes de crear instancias, la webapp comprueba que `ns` responde. Sin eso, el
primer `agregar` falla dentro de la cola y el usuario ve un error de DNS en vez
de "la VM DNS no está encendida".

### Lo que se hizo

`Comprobar(ctx) error` en `internal/dns/dns.go`. Resuelve `ns` con el mismo
`Resolver` del adaptador y compara con `cfg.DNS.IP`:

1. Si resuelve a la IP configurada, devuelve `nil`.
2. Si falla o resuelve a otra IP, devuelve `*errorComprobacion`: el mensaje es la
   receta para el usuario (`VBoxManage import ns.ova` o `deploy/ns/instalar.sh`)
   y la causa técnica queda en `Unwrap`, para `errors.Is`/`errors.As`.
3. No crea ni repara nada: solo diagnostica.

No se agregó a `puertos.DNS` a propósito: es un diagnóstico de arranque de este
adaptador, no algo que el orquestador necesite de todo DNS. El contrato de
`docs/PLAN.md` §4 queda intacto.

### Verificación (salida real)

```text
$ go test -count=1 ./internal/dns/
ok  	github.com/lyc4nthrope/httpaas-paginas-web/internal/dns

$ go test -tags vm -count=1 -v -run TestVMComprobar ./internal/dns/
=== RUN   TestVMComprobar
--- PASS: TestVMComprobar (0.05s)
```

Cuatro pruebas unitarias: `TestComprobarOK`, `TestComprobarServidorCaido`,
`TestComprobarIpEquivocada` y el caso de host vacío. El camino de error se
comprobó contra la VM real apuntando `dns.ip` a `192.168.10.250`: tarda 5.01 s
(el tiempo de espera del resolvedor) y devuelve la receta exacta.

### Pendiente

- **Banner en el dashboard (B-12).** Requiere un campo en `web.Vista`
  (`internal/web/web.go`, de A) y el cableado en `cmd/httpaas/main.go` (A7).
  Además `main.go` hoy rechaza el modo real (`"el modo real se conecta en la
  fase A7"`), así que la captura no se puede tomar hasta A7. Aparcado sin tocar
  archivos de A.

## B7. Dashboard — ✅ 2026-11-09

La pantalla que ve el usuario: un formulario para crear y publicar, y la tabla de
instancias con su estado. Part B reescribe la plantilla provisional de A5.

### Lo que se hizo

`internal/web/templates/dashboard.html`:

1. **Estados con color y texto.** `<span class="estado estado-{{.Estado}}">` con un
   punto de color y la etiqueta: verde `lista`, ámbar `creando`/`publicando`/
   `eliminando`, rojo `error`/`perdida`. No depende solo del color.
2. **Responsive a 360 px.** Bajo 640 px la tabla pasa a tarjetas: cada celda lleva
   `data-etiqueta` y `::before` muestra el nombre de la columna.
3. **Accesibilidad.** `:focus-visible` con contorno, `role="status"` en el aviso,
   `role="alert"` en los errores por campo, etiquetas asociadas con `for`, botones
   con texto.
4. **Resto del contrato de B7:** un solo formulario `multipart/form-data` con
   Aceptar/Publicar, enlace en pestaña nueva, IP/24, nombre de la VM, fecha,
   Eliminar con diálogo de confirmación (`<dialog>`, con `confirm()` de respaldo),
   estado vacío y mensajes flash.
5. **Rediseño visual:** descripción de qué hace la página, tira de tres pasos,
   tarjetas con sombra, píldoras de estado con color y texto, tabla con hover,
   contador de instancias, foco visible y selector de archivo propio (label +
   nombre del archivo) para que el texto nativo no ensanche la página. Los
   estilos siguen inline.
6. **Diálogo de eliminación:** `<dialog>` nativo con el nombre de la instancia y
   los botones Cancelar/Eliminar. Si el navegador no soporta `showModal()`, queda
   el `confirm()` del atributo `onsubmit` como respaldo.
7. **Publicar inteligente (mejora progresiva):** con JavaScript, el botón
   Publicar crea la instancia si no existe, espera a que esté `lista` sondeando
   `/api/instancias` y recién entonces sube el zip. Sin JavaScript, el formulario
   sigue funcionando con los dos botones del plan. La pestaña debe quedar abierta
   durante el proceso (~1 min en real); si se cierra, la instancia queda creada
   pero sin contenido. La versión robusta está planteada como M3 en
   `docs/problemas.md`.

### Verificación (modo simulado, binario real)

```text
GET /            -> 200  vacío=True  título=True
POST /instancias -> 200
refresco creando -> True
fila paisajes    -> nombre=True ip=True badge=estado-lista enlace=True target=True
api              -> {"Nombre":"paisajes","IP":"192.168.10.30",...,"Estado":"lista",...}
POST eliminar    -> 200
vacío otra vez   -> True
```

Con dos instancias (`paisajes` `.30`, `negocios` `.31`): 2 filas, 2 badges
`estado-lista`, 2 botones Eliminar, sin `meta refresh` cuando no hay trabajo y
con el flash `Instancia negocios en creación`.

`go test ./internal/web/` en verde; `gofmt`, `go vet` y `go build` limpios.

La vista se comprobó a **360 px reales** con el protocolo de DevTools
(`Emulation.setDeviceMetricsOverride`). El headless de Edge clampa la ventana a
unos 500 px, así que una captura con `--window-size=360` sale recortada y no
sirve para validar el layout móvil.

Publicar sin crear antes (solo nombre + zip), con el archivo real:

```text
secuencia de estados: creando -> publicando -> lista
final:                paisajes / lista
```

Con un nombre inválido (`Paisajes`) el aviso de validación aparece en la vista y
no se crea ninguna instancia.

### Decisiones

- **Estilos inline, no `static/estilos.css`.** `web.go` solo embebe
  `templates/*.html`; servir `static/` exigiría agregar `//go:embed static/*` y una
  ruta en un archivo de A. Para una hoja de estilos no vale la dependencia.
- **`<meta refresh>` en vez de `static/app.js`.** El plan lo dejaba como opcional.
  `GET /api/instancias` ya existe, pero el refresco completo es suficiente y evita
  otro archivo estático.
- **Publicar inteligente como mejora progresiva.** No cambia las rutas de A ni la
  figura: los dos botones siguen ahí y sin JavaScript todo funciona igual. La
  orquestación (crear → esperar `lista` → publicar) vive en el navegador porque el
  servidor no tiene una operación combinada; el cambio de servidor está en M3 de
  `docs/problemas.md`.

### Pendiente

- Las capturas B-09, B-10 y B-11 las toma el usuario en su documento aparte.

## B8. Documento final — 🟡 borrador 2026-11-09

`docs/DOCUMENTO.md` tiene la estructura completa, la Parte B desarrollada (B0–B7, con
problemas y decisiones) y los huecos marcados para A y C. Faltan sus secciones, las
capturas y el banner B-12 (A7). El checklist de entrega está al final del documento.
La prueba completa de punta a punta se ejecuta cuando A7 y C7 estén cerrados.
