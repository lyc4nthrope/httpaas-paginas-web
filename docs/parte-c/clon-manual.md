# Procedimiento de Clonación Manual para Instancias HTTPaaS

Este documento especifica los comandos VBoxManage requeridos para clonar, aprovisionar y destruir una instancia web basada en el disco multiattach de httpaas-base.
# Procedimiento de Clonación Manual para Instancias HTTPaaS

Este documento especifica los comandos VBoxManage requeridos para clonar, aprovisionar y destruir una instancia web basada en el disco multiattach de httpaas-base.

## Variables de Entorno (Ejemplo)
- NAME: Nombre de la VM (ej. httpaas-paisajes)
- IP: IP asignada (ej. 192.168.10.30)
- HOSTNAME: Nombre de host (ej. paisajes-web)
- DOMAIN: Dominio asignado (ej. paisajes.cloud.local)
- DNS: Servidor DNS (ej. 192.168.10.10)
- HOSTONLY_NET: Nombre de la red Host-Only (ej. "vboxnet0" o "VirtualBox Host-Only Ethernet Adapter")
- BASE_VDI: Ruta al disco multiattach ("$HOME/VirtualBox VMs/httpaas-base/ApacheServer.vdi")

---

## Secuencia de Comandos VBoxManage

### 1. Registrar VM vacía
VBoxManage createvm --name "<NAME>" --ostype Debian_64 --register

### 2. Configurar Recursos y Red (Solo-Anfitrión)
VBoxManage modifyvm "<NAME>" --memory 512 --cpus 1 --nic1 hostonly --hostonlyadapter1 "<HOSTONLY_NET>"

### 3. Crear Controlador de Almacenamiento
VBoxManage storagectl "<NAME>" --name "SATA" --add sata --controller IntelAhci

### 4. Conectar Disco Multiattach (Crea Diferencial Automático)
VBoxManage storageattach "<NAME>" --storagectl "SATA" --port 0 --device 0 --type hdd --medium "<BASE_VDI>"

### 5. Configurar Guest Properties (Parámetros para httpaas-firstboot)
VBoxManage guestproperty set "<NAME>" /httpaas/hostname "<HOSTNAME>"
VBoxManage guestproperty set "<NAME>" /httpaas/ip "<IP>"
VBoxManage guestproperty set "<NAME>" /httpaas/dominio "<DOMAIN>"
VBoxManage guestproperty set "<NAME>" /httpaas/dns "<DNS>"

### 6. Encender Instancia Headless
VBoxManage startvm "<NAME>" --type headless

### 7. Esperar Estado "listo"
VBoxManage guestproperty get "<NAME>" /httpaas/estado

### 8. Apagar y Destruir Instancia (Sin afectar el disco base multiattach)
VBoxManage controlvm "<NAME>" poweroff
VBoxManage unregistervm "<NAME>" --delete
## Antes de clonar: el disco base en multiconexión

`VBoxManage modifymedium disk ApacheServer.vdi --type multiattach` falla si el disco está suelto (*"can only be used on media registered with a machine"*). Primero hay que registrarlo en una VM: se conecta a `apache-plantilla`, se desconecta, y entonces sí se cambia el tipo. Los comandos están en [ACTUALIZAR-PLANTILLA.md](ACTUALIZAR-PLANTILLA.md), paso 5. Comprobado el 2026-11-10: con el disco así, cada clon recibe su propio diferencial, y `unregistervm --delete` borra solo ese diferencial; el disco base queda intacto.
