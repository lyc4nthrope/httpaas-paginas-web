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

## B3. Actualización dinámica con `httpaas-dns` — ⬜ pendiente

## B4. Imagen `ns.ova` — ⬜ pendiente

## B5. Resolución de `*.cloud.local` en el host — ⬜ pendiente

## B6. Adaptador `internal/dns` — ⬜ pendiente

## B6.5. Validación de `ns` al arrancar — ⬜ pendiente

## B7. Dashboard — ⬜ pendiente

## B8. Prueba de punta a punta y documento — ⬜ pendiente
