# Dejar bien el disco de la plantilla (Parte C)

**Para:** Wiliam (y su asistente de IA) · **Base:** tu VM `apache-plantilla` y tu disco `ApacheServer.vdi`. **No hay que rehacer nada desde cero**: Apache, las llaves y las Guest Additions ya están bien. Solo falta instalar bien los scripts y **probar un clon antes de subir el disco**.

## Para el asistente de IA (copiar tal cual)

```text
Ayudas a Wiliam a dejar listo el disco ApacheServer.vdi de la plantilla del proyecto HTTPaaS
(Parcial 3). Repositorio httpaas-paginas-web, rama wiliam. Sigue docs/parte-c/ACTUALIZAR-PLANTILLA.md
paso por paso, en orden, sin saltar ninguno.

Reglas obligatorias:
- La base es la VM apache-plantilla existente: NO reinstales Debian, NO borres la VM, NO cambies
  Apache ni las Guest Additions.
- Solo se instalan los scripts con deploy/plantilla/instalar.sh. NUNCA copies scripts a mano con cp,
  NUNCA edites /usr/local/bin/httpaas-firstboot dentro de la VM, NUNCA uses `systemctl mask`.
- No cambies los nombres del contrato (docs/PLAN.md §4.3): /httpaas/hostname, /httpaas/ip,
  /httpaas/dominio, /httpaas/dns, /httpaas/estado (listo | error:...).
- Si cambias un script del repositorio, corre `bash -n` sobre él y `make check` antes del commit.
- El disco SOLO se sube si se cumplen las dos condiciones:
    1) dentro de la VM: verificar-plantilla.sh dice "23 OK, 0 FALLA";
    2) en el host: probar-clon.sh dice "El clon funciona".
  Si una falla, se corrige y se repite. No subas un disco "para probar".
- Antes de cada paso, di qué vas a hacer y por qué; después, muestra la salida de la comprobación.
```

---

## Por qué fallaron los tres discos anteriores

