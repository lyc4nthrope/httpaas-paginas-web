# Problemas encontrados

Anota aquí cada problema cuando te pase, no al final. Sirve para la sección "Problemas, mejoras y trabajo futuro" del documento.

| Fecha | Quién | Problema | Causa | Solución | Captura |
|---|---|---|---|---|---|
| 2026-10-29 | Cristhian | VirtualBox no deja crear la red host-only 192.168.10.0/24 en Linux | Sin `/etc/vbox/networks.conf`, solo se permite 192.168.56.0/21 | Paso P1 del plan | — |
| 2026-10-29 | Cristhian | `*.cloud.local` no resolvería en el host | `nsswitch.conf` manda `.local` a mDNS con `[NOTFOUND=return]` antes de `dns` | Fase B5 | — |
| 2026-11-07 | Daniel | La VM `ns` no arrancaba: *no bootable option or device* | El NVRAM de la VM estaba vacío; la entrada de arranque se pierde al crear una VM copiando el VDI en vez de usar `VBoxManage clonevm` | `grub-install --removable`, que crea `\EFI\BOOT\BOOTX64.EFI` | — |
| 2026-11-07 | Daniel | `grub-install: command not found` en la VM base | Solo estaba instalado el GRUB que arranca, no el que reinstala el cargador | `apt install grub-efi-amd64` | — |
| 2026-11-07 | Daniel | `apt` falla con *Temporary failure resolving* en `ns` | `enp0s8` (NAT) estaba `DOWN` y `resolv.conf` traía los DNS del ISP heredados del clon | `/etc/network/interfaces` con `enp0s3` estática sin gateway y `enp0s8` por DHCP | B-01 |

## Mejoras y trabajo futuro

| # | Mejora | Por qué | Quién |
|---|---|---|---|
| M1 | Gestión de llaves SSH desde la interfaz: pegar una llave pública o generar un par y descargar la privada una sola vez (como el key pair de AWS EC2) | Hoy la llave se genera en H0 (P3) y las VMs confían en ella al construirse (`authorized_keys` de `ns` y de la plantilla); cambiarla exige reconfigurar cada VM a mano | B (dashboard) + A (rutas) |
| M2 | Que la webapp detecte y aprovisione la VM `ns` si no existe (`VBoxManage import ns.ova`, B4) | Hoy `ns` es plataforma previa creada a mano (D14); el producto solo la consume y falla con un mensaje claro (B6.5) | B |
