# Problemas encontrados

Anota aquí cada problema cuando te pase, no al final. Sirve para la sección "Problemas, mejoras y trabajo futuro" del documento.

| Fecha | Quién | Problema | Causa | Solución | Captura |
|---|---|---|---|---|---|
| 2026-10-29 | Cristhian | VirtualBox no deja crear la red host-only 192.168.10.0/24 en Linux | Sin `/etc/vbox/networks.conf`, solo se permite 192.168.56.0/21 | Paso P1 del plan | — |
| 2026-10-29 | Cristhian | `*.cloud.local` no resolvería en el host | `nsswitch.conf` manda `.local` a mDNS con `[NOTFOUND=return]` antes de `dns` | Fase B5 | — |

## Mejoras y trabajo futuro

| # | Mejora | Por qué | Quién |
|---|---|---|---|
| M1 | Gestión de llaves SSH desde la interfaz: pegar una llave pública o generar un par y descargar la privada una sola vez (como el key pair de AWS EC2) | Hoy la llave se genera en H0 (P3) y las VMs confían en ella al construirse (`authorized_keys` de `ns` y de la plantilla); cambiarla exige reconfigurar cada VM a mano | B (dashboard) + A (rutas) |
| M2 | Que la webapp detecte y aprovisione la VM `ns` si no existe (`VBoxManage import ns.ova`, B4) | Hoy `ns` es plataforma previa creada a mano (D14); el producto solo la consume y falla con un mensaje claro (B6.5) | B |
