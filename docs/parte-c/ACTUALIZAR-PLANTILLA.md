# Actualizar el disco de la plantilla (Parte C)

**Para:** Wiliam · **Por qué:** el `ApacheServer.vdi` del Drive (10 de noviembre) tiene el `httpaas-firstboot` del 7 de noviembre. Ese script lee `/httpaas/dominio` como `/httpaas/domain`, así que todas las instancias quedan en `error:datos-invalidos`. Además, el script es de `httpaas` (775) y root lo ejecuta en cada arranque: cualquiera con esa llave podría volverse root. Los scripts del repositorio ya están corregidos; falta **instalarlos en el disco**.

Toma unos 15 minutos. Cada paso tiene su comprobación.

## 1. Arrancar la plantilla y traer el repositorio

En la VM `apache-plantilla` (la de tu PC, con el disco en modo **normal**, no multiconexión), como root:
```bash
cd /ruta/al/repo && git fetch && git switch wiliam && git pull
```
**Comprobación:** `ls deploy/plantilla/` muestra `instalar.sh` y `verificar-plantilla.sh`.

Si el disco ya está en multiconexión en tu VirtualBox, primero vuélvelo normal, con la VM apagada y sin clones:
```bash
VBoxManage modifymedium disk ApacheServer.vdi --type normal
```

## 2. Instalar los scripts corregidos

```bash
sudo bash deploy/plantilla/instalar.sh
```
**Comprobación:** termina con `Instalacion de la plantilla HTTPaaS completada con exito.` Si muestra un error, no sigas: corrígelo primero.

> **Ojo:** este `instalar.sh` deja SSH **solo con llaves** (`PasswordAuthentication no` y `PermitRootLogin no`, como pide C1.5), e instala las llaves de `deploy/llaves/`. Si entras por SSH con contraseña, desde ahora usa tu llave, o la consola de VirtualBox. Las llaves de Daniel están en su rama: haz `git merge origin/dev` antes del paso 2 si las quieres incluir.

## 3. Verificar antes de apagar

```bash
sudo bash deploy/plantilla/verificar-plantilla.sh
```
**Comprobación:** `Resultado: 21 OK, 0 FALLA`. Revisa, entre otras cosas:
- Los dos scripts son de root, nadie más puede modificarlos, y son iguales a los del repo.
- El primer arranque lee `/httpaas/dominio`.
- La unidad está habilitada y espera a `vboxadd-service`.
- SSH sin contraseña y sin root, y `sudo` limitado a `httpaas-publicar`.
- La plantilla está limpia: no existe `/etc/httpaas/aplicado` y el hostname es `apache-plantilla`.

Si algo da FALLA, la línea dice qué es. Si `/etc/httpaas/aplicado` existe (C5.2), bórralo con `sudo rm /etc/httpaas/aplicado`.

## 4. Apagar y subir el disco

```bash
sudo poweroff
```
Sube `ApacheServer.vdi` al Drive y avisa al equipo.

## 5. (Para quien lo recibe) Ponerlo en multiconexión

`VBoxManage modifymedium --type multiattach` **falla** con un disco suelto: *"can only be used on media registered with a machine"*. El disco tiene que estar en el registro de una VM:
```bash
mkdir -p "$HOME/VirtualBox VMs/httpaas-base" && mv ApacheServer.vdi "$HOME/VirtualBox VMs/httpaas-base/"
D="$HOME/VirtualBox VMs/httpaas-base/ApacheServer.vdi"
VBoxManage createvm --name apache-plantilla --ostype Debian_64 --register
VBoxManage storagectl apache-plantilla --name SATA --add sata --controller IntelAhci
VBoxManage storageattach apache-plantilla --storagectl SATA --port 0 --device 0 --type hdd --medium "$D"
VBoxManage storageattach apache-plantilla --storagectl SATA --port 0 --device 0 --medium none
VBoxManage modifymedium disk "$D" --type multiattach
```
`apache-plantilla` queda registrada **sin disco y sin encenderse nunca**: es la "dueña" del disco base. Si ya existe un disco viejo con el mismo UUID, primero `VBoxManage closemedium disk <ruta>` (sin `--delete`).
