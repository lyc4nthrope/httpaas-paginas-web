# Importar y verificar la VM `ns` (los tres integrantes)

Daniel exportó la VM `ns` (Bind9, Parte B) a `ns.ova`. Cada integrante la importa en su VirtualBox para probar la integración. Esta guía hace que funcione igual en los tres PCs.

## Por qué hace falta un paso extra

Un `.ova` guarda **el nombre de la red host-only del PC donde se exportó**. `ns.ova` se exportó desde Windows con `VirtualBox Host-Only Ethernet Adapter #2`. En otro PC esa red se llama distinto (`vboxnet0` en Linux, u otro número en Windows), así que la VM arrancaría **sin la red del proyecto** (`192.168.10.0/24`). El script la conecta a tu red.

## Antes de empezar

- La red host-only existe con el host en `192.168.10.1` y sin DHCP (plan general §7, P2).
- Tienes tu llave en `keys/httpaas_ed25519` (P3), y su `.pub` está instalado en `ns`. Las tres llaves de `deploy/llaves/` deben estar en `authorized_keys`.
- Hay **unos 4 GB libres**: el `.ova` pesa 1,2 GB y la VM importada unos 3 GB. Cuando `verificar-ns.sh` pase, se puede borrar el `.ova`.

## Linux

```bash
cd httpaas-paginas-web
scripts/importar-ns.sh ~/Descargas/ns.ova        # importa y conecta el adaptador 1 a vboxnet0
VBoxManage startvm ns --type headless
scripts/verificar-ns.sh                          # 15 verificaciones de B1–B3
```

## Windows (Git Bash)

1. Busca el nombre exacto de tu red host-only con `192.168.10.1`:
   ```bash
   VBoxManage list hostonlyifs
   ```
2. Importa pasando ese nombre entre comillas:
   ```bash
   scripts/importar-ns.sh /c/Users/<tu-usuario>/Downloads/ns.ova "VirtualBox Host-Only Ethernet Adapter #2"
   VBoxManage startvm ns --type headless
   scripts/verificar-ns.sh
   ```
   Si `VBoxManage` no está en el `PATH`: `export PATH="$PATH:/c/Program Files/Oracle/VirtualBox"`.

## Si ya la importaste a mano

Apágala y corre el script sin `.ova`. Solo corrige la red:
```bash
scripts/importar-ns.sh "" vboxnet0
```

## Qué comprueba `verificar-ns.sh`

| Fase | Comprobación |
|---|---|
| B1 | SSH con la llave como `httpaas` · contraseña desactivada · `sudo` permitido **solo** para `httpaas-dns` |
| B2 | `named` activo · `ns.cloud.local → 192.168.10.10` · zona inversa · resuelve internet |
| B3 | Agregar, listar y quitar un nombre de prueba · rechaza nombres maliciosos e IPs fuera de la red · quitar algo inexistente no es error · `sudo` para otra cosa es rechazado · la zona es dinámica (`.jnl`) |

Termina con `Resultado: N OK, M FALLA` y con un código distinto de 0 si algo falló. El nombre de prueba (`verificacion`, `.250`) se borra al final.

## Diferencias de `ns.ova` con el plan (no bloquean)

- 1024 MB y 2 CPUs; el plan dice 512 MB y 1 CPU. Funciona, pero usa más RAM.
- Firmware EFI; el plan dice BIOS. Solo importa en la plantilla, que se clona; `ns` no se clona.
- Los scripts para reconstruirla (`deploy/ns/`, decisión D14) todavía no están en el repositorio.