| Disco | Qué tenía | Por qué pasó |
|---|---|---|
| 1 (10 nov, mañana) | El primer arranque **viejo** (`/httpaas/domain`), y **del usuario `httpaas`** aunque lo ejecuta root | El script se corrigió en el repo, pero el disco se armó antes, copiando a mano |
| 2 (10 nov, 15:29) | Scripts de root ✅, pero **dos bugs**: una comilla de más (línea 120) y la IP sacada de la validación del DNS (todos los clones en `.192`) | El script nunca se ejecutó antes de armar el disco (corregidos en el PR #9) |
| 3 (10 nov, 16:46) | El script **vacío (0 bytes)** y el servicio **`masked`** | Se copió y se apagó a mano, y no se corrió `verificar-plantilla.sh` |

**La lección:** cada error recién se veía en otro PC, una hora después. Los pasos 4 y 5 los atrapan **en tu PC, antes de subir**.

---

## Paso 1. Poner el disco en modo normal y encender la plantilla

En el host, con la VM apagada:
```bash
VBoxManage showmediuminfo disk ApacheServer.vdi | grep Type
VBoxManage modifymedium disk ApacheServer.vdi --type normal      # solo si dice "multiattach"
VBoxManage startvm apache-plantilla
```
**Por qué:** en multiconexión el disco es de solo lectura para las VMs, y lo que instales se perdería.
**Comprobación:** la VM arranca y entras **como root** por la consola de VirtualBox.

## Paso 2. Traer los scripts corregidos

Dentro de la VM, como root:
```bash
cd /ruta/al/repo && git switch wiliam && git pull
bash -n deploy/plantilla/httpaas-firstboot && echo "sintaxis OK"
```
**Por qué:** los PRs #9 (los dos bugs) y #10 (`unmask` y esta guía) están en tu rama. Sin `git pull` instalarías otra vez lo viejo.
**Comprobación:** dice `sintaxis OK`, y `git log --oneline -3` muestra los merges de #9 y #10.

## Paso 3. Instalar con `instalar.sh` (nunca a mano)

```bash
sudo bash deploy/plantilla/instalar.sh
```
**Qué hace y por qué:**
- **Copia los scripts con dueño root** (`install -o root`). Un `cp` encima de un archivo existente deja el dueño anterior, y por eso en el disco 1 el script seguía siendo de `httpaas`.
- **Reactiva el servicio** (`systemctl unmask`) y lo habilita. Sin eso, en el disco 3 el primer arranque nunca se ejecutaba.
- **Deja SSH solo con llaves**, después de instalar las de `deploy/llaves/`, así nadie queda afuera.

**Comprobación:** termina con `completada con exito`. Si sale un error, se detiene ahí: corrígelo antes de seguir.

## Paso 4. Verificar dentro de la VM

```bash
sudo bash deploy/plantilla/verificar-plantilla.sh
md5sum /usr/local/bin/httpaas-firstboot deploy/plantilla/httpaas-firstboot
```
**Por qué:** son 23 comprobaciones de todo lo que falló antes: dueño root, sintaxis, que sea igual al del repo, servicio habilitado, SSH, `sudo`, y que la plantilla esté limpia. El `md5` confirma que el instalado **no está vacío** y es idéntico al del repo.
**Comprobación:** `23 OK, 0 FALLA`, y los dos `md5` **iguales**.

Las fallas más comunes:
- `/etc/httpaas/aplicado` existe: `sudo rm /etc/httpaas/aplicado`.
- Quedó un sitio de una prueba (por ejemplo `paisajes-web.conf`): `sudo a2dissite paisajes-web && sudo rm /etc/apache2/sites-available/paisajes-web.conf`.

Después: `sudo poweroff`.

## Paso 5. Probar un clon en tu PC (el paso que faltaba)

En el **host**, con la VM apagada:
```bash
deploy/plantilla/probar-clon.sh
```
**Qué hace y por qué:** hace **exactamente** lo que hará la app con cada clon. Toma un snapshot, escribe las propiedades (`prueba`, `192.168.10.29`), enciende la VM y comprueba:
- que el primer arranque llegue a `listo`;
- que tome la IP nueva;
- que SSH responda;
- que Apache sirva la página de `prueba.cloud.local`.

Al terminar, **siempre** apaga la VM, restaura el snapshot y borra las propiedades. **La plantilla queda exactamente como estaba.**
**Comprobación:** `El clon funciona: ya se puede subir el disco.` Si dice `NO subas el disco`, muestra qué falló y una pista. Corrige (pasos 2 a 4) y repítelo.

> Requisito: la red host-only con `192.168.10.1` (en Windows, el adaptador host-only de VirtualBox) para llegar a `.29`. Si `listo` sale OK y solo fallan la IP o SSH, revisa esa red.

## Paso 6. Subir el disco

Con la VM **apagada** (el paso 5 ya la deja así), sube `ApacheServer.vdi` al Drive y avisa con este texto: *"verificar-plantilla 23 OK y probar-clon OK"*.

---

## Paso 7. (Para quien lo recibe) Ponerlo en multiconexión

`VBoxManage modifymedium --type multiattach` **falla** con un disco suelto (*"can only be used on media registered with a machine"*). Primero hay que registrarlo en una VM:
```bash
mkdir -p "$HOME/VirtualBox VMs/httpaas-base" && mv ApacheServer.vdi "$HOME/VirtualBox VMs/httpaas-base/"
D="$HOME/VirtualBox VMs/httpaas-base/ApacheServer.vdi"
VBoxManage createvm --name apache-plantilla --ostype Debian_64 --register      # si no existe
VBoxManage storagectl apache-plantilla --name SATA --add sata --controller IntelAhci
VBoxManage storageattach apache-plantilla --storagectl SATA --port 0 --device 0 --type hdd --medium "$D"
VBoxManage storageattach apache-plantilla --storagectl SATA --port 0 --device 0 --medium none
VBoxManage modifymedium disk "$D" --type multiattach
```
`apache-plantilla` queda registrada **sin disco y sin encenderse nunca**: es la "dueña" del disco base. Si había un disco viejo registrado, primero `VBoxManage closemedium disk <ruta>` (sin `--delete`).
