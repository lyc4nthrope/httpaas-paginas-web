# Problemas encontrados

Anota aquí cada problema cuando te pase, no al final. Sirve para la sección "Problemas, mejoras y trabajo futuro" del documento.

| Fecha | Quién | Problema | Causa | Solución | Captura |
|---|---|---|---|---|---|
| 2026-10-29 | Cristhian | VirtualBox no deja crear la red host-only 192.168.10.0/24 en Linux | Sin `/etc/vbox/networks.conf`, solo se permite 192.168.56.0/21 | Paso P1 del plan | — |
| 2026-10-29 | Cristhian | `*.cloud.local` no resolvería en el host | `nsswitch.conf` manda `.local` a mDNS con `[NOTFOUND=return]` antes de `dns` | Fase B5 | — |
