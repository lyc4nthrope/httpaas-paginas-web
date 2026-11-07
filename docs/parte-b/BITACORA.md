# Bitácora de la parte B

Registro de lo ejecutado en la parte B (DNS y experiencia de usuario), fase por fase.
El plan vive en [`PLAN.md`](PLAN.md), los problemas del proyecto se anotan en
[`docs/problemas.md`](../problemas.md) y las capturas en `docs/capturas/<código>.png`.

Cada fase se marca ✅ cuando su sección **Verificación** del plan pasa contra la VM real.

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

```powershell
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

## B2. Zona `cloud.local` estática — ⬜ pendiente

## B3. Actualización dinámica con `httpaas-dns` — ⬜ pendiente

## B4. Imagen `ns.ova` — ⬜ pendiente

## B5. Resolución de `*.cloud.local` en el host — ⬜ pendiente

## B6. Adaptador `internal/dns` — ⬜ pendiente

## B6.5. Validación de `ns` al arrancar — ⬜ pendiente

## B7. Dashboard — ⬜ pendiente

## B8. Prueba de punta a punta y documento — ⬜ pendiente
