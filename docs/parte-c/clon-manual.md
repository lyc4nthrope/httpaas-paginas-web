# Procedimiento de Clonación Manual para Instancias HTTPaaS

Este documento especifica los comandos VBoxManage requeridos para clonar, aprovisionar y destruir una instancia web basada en el disco multiattach de httpaas-base.

## Variables de Entorno (Ejemplo)
- NAME: Nombre de la VM (ej. httpaas-paisajes)
- IP: IP asignada (ej. 192.168.10.30)
- HOSTNAME: Nombre de host (ej. paisajes-web)
- DOMAIN: Dominio asignado (ej. paisajes.cloud.local)
- DNS: Servidor DNS (ej. 192.168.10.1)
- BRIDGE: Nombre de la interfaz de red Host (ej. "VirtualBox Host-Only Ethernet Adapter")
- BASE_VDI: Ruta al disco multiattach ("$HOME/VirtualBox VMs/httpaas-base/ApacheServer.vdi")

---

## Secuencia de Comandos VBoxManage

### 1. Registrar VM vacía
VBoxManage createvm --name "<NAME>" --ostype Debian_64 --register

### 2. Configurar Recursos y Red
VBoxManage modifyvm "<NAME>" --memory 1024 --cpus 1 --nic1 bridged --bridgeadapter1 "<BRIDGE>"

### 3. Crear Controlador de Almacenamiento
VBoxManage storagectl "<NAME>" --name "SATA" --add sata --controller IntelAhci

### 4. Conectar Disco Multiattach (Crea Diferencial Automático)
VBoxManage storageattach "<NAME>" --storagectl "SATA" --port 0 --device 0 --type hdd --medium "<BASE_VDI>"

### 5. Configurar Guest Properties (Parámetros para httpaas-firstboot)
VBoxManage guestproperty set "<NAME>" /httpaas/hostname "<HOSTNAME>"
VBoxManage guestproperty set "<NAME>" /httpaas/ip "<IP>"
VBoxManage guestproperty set "<NAME>" /httpaas/domain "<DOMAIN>"
VBoxManage guestproperty set "<NAME>" /httpaas/dns "<DNS>"

### 6. Encender Instancia Headless
VBoxManage startvm "<NAME>" --type headless

### 7. Esperar Estado "listo"
VBoxManage guestproperty get "<NAME>" /httpaas/estado

### 8. Apagar y Destruir Instancia (Sin afectar el disco base multiattach)
VBoxManage controlvm "<NAME>" poweroff
VBoxManage unregistervm "<NAME>" --delete