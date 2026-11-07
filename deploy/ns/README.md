# VM `ns` — servidor DNS de la plataforma

`ns` resuelve `*.cloud.local` y es la VM a la que la webapp le registra y le quita
las instancias por SSH. Es **plataforma previa**: se monta una vez a mano y se
conserva como imagen base (decisión D14). La webapp no la crea; si no responde,
falla con un mensaje claro y una receta de recuperación (fase B6.5). Aprovisionarla
desde una OVA queda como trabajo futuro (M2 en [`docs/problemas.md`](../../docs/problemas.md)).

## Estado actual

| Dato | Valor |
|---|---|
| Nombre en VirtualBox | `ns` |
| Sistema | Debian 13 (trixie) sin entorno gráfico, con `openssh-server` y utilidades estándar |
| Firmware | EFI (ver desviaciones) |
| CPU / RAM / disco | 2 vCPU / 1024 MB / 18 GB |
| `nic1` | Host-only sobre `VirtualBox Host-Only Ethernet Adapter #2` (`192.168.10.1/24`, sin DHCP) |
| `nic2` | NAT (salida a internet para `apt`) |
| IP del proyecto | `192.168.10.10/24`, **sin gateway** |
| Hostname | `ns`, FQDN `ns.cloud.local` |
| Usuario de servicio | `httpaas`, sin contraseña, acceso solo por llave |
| Instantánea base | `ns-base` |

## Configuración aplicada en B1

### Red — `/etc/network/interfaces`

```text
auto lo
iface lo inet loopback

# nic1 - host-only: the project network, fixed address, no gateway
auto enp0s3
iface enp0s3 inet static
    address 192.168.10.10/24

# nic2 - NAT: internet access for apt
auto enp0s8
iface enp0s8 inet dhcp
```

`enp0s3` no lleva `gateway` a propósito: la ruta por defecto tiene que venir de
`enp0s8` (NAT). Si se pone un gateway en la red host-only, todo el tráfico a
internet muere porque esa red no tiene router.

### Hostname — `/etc/hostname` y `/etc/hosts`

```bash
hostnamectl set-hostname ns
```

Y en `/etc/hosts`:

```text
127.0.1.1 ns.cloud.local ns
```

Sin esa línea, programas que resuelven el nombre de la propia máquina fallan
(`sudo: unable to resolve host ns`).

### Paquetes

```bash
apt install bind9 bind9-dnsutils sudo
```

`bind9-dnsutils` trae `dig`, `nslookup` y `nsupdate`, que usan B2 y B3.

### Usuario de servicio

```bash
adduser --disabled-password --gecos "" httpaas
```

Las llaves públicas de `deploy/llaves/*.pub` van en `~httpaas/.ssh/authorized_keys`,
con `.ssh` en `700` y `authorized_keys` en `600`, dueño `httpaas`. `sshd` rechaza
la autenticación si el archivo o sus directorios padres son escribibles por grupo
u otros (`StrictModes`).

### sshd — `/etc/ssh/sshd_config.d/10-httpaas.conf`

```text
PasswordAuthentication no
PermitRootLogin no
```

Se aplica **después** de comprobar que la llave entra: si se corta la contraseña
primero y la llave está mal, solo queda la consola de VirtualBox.
Validar siempre con `sshd -t` antes de `systemctl restart ssh`.

## Verificación

Desde el host:

```powershell
ssh -i keys/httpaas_ed25519 httpaas@192.168.10.10 hostname   # -> ns
```

Desde `ns`:

```bash
ping -c1 deb.debian.org
```

Capturas: `docs/capturas/B-01.png` (`ip -br addr`) y `docs/capturas/B-02.png`
(SSH sin contraseña).

## Desviaciones conscientes respecto de `docs/parte-b/PLAN.md`

| Plan (B1 paso 1) | VM real | Motivo |
|---|---|---|
| 1 CPU | 2 vCPU | La VM base ya estaba creada así; no afecta a `ns` |
| 512 MB | 1024 MB | Igual: holgura para `apt` y `named` |
| disco de 8 GB | 18 GB | Disco heredado de la imagen base |
| BIOS | EFI | El disco se instaló en EFI; pasarlo a BIOS obligaría a reinstalar el cargador. La restricción de BIOS del proyecto aplica al disco multiconexión (plantilla e instancias), no a `ns` |

## Cómo se reconstruye

1. Crear la VM con `nic1` host-only (el adaptador con `192.168.10.1`) y `nic2` NAT.
2. Instalar Debian 13 sin entorno gráfico, con `openssh-server` y utilidades estándar.
3. Aplicar red, hostname, paquetes, usuario y `sshd` como se describe arriba.
4. Verificar y tomar la instantánea `ns-base` con la VM **apagada**.

**Si la VM se crea copiando el disco a una VM nueva**, VirtualBox le genera un
NVRAM en blanco y no arranca aunque el disco esté bien. Dentro del guest, una vez:

```bash
sudo grub-install --removable
```

Eso crea `\EFI\BOOT\BOOTX64.EFI`, la ruta que el firmware prueba por defecto sin
necesitar ninguna entrada en el NVRAM. Es también lo que hace que los clones de la
plantilla arranquen solos.

## Archivos que viven aquí

| Archivo | Fase | Qué es |
|---|---|---|
| `README.md` | B1 | Este documento |
| `named.conf.options` | B2 | Opciones globales de Bind9 |
| `named.conf.local` | B2 | Declaración de las zonas |
| `db.cloud.local` | B2 | Zona directa `cloud.local` |
| `db.192.168.10` | B2 | Zona inversa `10.168.192.in-addr.arpa` |
| `httpaas-dns` | B3 | Registra y quita hosts con `nsupdate` |
| `sudoers` | B3 | Regla `NOPASSWD` solo para `httpaas-dns` |
| `instalar.sh` | B3 | Converge `ns` al estado versionado; idempotente |
