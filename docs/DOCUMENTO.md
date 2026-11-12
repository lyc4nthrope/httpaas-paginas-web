# HTTPaaS: servicio administrado de páginas y aplicaciones web

**Curso:** Computación en la Nube 2026-2 · Parcial 3 «HTTP como servicio»
**Profesores:** Carlos Eduardo Gómez Montoya y Juan Sebastián Salazar Osorio
**Equipo:** Cristhian Osorio, Daniel Pérez y Wiliam Carmona
**Fecha de entrega:** 11/11/2026
**Propósito de este documento:** explicar qué se construyó, cómo se construyó, con qué herramientas y para qué sirve cada parte, de modo que un lector sin contexto entienda el sistema, el proceso y la evidencia con la que se comprobó su funcionamiento.

> **Versión del repositorio.** Este texto es la versión de trabajo previa a la entrega. El documento entregado, que incluye las capturas C-01 a C-11, se envía junto con el repositorio y no se copia aquí.

## Tabla de contenido

1. Introducción
2. Requisitos del enunciado y cómo se cumplen
3. Arquitectura
4. Tecnologías y herramientas
5. El contrato común: `internal/puertos`
6. Parte A — Plataforma y orquestación
7. Parte B — DNS y experiencia de usuario
8. Parte C — Plantilla Apache y publicación
9. Prueba de punta a punta
10. Problemas encontrados
11. Conocimientos aprendidos
12. Uso de IA
13. Mejoras y trabajo futuro
14. Referencias
15. Anexo A: inventario de imágenes
16. Glosario

---

## 1. Introducción

### 1.1 Qué problema resuelve HTTPaaS

Publicar una página web suele exigir instalar un servidor, configurar su red, darle un nombre y copiar los archivos. HTTPaaS automatiza ese trabajo en un solo PC. El usuario escribe un nombre de host, por ejemplo `paisajes`, y sube un archivo `.zip` con su sitio. El sistema crea una máquina virtual con Apache, le asigna una IP, registra el nombre en un servidor DNS y publica el contenido en `http://paisajes.cloud.local`. Esto ocurre sin que el usuario toque una terminal.

### 1.2 Qué significa «HTTP como servicio»

En los servicios en la nube el usuario pide un recurso y la plataforma lo aprovisiona sola. Aquí el recurso es un servidor HTTP. El usuario no instala ni configura Apache: pide una instancia, recibe un enlace y puede eliminarla cuando termina. El proyecto reproduce a pequeña escala el flujo de una plataforma de servicios (IaaS y PaaS, según el enunciado) con Oracle VirtualBox, Apache y Bind9.

### 1.3 Objetivos

**Objetivo general.** Construir una aplicación en Go que, sobre VirtualBox y Bind9, permita crear, publicar y eliminar instancias Apache de forma reproducible, segura y observable desde un panel de control (*dashboard*).

**Objetivos específicos.**

1. Crear cada instancia a partir de un disco base multiconexión, sin copiar el disco completo, y configurarla sola en el primer arranque.
2. Registrar y quitar nombres `*.cloud.local` en un servidor DNS propio.
3. Publicar un `.zip` validado en dos capas: en el host y dentro de la VM.
4. Eliminar la instancia sin tocar el disco base y liberando su IP.
5. Mostrar el estado de cada instancia en un dashboard y permitir operarla desde allí.

**Alcance.** Hay un máximo de tres instancias simultáneas, configurable. La red es host-only `192.168.10.0/24`, y cada VM tiene además un adaptador NAT para `apt`. No hay autenticación de usuarios: la aplicación escucha en `127.0.0.1` y solo la usa el host.

### 1.4 Por qué el trabajo se dividió en tres partes

| Parte | Tema | Qué entrega |
|---|---|---|
| A | Plataforma y orquestación | Aplicación web en Go, configuración, ejecución remota por SSH, almacén de estado, orquestador con cola, rutas HTTP, adaptador de VirtualBox e integración |
| B | DNS y experiencia de usuario | VM `ns` con Bind9, script `httpaas-dns`, resolución de `*.cloud.local` en el host, adaptador DNS en Go y dashboard |
| C | Plantilla Apache y publicación | VM `apache-plantilla`, servicio de primer arranque, script de publicación, disco multiconexión, procedimiento de clonación, adaptador de publicación y diagrama |

Las tres partes se tocan en un solo punto, el contrato común (sección 5). Con él se acordaron las interfaces antes de repartir el trabajo, y por eso cada parte avanzó sin esperar a las otras.

---

## 2. Requisitos del enunciado y cómo se cumplen

| Requisito del enunciado | Cómo se resolvió | Dónde se evidencia |
|---|---|---|
| **1. Infraestructura virtualizada:** arquitectura con VirtualBox, Apache y Bind9, red local, dominio y zona DNS | Red host-only `192.168.10.0/24`, dominio `cloud.local`, VM `ns` autoritativa, instancias Apache sobre un disco multiconexión | Sección 3; P-01; diagrama `docs/Arquitectura.png` |
| **2. Bind9 en una VM dedicada** con zona local | VM `ns` (`192.168.10.10`) con la zona `cloud.local` y su zona inversa | Sección 7; B-01 a B-04 |
| **2. Registro dinámico de hosts** | `update-policy local` y `nsupdate -l` dentro de `ns`, a través del script `httpaas-dns` | Sección 7; B-05 a B-07 |
| **2. Resolución correcta hacia las IP de las VM** | La webapp comprueba que el nombre resuelve a la IP asignada antes de marcar la instancia como `lista` | Secciones 6.5 y 9; A-12 |
| **3. Debian 13 CLI con Apache**, acceso remoto, hostname único, directorio propio y sitio desde un `.zip` | VM `apache-plantilla`, servicio `httpaas-firstboot` y script `httpaas-publicar` | Sección 8; C-01 a C-08 |
| **3. Disco multiconexión como plantilla** | `ApacheServer.vdi` en modo `multiattach`; cada instancia recibe un disco diferencial | Sección 8.6; C-09 a C-11; A-07 |
| **4. Webapp en Go:** recibir solicitudes, ejecutar comandos remotos, actualizar DNS, desplegar contenido y mostrar un dashboard | Orquestador con cola, adaptadores `vbox`, `dns` y `publicar`, y un dashboard con enlaces en pestaña nueva | Secciones 6 y 7.7; A-04 a A-10; B-09 a B-12 |
| **5. Automatización sin interacción del usuario** | SSH con `BatchMode=yes`, guest properties y scripts de primer arranque | Secciones 6.2, 8.3 y 9 |
| **Eliminar instancias** (objetivo específico y botón de la figura) | Flujo «Eliminar»: DNS, VM, llave SSH y estado, sin tocar el disco base | Sección 3.4; A-08; A-16 |
| **Entregable 1:** código fuente | Repositorio público con `README.md` que explica cómo compilar, configurar y usar | https://github.com/lyc4nthrope/httpaas-paginas-web |
| **Entregable 2:** documento con capturas, problemas, mejoras y aprendizajes | Este documento | Secciones 10, 11 y 13; Anexo A |

---

## 3. Arquitectura

### 3.1 Conceptos previos de infraestructura

> **Concepto clave — Máquina virtual (VM).** Es un computador completo simulado por software que corre dentro de otro. Se parece a un apartamento amueblado dentro de un edificio: tiene sus propios servicios, pero comparte la estructura con los demás. En este proyecto cada instancia Apache y el servidor DNS son VMs.

> **Concepto clave — Hipervisor.** Es el programa que crea y ejecuta las VMs. Funciona como el administrador del edificio: asigna recursos a cada apartamento y los mantiene separados. Aquí lo es Oracle VirtualBox, y la webapp lo controla con el comando `VBoxManage`.

> **Concepto clave — Disco multiconexión (*multiattach*).** Es un disco virtual que varias VMs pueden usar a la vez sin modificarlo. Se parece a un libro de texto compartido: todos lo leen y cada uno escribe sus apuntes en un cuaderno aparte. Importa porque permite crear instancias nuevas sin copiar el disco completo.

> **Concepto clave — Disco diferencial.** Es el «cuaderno» de cada VM: guarda solo lo que esa VM cambió respecto al disco base. Empieza casi vacío y crece con lo que la VM escribe. Al eliminar una instancia se borra su diferencial y el disco base queda intacto.

> **Concepto clave — Red host-only.** Es una red privada entre el PC anfitrión (el *host*) y sus VMs, sin salida a internet. Se parece a un pasillo interno que comunica las oficinas de un edificio sin dar a la calle. Aquí da IP fijas y predecibles que no dependen del wifi.

> **Concepto clave — NAT.** Es un adaptador que deja a la VM salir a internet usando la conexión del host. Se parece a pedir cosas por la portería: la VM puede pedir hacia afuera, pero nadie de afuera llega a ella. Aquí se usa solo para `apt`.

> **Concepto clave — Zona DNS, registro A y registro PTR.** El DNS traduce nombres a direcciones IP, como una guía telefónica. Una *zona* es la parte de esa guía de la que un servidor es dueño, aquí `cloud.local`. Un registro **A** asocia un nombre con una IP (`paisajes → 192.168.10.30`). Un registro **PTR** hace lo inverso: dada la IP, devuelve el nombre.

### 3.2 Componentes y plan de direcciones

| Componente | Descripción |
|---|---|
| **Webapp Go** | Escucha en `127.0.0.1:8080`. Valida, encola trabajos y sirve el dashboard |
| **VirtualBox** | Crea, arranca y elimina las VMs; el disco base es multiconexión |
| **VM `ns`** | Bind9 autoritativo de `cloud.local` y de su zona inversa |
| **VM `apache-plantilla`** | Solo existe durante la preparación; contiene el disco base `ApacheServer.vdi` con Apache, el servicio de primer arranque y `httpaas-publicar` |
| **Instancias** | Clones con disco diferencial sobre el base |

| IP | Quién |
|---|---|
| `192.168.10.1` | Host, interfaz host-only |
| `192.168.10.10` | VM `ns` (Bind9) |
| `192.168.10.20` | VM `apache-plantilla` (solo durante la preparación) |
| `192.168.10.30` a `.99` | Instancias Apache, asignadas por la webapp |

Todas las VMs tienen `nic1` host-only (sin DHCP) y `nic2` NAT. La comunicación del proyecto va por host-only y el NAT se usa solo para `apt`.

### 3.3 Preparación de la red del host

En Linux, VirtualBox 7 solo permite crear redes host-only dentro de `192.168.56.0/21` hasta configurar `/etc/vbox/networks.conf`. En Windows no hace falta.

```bash
sudo mkdir -p /etc/vbox
echo '* 192.168.10.0/24 192.168.56.0/21' | sudo tee /etc/vbox/networks.conf
```

Crea el archivo de rangos permitidos y escribe la línea que autoriza `192.168.10.0/24`. Se espera que `cat /etc/vbox/networks.conf` muestre esa línea.

```bash
VBoxManage hostonlyif create
VBoxManage hostonlyif ipconfig vboxnet0 --ip 192.168.10.1 --netmask 255.255.255.0
VBoxManage dhcpserver remove --interface vboxnet0 2>/dev/null || true
```

El primer comando crea la interfaz host-only y la nombra (`vboxnet0`). El segundo le asigna `192.168.10.1/24`. El tercero elimina su servidor DHCP, para que las IP sean fijas. Se espera que `ip -br addr show vboxnet0` muestre `192.168.10.1/24`. En Windows el equivalente es configurar el adaptador host-only desde el administrador de red de VirtualBox, con DHCP desactivado.

[IMAGEN P-01 — pie: red host-only del host. Muestra el adaptador `vboxnet0` con `192.168.10.1/24` y sin DHCP, en Debian 13 con VirtualBox 7.2. Importa porque es la puerta de entrada del host a las VMs: sin esa red, nada del proyecto resuelve ni conecta.]

### 3.4 Flujos

> **Concepto clave — Guest properties.** Son pares nombre-valor que el host deja dentro de una VM (y que la VM puede leer y escribir) sin usar la red. Se parecen a una nota pegada en la puerta del apartamento: quien llega la lee. Aquí el host deja el hostname y la IP antes de encender la VM, y la VM responde con su estado. Es la misma idea que el *user data* de una instancia EC2.

**Aceptar (crear).**

1. La webapp valida el nombre: formato, nombre reservado, que no esté repetido y que no supere el máximo de instancias.
2. Guarda la instancia en estado `creando` con la siguiente IP libre (por ejemplo `.30`) y responde de inmediato.
3. La cola ejecuta `createvm`, configura red y disco multiconexión, escribe las guest properties y enciende la VM en modo *headless* (sin ventana).
4. Dentro de la VM, `httpaas-firstboot` lee las propiedades y se configura sola: hostname, IP, llaves SSH nuevas, `machine-id` nuevo, VirtualHost y directorio del sitio. Al terminar escribe `/httpaas/estado = listo`.
5. La webapp espera `listo` y comprueba que el puerto 22 esté abierto.
6. Registra el nombre en DNS por SSH hacia `ns`.
7. Verifica que `paisajes.cloud.local` resuelva a la IP asignada consultando a `192.168.10.10` y marca la instancia como `lista`.

> **Concepto clave — Enlace simbólico (*symlink*) y Zip Slip.** Un enlace simbólico es un archivo que apunta a otro, como un acceso directo. *Zip Slip* es un ataque en el que un `.zip` trae rutas como `../../etc/passwd` para escribir fuera de la carpeta destino al descomprimirlo. Se parece a un paquete que, al abrirlo, mueve los muebles de otra habitación. Importa porque el `.zip` lo envía el usuario y debe tratarse como dato hostil.

**Publicar.**

1. La webapp recibe el nombre y el `.zip` (tamaño máximo configurable) y lo valida en el host: sin rutas con `..`, sin rutas absolutas, sin enlaces simbólicos, con límite de archivos y de tamaño descomprimido, y con `index.html` en la raíz o dentro de una única carpeta.
2. Copia el zip a `/home/httpaas/subidas/<id>.zip` en la instancia con `scp`.
3. Ejecuta por SSH `sudo httpaas-publicar` sobre ese archivo. El script valida otra vez, descomprime en una carpeta temporal, cambia el sitio de forma atómica y recarga Apache.
4. La webapp comprueba `GET http://<ip>/` con la cabecera `Host: <nombre>.cloud.local` y espera un 200.

**Eliminar.**

1. Marca la instancia como `eliminando`.
2. Quita el registro DNS con `httpaas-dns quitar`.
3. Apaga la VM, espera a que se apague y ejecuta `unregistervm --delete`, que borra solo el disco diferencial.
4. Quita la llave de la instancia del `known_hosts` propio de la app y borra la instancia del estado. La IP vuelve a quedar libre.
5. Comprueba que `ApacheServer.vdi` sigue existiendo.

Si un paso falla, la instancia queda en `error` con el detalle en el registro, y el botón Eliminar limpia lo que se haya alcanzado a crear.

### 3.5 Diagrama

```text
                       HOST — 192.168.10.1 en vboxnet0
 ┌────────────────────────────────────────────────────────────────────────────┐
 │  Navegador ──http──► Webapp Go (127.0.0.1:8080)                             │
 │                       │  cola de trabajos (1 a la vez)                      │
 │                       ├─ VBoxManage ─────────────► VirtualBox               │
 │                       ├─ ssh httpaas@192.168.10.10 ► sudo httpaas-dns ...    │
 │                       └─ scp/ssh httpaas@<ip> ─────► sudo httpaas-publicar   │
 └──────────────────────────────┬─────────────────────────────────────────────┘
                                │ red host-only  192.168.10.0/24
       ┌────────────────────────┼─────────────────────────────┐
       ▼                        ▼                             ▼
 VM "ns" (Bind9)          VM "paisajes" (Apache)       VM "negocios" (Apache)
 192.168.10.10            192.168.10.30                192.168.10.31
 zona cloud.local.        disco diferencial ─┐         disco diferencial ─┐
 A paisajes → .30                            └──── ApacheServer.vdi ◄─────┘
 A negocios → .31                                  (multiconexión)
```

[IMAGEN docs/Arquitectura.png — pie: diagrama de la arquitectura. El host ejecuta la webapp y VirtualBox; la webapp habla con VirtualBox por `VBoxManage`, con `ns` por SSH y con cada instancia por `scp`/SSH. Todas las VMs comparten la red host-only y salen a internet por NAT.]

### 3.6 Decisiones de diseño (D1–D16)

| # | Qué se decidió | Por qué | Alternativa descartada |
|---|---|---|---|
| D1 | La webapp corre en el host | Solo ahí existe `VBoxManage`; es el plano de control, como la consola de AWS | — |
| D2 | Red host-only sin DHCP y un adaptador NAT por VM | El host llega a las VMs con IP fijas que no dependen del wifi; sin DHCP no hay choques de IP; el NAT da internet para `apt` | Un solo adaptador, que no cumple las dos funciones |
| D3 | Configurar cada clon con guest properties y un servicio de primer arranque | La webapp deja los datos antes de arrancar y no necesita entrar a una VM que todavía tiene la IP de otra; funciona aunque se creen varias seguidas | Entrar por SSH a la IP de mantenimiento `.20` del clon (queda como plan B si las Guest Additions no funcionan) |
| D4 | Procesar los trabajos de a uno (una cola) | Evita carreras al asignar IP y al registrar el disco multiconexión; crear tarda aproximadamente 1 minuto, así que no hace falta más | Ejecutar trabajos en paralelo |
| D5 | DNS dinámico con `update-policy local` y `nsupdate -l` dentro de `ns`, por un script fijo | No hay que instalar herramientas de BIND en el host (sirve en Windows) ni repartir llaves TSIG; basta SSH | Llaves TSIG repartidas y herramientas de BIND en el host |
| D6 | Zonas en `/var/lib/bind`, nunca editadas a mano con `named` encendido | Con actualizaciones dinámicas BIND escribe un journal `.jnl`; editar el archivo lo corrompe; AppArmor solo permite escribir en esa carpeta | Editar la zona a mano con el servidor vivo |
| D7 | Usuario `httpaas` con llaves; `sudo` solo para `httpaas-dns` o `httpaas-publicar` | Mínimo privilegio: con una llave robada solo se pueden ejecutar esos dos scripts, que validan sus argumentos | — |
| D8 | SSH con `BatchMode=yes`, `StrictHostKeyChecking=accept-new` y un `known_hosts` propio; `ssh-keygen -R` al eliminar | Cada clon genera llaves de host nuevas; `accept-new` fija la llave la primera vez y detecta cambios; sin `-R`, la siguiente VM con esa IP sería rechazada | Eliminar sin olvidar la llave del host |
| D9 | Datos del usuario siempre como argumentos separados; nunca concatenados en `sh -c` | Evita la inyección de comandos (`paisajes; rm -rf /`) | Armar comandos con `sh -c` (fue el error principal del Parcial 2) |
| D10 | Hostname `^[a-z][a-z0-9-]{1,30}[a-z0-9]$`; reservados `ns`, `www`, `apache-plantilla`, `localhost`; el nombre de la VM es el hostname | Es una etiqueta DNS válida (RFC 1123) y es lo que muestra la figura del profesor | — |
| D11 | Validar el zip en el host (Go) y de nuevo en la VM (`httpaas-publicar`) | Defensa en dos capas contra Zip Slip, enlaces simbólicos y bombas zip | — |
| D12 | Estado en `data/instancias.json` con escritura atómica; al arrancar se compara con `VBoxManage list vms` | Sin base de datos y sobrevive a reinicios; la comparación detecta VMs borradas a mano (`perdida`) | Una base de datos |
| D13 | Puertos y adaptadores: el servicio solo conoce `Hipervisor`, `DNS` y `Publicador`, cada uno con un Fake | Las tres partes trabajan en paralelo sin VMs y las pruebas corren sin VirtualBox | — |
| D14 | Configurar `ns` y la plantilla con scripts versionados | Cualquiera reconstruye las VMs igual en su PC; es infraestructura como código | Configurar y copiar archivos a mano, que causó los fallos de los primeros discos de la plantilla (sección 8.7) |
| D15 | Interfaz con `html/template` en un solo binario (`go:embed`) | Menos piezas, menos errores | Frameworks de frontend |
| D16 | Firmware BIOS y controlador SATA (AHCI) en plantilla y clones, ambos en `config.json` | El clon debe coincidir con el disco donde se instaló, o no arranca | Un clon con firmware o controlador distintos a los de la plantilla |

«—» significa que la fuente no registra una alternativa descartada; la justificación de la decisión sí está documentada.

**Por qué (lo ganado y lo sacrificado en cada decisión).**

- **D1.** Se gana acceso directo a `VBoxManage`; se sacrifica poder alojar la webapp fuera del PC.
- **D2.** Se gana direccionamiento predecible; se sacrifica sencillez, porque cada VM lleva dos adaptadores.
- **D3.** Se gana que cada clon se configure sin intervención; se sacrifica depender de que las Guest Additions funcionen.
- **D4.** Se gana ausencia de carreras; se sacrifica velocidad: las operaciones esperan turno.
- **D5.** Se gana no tener secretos que distribuir; se sacrifica una dependencia de SSH hacia `ns` para cada cambio DNS.
- **D6.** Se gana una zona que no se corrompe; se sacrifica la comodidad de editar el archivo directamente.
- **D7.** Se gana un daño limitado si una llave se filtra; se sacrifica flexibilidad administrativa.
- **D8.** Se gana detectar cambios de identidad sin pedir confirmaciones; se sacrifica mantener un `known_hosts` propio.
- **D9.** Se gana inmunidad a la inyección de comandos; se sacrifica la facilidad de escribir una sola línea de shell.
- **D10.** Se gana que el mismo nombre sirva como VM, hostname y etiqueta DNS; se sacrifica libertad en los nombres.
- **D11.** Se gana que ninguna capa dependa de la otra; se sacrifica mantener dos validadores. Esa duplicación causó una discrepancia real (sección 10.1), corregida igualando reglas.
- **D12.** Se gana simplicidad; se sacrifica consultar o consolidar datos con herramientas de base de datos.
- **D13.** Se gana paralelismo y pruebas rápidas; se sacrifica escribir y mantener Fakes.
- **D14.** Se gana reproducibilidad; se sacrifica el tiempo inicial de escribir los scripts.
- **D15.** Se gana un solo binario sin dependencias; se sacrifica la riqueza de un framework.
- **D16.** Se gana que los clones arranquen; se sacrifica la libertad de elegir otro firmware para la plantilla. La VM `ns` no está sujeta a esta restricción (sección 7.2).

---

## 4. Tecnologías y herramientas

| Herramienta | Para qué se usó | Por qué se eligió |
|---|---|---|
| Go (1.24 como mínimo), solo biblioteca estándar | Webapp, orquestador y adaptadores (`net/http`, `html/template`, `go:embed`, `os/exec`, `archive/zip`, `net`) | Un solo binario; ya se usaba en el Parcial 2; el enunciado exige Golang |
| Oracle VirtualBox y `VBoxManage` | Crear, arrancar y eliminar VMs, discos multiconexión y guest properties | Lo exige el enunciado; ofrece control completo por línea de comandos |
| Debian 13 (modo CLI) | Sistema de `ns`, de la plantilla y de las instancias | Lo exige el enunciado |
| Apache HTTP Server | Servidor web de cada instancia | Lo exige el enunciado |
| Bind9 (`named`, `nsupdate -l`, `dig`, `named-checkconf`, `named-checkzone`, `rndc`) | Servidor DNS, actualizaciones dinámicas y validación | Lo exige el enunciado; `nsupdate -l` evita repartir llaves |
| OpenSSH (`ssh`, `scp`, `ssh-keygen`) | Canal remoto sin contraseña hacia `ns` y las instancias | Permite automatizar sin interacción (`BatchMode=yes`) |
| Guest Additions y `VBoxControl` | Que la VM lea y escriba guest properties | Es el mecanismo del contrato entre host y VM |
| systemd | Unidad `httpaas-firstboot.service` de primer arranque | Garantiza que la configuración corra una vez al arrancar |
| Scripts bash (`set -euo pipefail`) | `httpaas-dns`, `httpaas-firstboot`, `httpaas-publicar`, `instalar.sh` | Scripts fijos que validan sus argumentos |
| `sudo` y `visudo -cf` | Permitir solo los scripts del proyecto y validar `sudoers` | Mínimo privilegio (D7) |
| `unzip` y `zipinfo` | Listar y descomprimir el zip dentro de la VM | Validación e instalación del sitio |
| `make`, `gofmt`, `go vet`, `go test -race` | Puerta de calidad local (`make check`) | Evita que entre código que no compila o no pasa las pruebas |
| Flujo de GitHub que ejecuta `make check` | Misma verificación en cada cambio propuesto | En el Parcial 2 un commit que no compilaba bloqueó al equipo |
| NRPT (Windows), `resolvectl` y `nsswitch.conf` (Linux) | Que el host resuelva `*.cloud.local` sin afectar internet | DNS dividido: solo ese sufijo va a `ns` |
| Git Bash | Ejecutar los scripts `.sh` del host en Windows | Los scripts son bash |
| JSON (`data/instancias.json`, `logs/operaciones.jsonl`) | Estado persistente y registro de operaciones | Sin base de datos |
| Markdown y Pandoc | Redacción y exportación de este documento | El repositorio no depende de una herramienta concreta |

---

## 5. El contrato común: `internal/puertos`

### 5.1 Qué es y por qué se acordó primero

`internal/puertos` define las tres interfaces que el orquestador necesita. El centro del sistema (`internal/instancias`) solo conoce esas interfaces; cada parte implementa una. Esta organización se llama *puertos y adaptadores*. Se acordó antes de dividir el trabajo porque era el único punto donde las tres partes se tocan. Con las interfaces fijas, cada parte pudo avanzar sin esperar las VMs de las otras. Un cambio al contrato exige acuerdo del equipo, y cuando hubo ajustes se anotaron primero en el plan.

> **Concepto clave — Idempotencia.** Una operación es idempotente si repetirla produce el mismo resultado que hacerla una vez. Se parece a pulsar el botón de un ascensor ya llamado: apretarlo otra vez no hace nada nuevo. Importa aquí porque `Agregar` y `Quitar` pueden repetirse sin romper la zona, y porque los scripts de instalación pueden correrse dos veces.

### 5.2 Las tres interfaces

```go
type Hipervisor interface {
    Crear(ctx context.Context, spec SpecVM) error
    EsperarLista(ctx context.Context, nombre string) error
    Eliminar(ctx context.Context, nombre string) error
    Listar(ctx context.Context) ([]string, error)
}

type SpecVM struct {
    Nombre  string // hostname y nombre de la VM
    IP      string // p. ej. "192.168.10.30"
    Prefijo int    // p. ej. 24
}

type DNS interface {
    Agregar(ctx context.Context, host, ip string) error
    Quitar(ctx context.Context, host string) error
    Resolver(ctx context.Context, host string) (string, error)
}

type Publicador interface {
    Publicar(ctx context.Context, host, ip, zipLocal string) error
}
```

| Interfaz | Qué representa | Quién la implementa |
|---|---|---|
| `Hipervisor` | Crear, esperar, eliminar y listar las VMs | `internal/vbox` (A) |
| `DNS` | Agregar y quitar registros A y consultar la zona directamente (`Agregar` y `Quitar` son idempotentes) | `internal/dns` (B) |
| `Publicador` | Desplegar un zip ya validado en una instancia | `internal/publicar` (C) |

Una regla de seguridad quedó en el contrato: `Eliminar` solo borra VMs que creó el servicio, las que tienen la guest property `/httpaas/hostname` igual al nombre. Una VM ajena con el mismo nombre no se toca, y eso no es un error.

### 5.3 Los Fakes

`internal/puertos/fakes.go` contiene `FakeHipervisor`, `FakeDNS` y `FakePublicador`. Guardan las llamadas recibidas y aceptan un error programado por método. Sirven para tres cosas:

- probar el orquestador sin VMs ni VirtualBox;
- ejecutar toda la webapp en modo `-simular`;
- permitir que cada parte avance sin depender de las demás.

### 5.4 Guest properties, scripts y estados

| Propiedad | La escribe | Valor de ejemplo | Para qué |
|---|---|---|---|
| `/httpaas/hostname` | Host, antes de `startvm` | `paisajes` | Hostname y VirtualHost |
| `/httpaas/ip` | Host | `192.168.10.30/24` | IP fija de `enp0s3` |
| `/httpaas/dominio` | Host | `cloud.local` | `ServerName paisajes.cloud.local` |
| `/httpaas/dns` | Host | `192.168.10.10` | Resolver de la instancia |
| `/httpaas/estado` | VM (`httpaas-firstboot`) | `configurando`, `listo` o `error:<detalle>` | El host sabe cuándo puede seguir |

| Script (en la VM) | Uso | Salida |
|---|---|---|
| `/usr/local/sbin/httpaas-dns` (en `ns`) | `sudo httpaas-dns agregar <host> <ip>`, `quitar <host>` o `listar` | Código 0 si salió bien; mensaje en stderr si no; `listar` imprime `host ip` por línea |
| `/usr/local/sbin/httpaas-publicar` (en cada instancia) | `sudo httpaas-publicar /home/httpaas/subidas/<id>.zip` | Código 0 y `publicado <n> archivos`; código 2 (zip inválido); código 3 (falló Apache) |

Estados de una instancia: `creando` → `lista` → (`publicando` → `lista`) → `eliminando` → se borra. Cualquier paso puede terminar en `error`. Una instancia que está en el JSON pero no en VirtualBox pasa a `perdida`. Desde `error` y `perdida` solo se puede Eliminar. Mientras una instancia está en `creando`, `publicando` o `eliminando`, el trabajador de la cola es su dueño y no se acepta otra operación sobre ella. Así una publicación nunca deshace un borrado.

---

## 6. Parte A — Plataforma y orquestación

### 6.1 Para qué sirve esta parte y qué problema resuelve

La Parte A es el plano de control: la webapp que recibe las solicitudes, la cola que crea, publica y elimina instancias, y los adaptadores que hablan con VirtualBox y por SSH. Resuelve que ninguna operación dependa de que alguien escriba comandos. También resuelve cuatro riesgos:

- que dos clics simultáneos reciban la misma IP;
- que una operación larga bloquee la página;
- que un fallo a medias deje recursos huérfanos;
- que un dato del usuario llegue a un comando del sistema.

### 6.2 Piezas

**Esqueleto y control de calidad.** El proyecto es un módulo Go con un `Makefile` (`build`, `run`, `simular`, `test`, `check`, `vm-test`) y un flujo en GitHub que ejecuta `make check` en cada propuesta de cambio. `make check` corre `gofmt`, `go vet`, las pruebas con `-race` y la compilación. Las pruebas que necesitan VMs reales usan la etiqueta de compilación `vm` y se ejecutan con `make vm-test`.

[IMAGEN A-01 — pie: `make check` en verde. `gofmt`, `go vet`, las pruebas con `-race` y la compilación pasan. Importa porque es la puerta de calidad antes de cada commit.]

[IMAGEN A-02 — pie: el flujo `check` en verde en GitHub. Corre `make check` en cada propuesta de cambio. Importa porque nadie puede integrar código que no compila o no pasa las pruebas.]

**Configuración (`internal/config`).** Lee `config.json`, valida cada campo y muestra todos los errores juntos. Rechaza campos desconocidos (un `"dominoi"` se detecta en vez de ignorarse) y rechaza `listen` en `0.0.0.0`, que dejaría la consola accesible desde fuera. Verifica IP, rango `primera ≤ ultima`, existencia del disco base y de la llave, y `max_instancias ≥ 1`. El ejemplo de la configuración:

```json
{
  "listen": "127.0.0.1:8080",
  "dominio": "cloud.local",
  "red": { "base": "192.168.10.", "prefijo": 24, "primera": 30, "ultima": 99 },
  "max_instancias": 3,
  "max_zip_mb": 50,
  "dns": { "ip": "192.168.10.10", "usuario": "httpaas" },
  "ssh": { "usuario": "httpaas", "llave": "keys/httpaas_ed25519", "known_hosts": "keys/known_hosts" },
  "vbox": {
    "vboxmanage": "VBoxManage",
    "disco_base": "/home/<usuario>/VirtualBox VMs/httpaas-base/ApacheServer.vdi",
    "red_hostonly": "vboxnet0",
    "ostype": "Debian_64",
    "firmware": "bios",
    "controlador": "sata",
    "memoria_mb": 512,
    "cpus": 1
  },
  "datos": "data/instancias.json",
  "logs": "logs/"
}
```

En Windows, `red_hostonly` es `VirtualBox Host-Only Ethernet Adapter` y `vboxmanage` es la ruta completa a `VBoxManage.exe`, porque no está en el `PATH`.

**Ejecutor SSH (`internal/executor`).** Ejecuta comandos remotos y copia archivos sin interacción. Usa estas opciones:

- `-i <llave>`: indica la llave privada.
- `-o BatchMode=yes`: falla en lugar de pedir contraseña.
- `-o StrictHostKeyChecking=accept-new`: fija la huella de un host nuevo y detecta cambios posteriores.
- `-o UserKnownHostsFile=<known_hosts>`: usa un archivo de huellas propio de la app.
- `-o ConnectTimeout=10`: limita el tiempo de conexión.

`OlvidarHost(ip)` ejecuta `ssh-keygen -R <ip> -f <known_hosts>` para que la próxima VM con esa IP no sea rechazada (D8). Si `ssh` no existe en el `PATH`, si el proceso muere por el límite de tiempo o si el código es 255, cuenta como fallo de conexión y nunca como éxito.

**Registro de operaciones (`internal/oplog`).** Escribe una línea JSON por operación en `logs/operaciones.jsonl`, con comandos, salida, código y duración. El archivo tiene permisos 600, porque la salida puede contener datos sensibles. Al usuario se le muestra un mensaje claro y el detalle técnico queda en este registro.

[IMAGEN A-03 — pie: una línea de `logs/operaciones.jsonl`. La publicación de `paisajes`: cada comando (`scp` y `ssh`) con su salida, su código de salida y su duración; el archivo tiene permisos 600. Importa porque deja la información técnica para diagnosticar un fallo sin repetirlo.]

**Almacén e IP (`internal/almacen`).**

- `Reservar` valida el nombre, el máximo de instancias y la IP libre en un solo paso bajo un candado. La prueba lanza 40 reservas simultáneas y ninguna recibe la misma IP.
- La IP asignada es la menor libre entre `primera` y `ultima`; si el rango está lleno, devuelve un error claro.
- Cada cambio se guarda con un archivo temporal y `os.Rename`, así un corte nunca deja el JSON a medias.
- Un archivo inexistente equivale a una lista vacía, y un JSON corrupto produce un error y nunca se sobrescribe.

**Orquestador con cola (`internal/instancias`).** Un solo goroutine consume un canal de trabajos (D4). Los métodos `Solicitar*` validan, guardan el estado y encolan, así la respuesta HTTP es inmediata.

| Operación | Secuencia |
|---|---|
| Crear | validar → `creando` + IP → `Crear` → `EsperarLista` (máximo 3 min) → `DNS.Agregar` → `DNS.Resolver` igual a la IP → `lista` |
| Publicar | solo desde `lista` → `publicando` → `Publicador.Publicar` → `lista`. Si falla, vuelve a `lista` con el error anotado (Apache sigue sirviendo el contenido anterior). Borra el zip temporal siempre |
| Eliminar | desde cualquier estado excepto `creando`, `publicando` o `eliminando` → `DNS.Quitar` → `Hipervisor.Eliminar` → `OlvidarHost` → quitar del almacén. Si un paso falla, sigue con los demás y deja `error` con el detalle |

Al arrancar, `Reconciliar()` compara el JSON con `Hipervisor.Listar`: las instancias que no existen en VirtualBox pasan a `perdida`, y las que quedaron en `creando` o `publicando` pasan a `error` («la app se reinició durante la operación»). Cada trabajo tiene un límite de tiempo, y al apagar la app se espera a que termine el trabajo en curso.

**Servidor web (`internal/web`).**

| Ruta | Función |
|---|---|
| `GET /` | Dashboard |
| `POST /instancias` | Botón Aceptar (campo `nombre`) |
| `POST /contenido` | Botón Publicar (campos `nombre` y `archivo`, multipart) |
| `POST /instancias/{nombre}/eliminar` | Botón Eliminar |
| `GET /api/instancias` | JSON del estado |
| `GET /salud` | Responde `ok` |

Publicar usa una ruta fija para que un solo formulario con dos botones, como en la figura del enunciado, funcione sin JavaScript. Todo POST sigue el patrón POST → redirección 303 → GET, con un mensaje flash. Un error de validación produce un 422 y vuelve a mostrar el formulario con el error junto al campo. Medidas de seguridad:

- solo acepta su propio `Host`, contra el DNS *rebinding* (si la lista está vacía, rechaza todo);
- rechaza los POST cuyo `Origin` no sea el de la propia app (CSRF);
- limita el cuerpo con `http.MaxBytesReader` (413);
- usa `html/template`, que escapa el contenido.

Los nombres de los campos HTML son constantes compartidas (`dominio.CampoNombre` y `dominio.CampoArchivo`), porque en el Parcial 2 hubo formularios que no coincidían con el código. El modo `-simular` usa los tres Fakes con un retardo de 2 s para ver los estados sin VMs.

[IMAGEN A-04 — pie: dashboard en modo simulado con `paisajes` en `creando`. La página responde al instante y el trabajo corre en la cola. Importa porque la interfaz nunca se bloquea esperando a VirtualBox.]

[IMAGEN A-05 — pie: el nombre `a;rm` rechazado junto al campo (HTTP 422). Importa porque se valida antes de ejecutar nada: un nombre con `;` nunca llega a un comando.]

**Adaptador VirtualBox (`internal/vbox`).** Implementa `Hipervisor` llamando a `VBoxManage` con `exec.CommandContext` y argumentos separados. Los argumentos los arman funciones puras (`argsCrear(spec, cfg)`), con pruebas de tabla que no requieren VirtualBox. La secuencia de comandos es la del procedimiento de clonación manual (sección 8.6).

- Si `Crear` falla después de `createvm`, deshace lo creado con `unregistervm --delete`.
- La marca `/httpaas/hostname` se escribe justo después de `createvm`, y `Eliminar` solo borra VMs que la tienen.
- `EsperarLista` exige dos cosas: `/httpaas/estado = listo` y el puerto 22 abierto en la IP de la instancia.
- `Eliminar` ejecuta `controlvm <nombre> poweroff` (ignora «not running»), espera `VMState="poweroff"` en `showvminfo --machinereadable` y ejecuta `unregistervm <nombre> --delete`. Si VirtualBox sigue con la VM bloqueada después de apagarla, reintenta el borrado, y solo en ese caso.

[IMAGEN A-06 — pie: `VBoxManage list vms` con `paisajes` y `negocios`. Las dos VMs las creó la app, sin intervención.]

[IMAGEN A-07 — pie: el disco base en multiconexión con dos hijos diferenciales, uno por instancia. Importa porque cada clon escribe solo en su hijo y el disco base no cambia.]

[IMAGEN A-08 — pie: después de eliminar `paisajes` desde la app, la VM ya no está, el nombre ya no resuelve y `ApacheServer.vdi` sigue ahí. Importa porque Eliminar no toca el disco base.]

**Integración.** `main.go` conecta en modo real `vbox`, `dns` (con SSH hacia `ns`) y `publicar` (con SSH por IP de instancia), junto con `OlvidarHost`. `publicar.ValidarZip` se usa también en modo simulado. El aviso de `ns` (sección 7.6) se revisa cada 30 s en segundo plano y llega a la plantilla como `Vista.DNSAviso`.

[IMAGEN A-09 — pie: el formulario con `paisajes` y `paisajes.zip`, antes de pulsar Publicar. Es el flujo de la figura del enunciado.]

[IMAGEN A-10 — pie: las instancias en `lista`, cada una con su enlace. Es el resultado que ve el usuario.]

**Script de prueba completa y README.** `scripts/prueba-completa.sh` ejecuta los pasos de la sección 9. El `README.md` explica la preparación, la configuración, el uso y las pruebas, pensado para quien llega al repositorio sin contexto.

### 6.3 Cómo se verificó la Parte A

- **Una prueba que falla antes de cada corrección.** Si la prueba no falla antes de corregir, no prueba el bug.
- **Mutaciones.** Se rompió a propósito cada corrección para comprobar que alguna prueba lo detecta. Resultado: 25 de 25 en la Parte A y 22 de 22 en el adaptador de publicación.
- **Dos revisiones independientes** encontraron 18 problemas reales, todos corregidos con su prueba.
- **Control de calidad automático** en cada cambio propuesto. Este control encontró una carrera al apagar que en el PC no aparecía: 3 fallas en 300 repeticiones antes de corregirla y 0 en 300 después.
- **Prueba con VM real.** Con el disco real de la Parte C se crearon y borraron dos instancias, y `ApacheServer.vdi` siguió en multiconexión y sin hijos.
- **Con un disco base inválido a propósito**, VirtualBox lo rechazó, la app deshizo la VM (no quedó ninguna), mostró un error claro, y Eliminar limpió el DNS real.

### 6.4 Problemas de la Parte A

Se detallan en la sección 10.1.

### 6.5 Verificación de resolución en el flujo

Al crear una instancia, la webapp no se limita a registrar el nombre: consulta directamente a `192.168.10.10` y compara la IP devuelta con la asignada. Solo entonces marca la instancia como `lista`.

---

## 7. Parte B — DNS y experiencia de usuario

### 7.1 Para qué sirve esta parte y qué problema resuelve

La Parte B da nombre a cada instancia y presenta el sistema al usuario. El proyecto necesita que `paisajes.cloud.local` resuelva aunque el DNS del proveedor de internet no sepa nada de ese dominio. Para eso `ns` es autoritativo de `cloud.local`, y la webapp lo actualiza por SSH con un script fijo. La parte también resuelve que el host consulte a `ns` solo para ese sufijo y que el usuario vea y opere las instancias en un dashboard.

### 7.2 VM `ns`

La VM `ns` es Debian 13 sin entorno gráfico, con `enp0s3` host-only en `192.168.10.10/24` sin gateway y `enp0s8` por DHCP (NAT). Lleva el usuario `httpaas` con las llaves de `deploy/llaves/`, `sshd` con `PasswordAuthentication no` y `PermitRootLogin no`, y una instantánea `ns-base`.

> **Concepto clave — Instantánea (*snapshot*).** Es una fotografía del estado de una VM en un momento dado, a la que se puede volver. Se parece al punto de guardado de un videojuego. Aquí `ns-base` guarda la VM ya configurada, y la plantilla usa snapshots para probar clones sin dañarla.

**Recursos reales.** La VM quedó con EFI, 2 vCPU, 1024 MB y disco de 18 GB. El plan preveía BIOS, 1 CPU, 512 MB y 8 GB. Se mantuvo EFI porque el disco ya estaba instalado así y cambiarlo obliga a reinstalar el cargador. La restricción de BIOS y SATA es del disco multiconexión de la plantilla, no de `ns`.

**Decisiones.**

- `ns` no la aprovisiona la webapp: es plataforma previa (D14). Automatizarlo es trabajo futuro (M2).
- El endurecimiento de `sshd` va después de probar la llave. Al revés, un error en `authorized_keys` deja la VM accesible solo por consola.
- La instantánea se toma con la VM apagada, porque una con RAM guardada es más frágil como imagen base.
- La VM se resuelve a sí misma: `named` primero y el DNS del NAT después. El DHCP de `enp0s8` entregaba un DNS inalcanzable, y se corrigió anteponiendo `127.0.0.1` y `10.0.3.3` en `/etc/dhcp/dhclient.conf`.

```bash
ssh -i keys/httpaas_ed25519 httpaas@192.168.10.10 hostname
```

Ejecuta `hostname` en `ns` por SSH con la llave del proyecto. Se espera ver `ns` sin que se pida contraseña.

[IMAGEN B-01 — pie: `ip -br addr` en `ns`. `enp0s3` con `192.168.10.10/24` (host-only) y `enp0s8` con una IP del NAT. Importa porque confirma la IP fija que usará la webapp y la salida a internet para `apt`.]

[IMAGEN B-02 — pie: SSH desde el host sin contraseña. `ssh -i keys/httpaas_ed25519 httpaas@192.168.10.10 hostname` devuelve `ns`. Importa porque es el mismo canal que usará el adaptador `internal/dns`.]

### 7.3 Zona `cloud.local`

Cuatro archivos versionados en `deploy/ns/` y copiados a la VM:

| Archivo | Destino | Qué hace |
|---|---|---|
| `named.conf.options` | `/etc/bind/named.conf.options` | Escucha en `127.0.0.1` y `192.168.10.10`; atiende la red del proyecto; recursión con reenvío a `1.1.1.1` y `8.8.8.8` |
| `named.conf.local` | `/etc/bind/named.conf.local` | Declara las dos zonas como `type primary` con `update-policy local` |
| `db.cloud.local` | `/var/lib/bind/db.cloud.local` | Zona directa: `ns A 192.168.10.10` |
| `db.192.168.10` | `/var/lib/bind/db.192.168.10` | Zona inversa: `10 PTR ns.cloud.local.` |

La zona inversa es un extra: el enunciado solo pide registros A. Cuesta poco y permite `dig -x`.

```bash
named-checkconf
named-checkzone cloud.local /var/lib/bind/db.cloud.local
systemctl restart named
```

`named-checkconf` valida la sintaxis y no imprime nada si está bien. `named-checkzone` parsea la zona y muestra el serial cargado. `systemctl restart named` aplica los cambios; se valida siempre antes de reiniciar, porque un error de sintaxis dejaría `named` caído.

```bash
dig @127.0.0.1 ns.cloud.local +short
dig @127.0.0.1 -x 192.168.10.10 +short
dig @127.0.0.1 debian.org +short
```

Las tres consultas cubren cosas distintas: la zona directa (se espera `192.168.10.10`), la inversa (se espera `ns.cloud.local.`) y la recursión (se espera una o más IP de `debian.org`).

**Decisiones.** Los archivos de zona van en `/var/lib/bind` porque el usuario `bind` necesita escribir el journal y `/etc/bind` es de `root` (D6). Se usa `update-policy local` y no una llave TSIG repartida. `forwarders` y `recursion` permiten que las instancias resuelvan `deb.debian.org` a través de `ns`. El serial tiene el formato `YYYYMMDDnn`, con uno por zona.

[IMAGEN B-03 — pie: validación de la configuración. `named-checkconf` y `named-checkzone` no imprimen errores y muestran el serial cargado. Importa porque valida antes de reiniciar: un error de sintaxis dejaría `named` caído.]

[IMAGEN B-04 — pie: zona directa, inversa y recursión. Los tres `dig` muestran `ns → .10`, el PTR inverso y una respuesta de `debian.org`. Importa porque cubre las tres funciones de `ns`: resolver el proyecto, la inversa y salir a internet.]

### 7.4 Actualización dinámica con `httpaas-dns`

> **Concepto clave — Actualización dinámica de DNS.** Es la posibilidad de agregar o quitar registros de una zona mientras el servidor está en marcha, sin editar el archivo ni reiniciar (RFC 2136). Se parece a corregir un directorio telefónico mientras la gente lo consulta, en vez de reimprimirlo. BIND guarda esos cambios en un archivo auxiliar llamado journal (`.jnl`).

`deploy/ns/httpaas-dns` recibe `agregar`, `quitar` y `listar`. Valida el host (`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`) y la IP (`192.168.10.1` a `254`) antes de ejecutar nada, y rechaza `ns` y las IP reservadas `.1`, `.10` y `.20`. Se instala en `/usr/local/sbin/httpaas-dns` (root, 0755). El archivo `/etc/sudoers.d/httpaas` permite solo ese script y se valida con `visudo -cf`.

`agregar` hace una sola llamada a `nsupdate -l` con dos bloques, uno por zona, cada uno con su `send`:

```text
zone cloud.local
update delete H.cloud.local A
update add H.cloud.local 300 A IP
send
zone 10.168.192.in-addr.arpa
update delete N.10.168.192.in-addr.arpa PTR
update add N.10.168.192.in-addr.arpa 300 PTR H.cloud.local.
send
```

Cada bloque actualiza una zona. `update delete` antes de `update add` hace que reagregar un host no falle. Después, el script confirma con `dig` que el `A` y el `PTR` coincidan, y si no, sale con código 1. `quitar` obtiene primero la IP actual para saber qué `PTR` borrar, borra `A` y `PTR`, y no falla si el registro no existía. `listar` hace una transferencia `AXFR` filtrada a `host ip`.

```bash
ssh httpaas@192.168.10.10 sudo httpaas-dns agregar prueba 192.168.10.99
dig @192.168.10.10 prueba.cloud.local +short
ssh httpaas@192.168.10.10 sudo httpaas-dns agregar 'x;id' 192.168.10.99
ssh httpaas@192.168.10.10 sudo -n cat /etc/shadow
```

El primer comando registra `prueba` y se espera `added prueba.cloud.local -> 192.168.10.99`. El segundo consulta el registro y se espera `192.168.10.99`. El tercero intenta un nombre malicioso y se espera un rechazo (`invalid host`, código 1). El cuarto intenta leer un archivo privilegiado y se espera `sudo: a password is required`.

**Decisiones.**

- `nsupdate -l` usa la llave de sesión de `named`, sin secretos que distribuir.
- Dos bloques con dos `send`: un mensaje UPDATE pertenece a una sola zona, y mezclar `A` y `PTR` falla con `NOTZONE`.
- Confirmar con `dig` dentro del script: el código de salida significa «el registro está», no «`nsupdate` no se quejó».
- Rechazar `ns` evita sobrescribir el servidor DNS.
- Rechazar `.1`, `.10` y `.20`: escribir `.10` borraría el `PTR` de `ns` y la confirmación con `dig` seguiría pasando, una falla silenciosa.
- `instalar.sh` no sobrescribe zonas existentes, porque ya contienen registros dinámicos; `--forzar-zonas` es la excepción deliberada.

[IMAGEN B-05 — pie: alta de un registro dinámico. `httpaas-dns agregar prueba 192.168.10.99` responde con el alta y `dig` devuelve `.99`. Importa porque es la operación que la webapp hará por SSH al crear una instancia.]

[IMAGEN B-06 — pie: rechazo de un nombre malicioso y de `sudo cat`. El script rechaza `x;id` y `sudo -n cat /etc/shadow` pide contraseña. Importa porque demuestra que los datos no se interpolan y que `httpaas` solo ejecuta `httpaas-dns`.]

[IMAGEN B-07 — pie: journal de la zona. Los `.jnl` en `/var/lib/bind` prueban que la actualización fue dinámica (RFC 2136), no una reescritura del archivo. Importa porque es la diferencia entre el camino soportado y el que corrompe la zona.]

### 7.5 Imagen `ns.ova` y resolución en el host

**Exportación.** `VBoxManage export ns -o ns.ova` con la VM apagada, validado con `VBoxManage import ns.ova --dry-run` antes de compartirlo. Resultado: archivo de 1.14 GB, disco de 18 GB `streamOptimized`, tipo `Debian13_64`, 2 vCPU, 1024 MB y redes HostOnly y NAT. El `.ova` está en `.gitignore` y se guarda fuera del repositorio. Al importarlo en otro PC, `scripts/importar-ns.sh` conecta el adaptador 1 a la red host-only de ese PC, porque el `.ova` guarda el nombre de la red del PC de origen. Se importó en otro PC y `scripts/verificar-ns.sh` pasó los chequeos de la VM, las zonas y `httpaas-dns`.

**Resolución en el host.** `ns` ya respondía, pero el sistema operativo del host no le preguntaba. Se conecta sin romper internet con DNS dividido, donde solo `cloud.local` va a `ns`:

- **Windows:** una regla NRPT: `Add-DnsClientNrptRule -Namespace ".cloud.local" -NameServers "192.168.10.10"`.
- **Linux:** `scripts/dns-host-linux.sh` (idempotente, con `--undo`). Reubica `dns` antes de `mdns4_minimal` en `nsswitch.conf`, porque `[NOTFOUND=return]` enviaba todo `*.local` a mDNS. Con `systemd-resolved` usa `resolvectl dns vboxnet0 192.168.10.10` y `resolvectl domain vboxnet0 '~cloud.local'`; sin él, antepone `nameserver 192.168.10.10` en `/etc/resolv.conf`.

La verificación final usa `getent hosts` (Linux) o `Resolve-DnsName` (Windows), que pasan por el mismo camino que el navegador. `dig` habla DNS directo y se salta `nsswitch.conf`, por lo que podría decir «funciona» con el sistema roto.

```text
PS> Resolve-DnsName ns.cloud.local
Name             Type IPAddress
ns.cloud.local   A    192.168.10.10
```

La consulta, hecha desde el host, devuelve la IP de `ns`, y `ping ns.cloud.local` responde 4/4. Además `debian.org` sigue resolviendo, lo que confirma que internet no se rompió.

[IMAGEN B-08 — pie: el host resuelve `cloud.local`. `Resolve-DnsName ns.cloud.local` devuelve `192.168.10.10`, `ping` responde 4/4 y `debian.org` sigue funcionando. Importa porque prueba que el navegador puede abrir `http://paisajes.cloud.local` sin romper el internet del host.]

### 7.6 Adaptador `internal/dns` y validación de `ns`

`internal/dns/dns.go` implementa `puertos.DNS`:

1. `Nuevo(executor, cfg)` no conecta: el SSH y el DNS solo se tocan al llamar a un método.
2. `Agregar` y `Quitar` validan antes de construir el comando y ejecutan `sudo /usr/local/sbin/httpaas-dns <sub> <args>` con `executor.Command`, nunca con `sh -c`.
3. `Resolver` usa `net.Resolver` con `PreferGo` y un `Dial` fijo a `192.168.10.10:53`, así no depende de NRPT ni de `nsswitch.conf`.
4. `ejecutar` revisa `ExitCode`, porque el ejecutor solo devuelve `error` cuando el comando no se pudo entregar.

`Resolver` recibe el nombre sin dominio y el adaptador lo agrega, para que nadie construya `x.cloud.local.cloud.local`. No aplica `ValidarHostname` porque la comprobación de `ns` necesita resolver `ns`, que está reservado, y el nombre solo llega a `net.Resolver`, nunca a una línea de comandos.

`Comprobar(ctx)` resuelve `ns` y compara con la IP configurada. Si falla o resuelve a otra IP, devuelve un error cuyo mensaje es la receta de recuperación (`VBoxManage import ns.ova` o `deploy/ns/instalar.sh`) y deja la causa técnica en `Unwrap`. No crea ni repara nada, y no se agregó a `puertos.DNS`, así que el contrato no cambió. Hay cuatro pruebas unitarias (OK, servidor caído, IP equivocada, host vacío) y `TestVMComprobar` contra la `ns` real. Con `dns.ip` apuntando a `192.168.10.250`, el camino de error tarda 5.01 s y devuelve la receta exacta.

### 7.7 Dashboard

`internal/web/templates/dashboard.html` reescribe la plantilla provisional. Incluye:

- **Estados** con color y texto: verde `lista`, ámbar en proceso, rojo `error` o `perdida`.
- **Diseño responsivo** a 360 px: bajo 640 px la tabla pasa a tarjetas.
- **Accesibilidad:** foco visible, etiquetas asociadas a los campos, botones con texto y roles `status` y `alert` en los avisos.
- **Formulario único** multipart con Aceptar y Publicar, enlace en pestaña nueva, IP/24, nombre de la VM, fecha, Eliminar con diálogo `<dialog>` (con `confirm()` de respaldo) y estado vacío.
- **Publicar inteligente** como mejora progresiva: con JavaScript crea la instancia si no existe, espera `lista` sondeando `/api/instancias` y sube el zip. Sin JavaScript quedan los dos botones del diseño original. Depende de que la pestaña siga abierta (aproximadamente 1 minuto en real); la versión robusta es la mejora M3.

**Decisiones.** Los estilos van inline porque `web.go` solo embebe `templates/*.html`. Se usa `<meta refresh>` en vez de sondeo, porque el plan lo dejaba como opcional. En la figura del enunciado la tercera columna dice `paisajes` también en la fila de `negocios`; se interpretó como un error de copia y se muestra el nombre de la VM de cada fila.

**Verificación.** `go test ./internal/web/` en verde. Con dos instancias (`.30` y `.31`) se comprobaron las píldoras y los enlaces. La vista se probó a 360 px reales con `Emulation.setDeviceMetricsOverride` del protocolo de DevTools. Se comprobó la secuencia `creando → publicando → lista` al publicar sin crear antes, y un nombre inválido muestra el aviso sin crear nada. El banner de `ns` apagada aparece en segundos y desaparece solo, en menos de un minuto, al encenderla.

[IMAGEN B-09 — pie: dashboard vacío. Muestra el formulario con sus dos pasos, la descripción de la página y el estado vacío. Importa porque es la primera impresión del usuario y muestra que la pantalla explica qué hace.]

[IMAGEN B-10 — pie: dos instancias en el dashboard. `paisajes` en `.30` y `negocios` en `.31`, con enlace, IP/24, VM, fecha, estado y Eliminar. `negocios` está `lista` y `paisajes` está `perdida` porque su VM se eliminó a mano y `Reconciliar` lo detectó al arrancar. Importa porque muestra el estado normal y la detección de VMs desaparecidas.]

[IMAGEN B-11 — pie: el dashboard a 400 px. La tabla pasa a tarjetas y el formulario se apila. Importa porque demuestra que la pantalla es usable en un celular.]

[IMAGEN B-12 — pie: banner con `ns` apagada. El dashboard muestra la receta (`VBoxManage import ns.ova` o `deploy/ns/instalar.sh`) en vez de un error a medias. Importa porque convierte un fallo de infraestructura en una acción concreta.]

### 7.8 Problemas de la Parte B

Se detallan en la sección 10.2.

---

## 8. Parte C — Plantilla Apache y publicación

> **Estado de la evidencia de esta parte.** Las fuentes documentan con detalle el diseño y los procedimientos de la Parte C, y el resultado global (prueba completa y prueba del adaptador con el disco real). Las capturas C-01 a C-11 están en el documento entregado.

### 8.1 Para qué sirve esta parte y qué problema resuelve

La Parte C entrega la «imagen dorada» de la nube: una VM Apache que, al clonarse, se configura sola con su nombre, su IP y su sitio. También entrega el procedimiento exacto de clonación (que la Parte A automatiza), la publicación segura del `.zip` y el diagrama. Resuelve tres problemas: que cada clon tenga identidad propia (hostname, IP, llaves SSH), que el sitio del usuario se publique sin dejar el servidor a medias y que un zip hostil no pueda dañar la VM.

### 8.2 VM `apache-plantilla` y Guest Additions

La VM base es Debian 13 sin entorno gráfico, con BIOS, controlador SATA (AHCI), disco dinámico de 6 GB llamado `ApacheServer.vdi`, `nic1` host-only con IP de mantenimiento `192.168.10.20` sin gateway y `nic2` NAT por DHCP. Lleva Apache, `unzip`, `sudo`, `curl`, el usuario `httpaas` con las llaves de `deploy/llaves/` y la carpeta `/home/httpaas/subidas` (dueño `httpaas`, modo 0700).

La configuración de red de cada interfaz va en su propio archivo dentro de `/etc/network/interfaces.d/`, porque el primer arranque de los clones reescribe solo el de `enp0s3`. `/etc/network/interfaces` solo contiene `source /etc/network/interfaces.d/*` y la interfaz `lo`.

```bash
VBoxManage showvminfo apache-plantilla | grep -E 'Firmware|Storage|NIC'
```

Muestra firmware, controlador de almacenamiento y adaptadores de red de la VM. Se espera ver BIOS, SATA y los dos adaptadores, que son los datos que el clon debe repetir (D16).

[IMAGEN C-01 — pie: configuración de `apache-plantilla`. `showvminfo` con firmware, controlador y NIC. Importa porque el clon debe coincidir con el disco donde se instaló. Incluida en el documento entregado.]

[IMAGEN C-02 — pie: página por defecto de Apache desde el navegador del host. Incluida en el documento entregado.]

**Guest Additions.** Se instalan para que `VBoxControl` funcione dentro de la VM, se reinicia y se quita el disco de las Additions. La verificación consiste en escribir una propiedad en el host y leerla en la VM, y viceversa:

```bash
VBoxManage guestproperty set apache-plantilla /httpaas/hostname prueba
VBoxControl --nologo guestproperty get /httpaas/hostname
VBoxControl --nologo guestproperty set /httpaas/estado hola
VBoxManage guestproperty get apache-plantilla /httpaas/estado
```

Los dos primeros comandos escriben una propiedad en el host y la leen desde la VM, y se espera `Value: prueba`. Los dos últimos escriben desde la VM y leen en el host, y se espera `Value: hola`. Si las Guest Additions no compilaran, el plan B es entrar por SSH a la IP de mantenimiento `.20` y aplicar la configuración desde la webapp.

[IMAGEN C-03 — pie: lectura y escritura de guest properties en ambas direcciones. Incluida en el documento entregado.]

### 8.3 Servicio de primer arranque `httpaas-firstboot`

> **Concepto clave — Unidad systemd.** *systemd* es el sistema que arranca y supervisa los servicios en Debian, y una *unidad* es el archivo que describe uno. Se parece a la lista de tareas que la VM cumple al encenderse. Aquí una unidad de tipo `oneshot` ejecuta la configuración una vez al arrancar.

El script lee `/httpaas/hostname`, `/httpaas/ip`, `/httpaas/dominio` y `/httpaas/dns`, y procede así:

1. Reintenta la lectura hasta 30 s, porque el driver puede tardar en cargar.
2. Si `/httpaas/hostname` está vacío, sale con 0 sin hacer nada: es la plantilla misma.
3. Valida hostname, IP, dominio y DNS; si algo es inválido, escribe `estado=error:datos-invalidos`.
4. Si `/etc/httpaas/aplicado` ya contiene `<host> <ip>`, es un reinicio: escribe `estado=listo` y sale.
5. Escribe `estado=configurando` e instala un `trap` en ERR que escribe `estado=error:linea-N`.
6. Cambia el hostname y `/etc/hosts`.
7. Genera una identidad única: borra las llaves de host SSH y las regenera con `ssh-keygen -A`, y regenera `/etc/machine-id`.
8. Reconfigura la red en este orden: `ifdown enp0s3`, reescribe el archivo de `enp0s3` con la IP nueva sin gateway, `ifup enp0s3`, y escribe `/etc/resolv.conf` con `nameserver 192.168.10.10` y `search cloud.local`.
9. Configura Apache: página provisional en `/var/www/<host>/`, VirtualHost desde `vhost.conf.tmpl` (`a2ensite`, `a2dissite 000-default`, `apache2ctl configtest`) y recarga.
10. Reinicia SSH, escribe `/etc/httpaas/aplicado` y, por último, `estado=listo`.

La unidad `httpaas-firstboot.service` es `Type=oneshot` con `RemainAfterExit=yes`, `After=vboxadd-service.service network.target`, `Wants=vboxadd-service.service` y `WantedBy=multi-user.target`.

La verificación se hace sobre la propia plantilla sin dañarla: se toma un snapshot, se escriben las propiedades de prueba, se reinicia, se verifica y se restaura el snapshot. Se espera `estado = listo`, `ssh httpaas@192.168.10.29 hostname` → `prueba`, y `curl -H 'Host: prueba.cloud.local' http://192.168.10.29/` devolviendo la página provisional.

[IMAGEN C-04 — pie: `systemctl status httpaas-firstboot`. Incluida en el documento entregado.]

[IMAGEN C-05 — pie: estado `listo` visto desde el host. Incluida en el documento entregado.]

[IMAGEN C-06 — pie: página provisional con el `Host` correcto. Incluida en el documento entregado.]

### 8.4 Script de publicación `httpaas-publicar`

El script recibe la ruta del zip y trabaja así:

1. El argumento debe coincidir con `^/home/httpaas/subidas/[a-zA-Z0-9-]{8,64}\.zip$` y existir; si no, termina con código 2.
2. El nombre de la instancia no llega como argumento: se lee del primer campo de `/etc/httpaas/aplicado`, que escribe `httpaas-firstboot`. Así nadie puede publicar en la carpeta de otro sitio. Si el archivo no existe o el nombre es inválido, código 3.
3. Rechaza rutas absolutas, rutas con `..` y enlaces simbólicos, y limita el zip a 2000 archivos y 200 MB descomprimidos.
4. Descomprime en una carpeta temporal. Exige `index.html` en la raíz o dentro de una única carpeta; si no, código 2.
5. Asigna dueño `www-data`, permisos 0755 y 0644, y cambia el sitio de forma atómica moviendo la carpeta anterior a `.old-<host>` y la nueva a su lugar.
6. Ejecuta `apache2ctl configtest` y recarga Apache. Si falla, restaura la carpeta anterior (rollback) y termina con código 3.
7. Imprime `publicado <n> archivos`.

El archivo `/etc/sudoers.d/httpaas` permite a `httpaas` ejecutar solo este script como root.

**Material de prueba.** `testdata/zips/` contiene `paisajes.zip`, `negocios.zip` (con una carpeta única, para probar esa regla), `sin-index.zip`, `zipslip.zip` (entrada `../../tmp/x`) y `symlink.zip`, y se reconstruyen con `testdata/zips/generar.sh`.

[IMAGEN C-07 — pie: publicación correcta. Incluida en el documento entregado.]

[IMAGEN C-08 — pie: rechazo de `zipslip.zip`. Incluida en el documento entregado.]

### 8.5 `instalar.sh` y disco multiconexión

`deploy/plantilla/instalar.sh` congela la plantilla en una Debian 13 recién instalada: paquetes, usuario, llaves, `sshd`, scripts, unidad systemd y `sudoers`. Se puede correr dos veces. Los pasos que importan:

- **Copia los scripts con dueño root** (`install -o root`). Un `cp` sobre un archivo existente deja el dueño anterior.
- **Reactiva el servicio** (`systemctl unmask`) y lo habilita.
- **Deja SSH solo con llaves**, después de instalar las de `deploy/llaves/`, para que nadie quede afuera.

El procedimiento verificado para dejar el disco en multiconexión es el siguiente. `VBoxManage modifymedium --type multiattach` falla con un disco suelto (*can only be used on media registered with a machine*), así que primero hay que registrarlo en una VM:

```bash
mkdir -p "$HOME/VirtualBox VMs/httpaas-base" && mv ApacheServer.vdi "$HOME/VirtualBox VMs/httpaas-base/"
D="$HOME/VirtualBox VMs/httpaas-base/ApacheServer.vdi"
VBoxManage createvm --name apache-plantilla --ostype Debian_64 --register
VBoxManage storagectl apache-plantilla --name SATA --add sata --controller IntelAhci
VBoxManage storageattach apache-plantilla --storagectl SATA --port 0 --device 0 --type hdd --medium "$D"
VBoxManage storageattach apache-plantilla --storagectl SATA --port 0 --device 0 --medium none
VBoxManage modifymedium disk "$D" --type multiattach
```

Los dos primeros comandos colocan el disco en la carpeta del disco base. El tercero registra la VM `apache-plantilla` (solo si no existe). El cuarto crea su controlador SATA. El quinto conecta el disco, lo que lo registra con esa VM. El sexto lo desconecta. El séptimo cambia el tipo a `multiattach`. La VM queda registrada sin disco y sin encenderse nunca: es la «dueña» del disco base. Se espera que `VBoxManage showmediuminfo disk "$D"` muestre `Type: multiattach`.

Antes de subir el disco a la carpeta compartida se aplican dos comprobaciones (sección 8.7): `verificar-plantilla.sh` dentro de la VM, que debe terminar con «23 OK, 0 FALLA», y `probar-clon.sh` en el host, que debe terminar con «El clon funciona».

[IMAGEN C-09 — pie: `showmediuminfo` con `Type: multiattach`. Incluida en el documento entregado.]

### 8.6 Clonación manual y adaptador Go

**Procedimiento manual.** Es la especificación que automatiza el adaptador `vbox`. Variables: `NAME` (nombre de la VM), `IP`, `HOSTNAME`, `DOMAIN`, `DNS`, `HOSTONLY_NET` y `BASE_VDI` (ruta del disco multiconexión).

| # | Comando | Qué hace y qué se espera |
|---|---|---|
| 1 | `VBoxManage createvm --name "<NAME>" --ostype Debian_64 --register` | Crea y registra una VM vacía; aparece en `VBoxManage list vms` |
| 2 | `VBoxManage modifyvm "<NAME>" --memory 512 --cpus 1 --nic1 hostonly --hostonlyadapter1 "<HOSTONLY_NET>"` | Fija memoria, CPU y la red host-only |
| 3 | `VBoxManage storagectl "<NAME>" --name "SATA" --add sata --controller IntelAhci` | Crea el controlador SATA (AHCI) |
| 4 | `VBoxManage storageattach "<NAME>" --storagectl "SATA" --port 0 --device 0 --type hdd --medium "<BASE_VDI>"` | Conecta el disco base; VirtualBox crea el diferencial automáticamente |
| 5 | `VBoxManage guestproperty set "<NAME>" /httpaas/hostname "<HOSTNAME>"` y lo mismo con `/httpaas/ip`, `/httpaas/dominio` y `/httpaas/dns` | Deja los datos que leerá `httpaas-firstboot` |
| 6 | `VBoxManage startvm "<NAME>" --type headless` | Enciende la VM sin ventana |
| 7 | `VBoxManage guestproperty get "<NAME>" /httpaas/estado` | Consulta el estado; se espera `listo` cuando la VM terminó de configurarse |
| 8 | `VBoxManage controlvm "<NAME>" poweroff` y `VBoxManage unregistervm "<NAME>" --delete` | Apaga y destruye la instancia; el disco base no se toca |

Se comprobó el 2026-11-10 que, con el disco registrado como se indica en la sección 8.5, cada clon recibe su propio diferencial y `unregistervm --delete` borra solo ese diferencial: el disco base queda intacto.

[IMAGEN C-10 — pie: las dos VMs corriendo, con sus discos diferenciales en `VBoxManage list hdds`. Incluida en el documento entregado.]

[IMAGEN C-11 — pie: después de eliminar, el disco base intacto. Incluida en el documento entregado.]

**Adaptador `internal/publicar`.**

- `ValidarZip(ruta, límites)` es una función pura con `archive/zip`. Aplica las mismas reglas que el script y rechaza las entradas cuyo `filepath.IsLocal` sea falso. Devuelve un `Resumen` (archivos, bytes y raíz) y mensajes legibles para el usuario. Sus reglas se compararon con `unzip` 6.0 real, porque la web y la VM decidían distinto sobre algunos zips (sección 10.3).
- `Publicar(host, ip, zip)` genera un identificador aleatorio, copia el zip a `/home/httpaas/subidas/<id>.zip` y ejecuta `sudo /usr/local/sbin/httpaas-publicar` sobre esa ruta. Convierte el código 2 en un error de validación y el 3 en un error de servidor, y verifica con `GET http://<ip>/` y `Host: <host>.cloud.local` que el servidor responda 200.
- Los comandos remotos van solo por `executor.Executor` con argumentos separados. El adaptador pasó 22 de 22 mutaciones (sección 6.3).

### 8.7 Cómo se verificó y qué problemas tuvo

**Verificación.**

- Dentro de la VM, `verificar-plantilla.sh` ejecuta 23 comprobaciones: dueño root del script, sintaxis, igualdad con el del repositorio, servicio habilitado, SSH, `sudo` y plantilla limpia. Se espera «23 OK, 0 FALLA». Además, `md5sum` del script instalado y del del repositorio deben ser iguales, lo que confirma que no está vacío.
- En el host, `probar-clon.sh` hace exactamente lo que hará la app con cada clon. Toma un snapshot, escribe propiedades (`prueba`, `192.168.10.29`), enciende la VM y comprueba que el primer arranque llegue a `listo`, que tome la IP nueva, que SSH responda y que Apache sirva la página de `prueba.cloud.local`. Al terminar, apaga la VM, restaura el snapshot y borra las propiedades, y la plantilla queda como estaba. Se espera «El clon funciona: ya se puede subir el disco».
- El disco solo se sube si se cumplen ambas condiciones.
- Con el disco real, la Parte A creó y borró dos instancias y `ApacheServer.vdi` siguió en multiconexión y sin hijos. La prueba completa (sección 9) cubre además la publicación de punta a punta.

**Problemas.** Se detallan en la sección 10.3. Resumen: los tres primeros discos fallaron por scripts instalados a mano y no ejecutados antes de armar el disco. La corrección fue instalar solo con `instalar.sh` y agregar los dos pasos de verificación antes de subir el disco.

---

## 9. Prueba de punta a punta

`scripts/prueba-completa.sh` usa las mismas rutas que el navegador (incluida la cabecera `Origin`) y verifica cada efecto donde ocurre. Para ejecutarlo corre la webapp y el script la consulta con `curl`. Secuencia:

1. Crear `paisajes` y `negocios` y esperar a que las dos queden `lista`.
2. Comprobar el DNS dentro de `ns` (por SSH, así funciona igual en Windows): cada nombre resuelve a la IP que le dio la app.
3. Publicar `testdata/zips/paisajes.zip` y `negocios.zip` y esperar a que vuelvan a `lista` sin error.
4. Comprobar que Apache sirve exactamente el `index.html` de cada zip, comparando el `md5` del archivo del zip con el de la página servida con `Host: <nombre>.cloud.local`.
5. Comprobar los rechazos: nombre repetido (422), `zipslip.zip` (422), `symlink.zip` (422), nombre malicioso (422) y otro `Origin` (403).
6. Eliminar `paisajes` y comprobar que la VM no aparece en `VBoxManage list vms`, que el nombre ya no resuelve y que `ApacheServer.vdi` sigue existiendo.
7. Eliminar `negocios`.

El script imprime `OK`, `FALLA` u `omitido` por paso y termina con código distinto de 0 si algo falla.

| Modo | Fecha | Resultado |
|---|---|---|
| Simulado (`SIMULADO=1`, contra `make simular`) | 2026-11-10 | 17 OK, 0 FALLA, 3 omitidos (los que necesitan VMs) |
| Real, con `ns` y el disco de la Parte C | 2026-11-11 | **26 OK, 0 FALLA, 0 omitidos** |

[IMAGEN A-11 — pie: dos instancias (`paisajes` y `negocios`) en `lista`. Varias instancias a la vez, cada una con su IP.]

[IMAGEN A-12 — pie: la resolución de los dos nombres en `ns` y en el host. El registro DNS fue automático.]

[IMAGEN A-13 — pie: la página de `paisajes`, abierta desde su enlace en una pestaña nueva. El sitio del zip, publicado y servido por su nombre.]

[IMAGEN A-14 — pie: la página de `negocios` (el zip trae una carpeta). La regla de la carpeta única funciona de punta a punta.]

[IMAGEN A-15 — pie: el rechazo de `zipslip.zip` en el formulario. Trae la ruta `../../tmp/x`. Importa porque la validación protege antes de tocar la VM.]

[IMAGEN A-16 — pie: la salida de `scripts/prueba-completa.sh` en modo real: 26 OK, 0 FALLA. La prueba completa del enunciado, de una sola vez.]

---

## 10. Problemas encontrados

Se recogen los problemas que cambiaron el diseño o dejaron una prueba; no es un historial completo de incidencias.

### 10.1 Parte A

| Síntoma | Causa | Corrección | Prueba que lo cubre |
|---|---|---|---|
| VirtualBox no permite crear la red `192.168.10.0/24` en Linux | Sin `/etc/vbox/networks.conf`, solo se permite `192.168.56.0/21` | Configurar el archivo (sección 3.3) | `ip -br addr show vboxnet0` muestra `192.168.10.1/24` (P-01) |
| Eliminar podía borrar una VM ajena con el mismo nombre | `Eliminar` confiaba en el nombre de la VM | Revisar el nombre antes de reservar y marcar `/httpaas/hostname`: solo se borran VMs con esa marca | Prueba con una corrección de prueba que falla antes y mutación que la detecta (sección 6.3) |
| Un fallo de SSH podía contarse como éxito | Sin `ssh` en el `PATH` el error no es de salida, y un proceso muerto por tiempo deja -1 | Binario ausente, proceso muerto o código 255 son fallos de conexión | Prueba unitaria del ejecutor con corrección verificada por mutación |
| El flujo de control de calidad falló con `TempDir RemoveAll cleanup: directory not empty` | Cancelar el trabajador no esperaba el guardado en curso del JSON | `Iniciar` devuelve un canal que se cierra al terminar y la app lo espera | 3 fallas en 300 repeticiones antes de corregir; 0 en 300 después |
| `ns.ova` arranca sin la red del proyecto en otro PC | El `.ova` guarda el nombre de la red host-only del PC de origen | `scripts/importar-ns.sh` conecta el adaptador 1 a la red de cada PC | `scripts/verificar-ns.sh` pasó los chequeos |
| La web y la VM decidían distinto sobre algunos zips | Un `link/` simbólico pasaba como carpeta; duplicados y archivo y carpeta con el mismo nombre fallan en `unzip`; `./index.html` se rechazaba | Las mismas reglas que `httpaas-publicar`, comparadas con `unzip` 6.0 real | Pruebas de `ValidarZip` con zips generados en memoria y con `testdata/zips` |
| La VM `ns` quedó `aborted` | El disco del host llegó al 98 % | Liberar espacio (de 1.8 GB a 15 GB libres) y volver a encenderla | — |
| Una prueba pasaba en el PC y fallaba en la CI | `Reconciliar` llama a `VBoxManage` al arrancar y la CI no tiene VirtualBox | La prueba usa un `VBoxManage` falso | Verificada con un `PATH` sin VirtualBox |

### 10.2 Parte B

| Síntoma | Causa | Corrección | Prueba que lo cubre |
|---|---|---|---|
| `*.cloud.local` no resolvería en el host | `nsswitch.conf` envía `.local` a mDNS con `[NOTFOUND=return]` antes de `dns` | `scripts/dns-host-linux.sh` o regla NRPT en Windows | `getent hosts` o `Resolve-DnsName` (B-08) |
| `ns` no arranca: *no bootable option or device* | NVRAM vacío al crear la VM copiando el VDI | `grub-install --removable` | `ns` arrancó y respondió por SSH (B-02) |
| `grub-install: command not found` | Solo estaba el GRUB que arranca | `apt install grub-efi-amd64` | Se completó `grub-install` |
| `apt` falla con *Temporary failure resolving* | `enp0s8` en `DOWN` y `resolv.conf` con DNS heredados | `/etc/network/interfaces` con `enp0s3` estática sin gateway y `enp0s8` por DHCP | Desde `ns`, `ping -c1 deb.debian.org` responde (B-01) |
| `named-checkconf`: `unknown option 'update-privacy'` | Transcripción manual de `update-policy` | Corregir y validar de nuevo antes de reiniciar | `named-checkconf` sin errores (B-03) |
| `dig ns.cloud.local` devuelve `192.168.10.0` | El registro `A` se escribió con la dirección de red | Corregir a `.10` y subir el serial | `dig` devuelve `.10` (B-04) |
| `rndc reload cloud.local` falla con *dynamic zone* | BIND no recarga desde archivo una zona dinámica porque descartaría el journal | `systemctl restart named` (no había journal); la vía manual es `rndc freeze`, editar y `rndc thaw` | `dig` tras el reinicio |
| `apt` y `dig` sin `@servidor` fallan en `ns` | El DHCP entregó el DNS del host, inalcanzable desde la VM | `prepend domain-name-servers 127.0.0.1, 10.0.3.3;` en `dhclient.conf` y renovar el lease | `apt` y `dig` funcionan (B-04) |
| `listar` devuelve `prueba.cloud.local.` | El `sed` anclaba `$` al final de una línea que termina en la IP | Separar con `awk` y quitar el sufijo ahí | `listar` imprime `prueba 192.168.10.99` |
| `agregar ns` dice *invalid host* en vez de *reserved* | El regex de longitud rechazaba `ns` antes del chequeo de reservado | Mover el chequeo de `ns` antes del regex | `httpaas_dns_test.go` fija las reglas de reservados |
| `Resolve-DnsName -Server` no consulta otro DNS | NRPT se aplica antes si el nombre cae en el sufijo | Usar `nslookup` para consultar otro servidor | — |
| El adaptador reportaba éxito con un comando fallido | El ejecutor solo devuelve `error` para fallos de transporte | Revisar `ExitCode` y devolver el `stderr` | Prueba con `executor.Fake`: un código distinto de 0 es un error con el `stderr` |
| `make vm-test` se saltaba la prueba | No existe `config.json` (está en `.gitignore`) | Config temporal con `HTTPAAS_CONFIG` | `TestVMRegistrarResolverQuitar` pasa |
| El formulario no se alineaba y el móvil desbordaba | `max-width` en el panel y el `input[type=file]` nativo imponía su ancho | Panel a ancho completo, selector propio, `min-width: 0` y `minmax(0, 1fr)` | Revisión a 360 px reales (B-11) |

### 10.3 Parte C

| Síntoma | Causa | Corrección | Prueba que lo cubre |
|---|---|---|---|
| Disco 1: el primer arranque seguía siendo el viejo (`/httpaas/domain`) y de usuario `httpaas` | El script se corrigió en el repositorio, pero el disco se armó antes, copiando a mano | Instalar siempre con `instalar.sh` (`install -o root`) | `verificar-plantilla.sh`: dueño root y `md5` igual al del repositorio |
| Disco 2: una comilla de más (línea 120) y todos los clones en `.192` | La IP se sacó de la validación del DNS; el script nunca se ejecutó antes de armar el disco | Corregir los dos errores; ejecutar y verificar antes de armar el disco | `bash -n` sobre el script y `probar-clon.sh` |
| Disco 3: el script vacío (0 bytes) y el servicio `masked` | Se copió y se apagó a mano, sin correr `verificar-plantilla.sh` | `instalar.sh` ejecuta `systemctl unmask`; `md5sum` detecta el archivo vacío | `verificar-plantilla.sh` (23 OK) y `probar-clon.sh` |
| `VBoxManage modifymedium --type multiattach` falla con un disco suelto | El disco no estaba registrado con ninguna VM | Registrarlo primero en una VM, desconectarlo y después cambiar el tipo (sección 8.5) | Comprobado el 2026-11-10: cada clon recibe su diferencial y el base queda intacto |

**Qué se corrigió en el proceso.** Cada error del disco se descubría en otro PC una hora después de armarlo. Los pasos `verificar-plantilla.sh` y `probar-clon.sh` los atrapan en el PC de quien arma el disco, antes de compartirlo. Se agregaron también las reglas de trabajo del asistente de IA (sección 12).

---

## 11. Conocimientos aprendidos

**Parte A.** Aprendimos que acordar el contrato primero (las interfaces y sus Fakes) es lo que permite trabajar en paralelo: cada parte avanzó sin esperar las VMs de las otras y al juntar las tres no hubo conflictos. Aprendimos que una prueba que pasa no demuestra nada si no falla cuando el código está mal: al romper a propósito cada corrección encontramos pruebas que no protegían lo que decían. Aprendimos que las revisiones independientes encuentran lo que uno no ve en su propio código, y la más grave fue que Eliminar podía borrar una VM ajena. En VirtualBox aprendimos a no borrar snapshots con la VM encendida y a esperar a que libere la VM antes de seguir.

**Parte B.** Aprendimos que una zona DNS con actualizaciones dinámicas tiene dos estados, el archivo y la memoria del demonio, y que editar el archivo con `named` encendido es la forma más rápida de corromperla. Aprendimos que el sistema operativo del host tiene su propia jerarquía de resolución (`nsswitch.conf`, NRPT) y que `dig` puede decir «funciona» mientras el navegador no resuelve. En el dashboard aprendimos a probar con un viewport real y no confiar en una captura: el ancho nativo de un `input[type=file]` rompía toda la página en móvil.

**Parte C.** Aprendimos que una imagen base no se arma ni se corrige a mano: cada disco fallido venía de copiar scripts a mano sin ejecutarlos, y la solución fue instalar siempre con un script versionado y comprobar la imagen antes de compartirla. También aprendimos que el disco multiconexión debe estar registrado en una VM antes de cambiar su tipo y que eliminar un clon solo borra su diferencial.

---

## 12. Uso de IA

El equipo usó asistentes de IA como apoyo para escribir código, revisar cambios y redactar documentación. Cada parte tenía un bloque de instrucciones para el asistente, con reglas obligatorias. Las comunes fueron:

- usar solo la biblioteca estándar de Go y no interpolar datos del usuario en comandos (argumentos separados, nunca `sh -c`);
- no cambiar el contrato común sin acuerdo del equipo;
- trabajar por fases, en orden, y no avanzar hasta que la verificación de la fase pasara;
- ejecutar `make check` antes de cada commit;
- explicar en pocas líneas qué se va a hacer y por qué antes de escribir código o configuración;
- no inventar comandos de VBoxManage y verificarlos con `--help`.

La Parte C añadió reglas propias: no reinstalar la plantilla, instalar los scripts solo con `instalar.sh`, nunca editar el script instalado dentro de la VM, nunca usar `systemctl mask` y subir el disco solo si pasan `verificar-plantilla.sh` y `probar-clon.sh`.

La IA fue útil sobre todo para acelerar lo repetitivo (plantillas, scripts, pruebas de tabla y revisión de código) y para detectar errores que se escapan en una lectura rápida, como el ancho de un `input` o un `pipefail` que falla abierto. El criterio para aceptar una sugerencia fue siempre el mismo: que se pudiera comprobar. Cada hallazgo se verificó contra el código antes de corregirlo y cada corrección tiene una prueba que fallaba antes. Las sugerencias sobre DNS se contrastaron contra la VM `ns`, `dig`, `named-checkzone` y las pruebas. Ninguna captura ni resultado se inventó: las capturas salen de la ejecución real y los problemas se anotaron cuando ocurrieron. El diseño y las decisiones (D1 a D16) los tomó el equipo y se cambiaron en el plan primero.

---

## 13. Mejoras y trabajo futuro

| # | Mejora | Por qué |
|---|---|---|
| M1 | Gestión de llaves SSH desde la interfaz: pegar una llave pública o generar un par y descargar la privada una vez | Hoy la llave se genera al preparar el PC y cambiarla exige reconfigurar cada VM a mano |
| M2 | Que la webapp detecte y aprovisione `ns` si no existe (`VBoxManage import ns.ova`) | Hoy `ns` es plataforma previa creada a mano; el producto solo la consume y falla con un mensaje claro |
| M3 | Publicar en un solo paso desde el servidor (`SolicitarCrearYPublicar`): crear, esperar `lista` y publicar, persistiendo el zip | La vista ya lo hace con JavaScript, pero depende de que la pestaña siga abierta |
| M4 | Editar una instancia desde el dashboard | El plan no lo pide; hostname e IP no cambian sin recrear la VM, así que requiere una operación y una ruta nuevas |

Otras mejoras identificadas en el camino:

- Mover los estilos a `static/estilos.css` cuando se sirvan archivos estáticos.
- Cambiar el `<meta refresh>` por sondeo a `/api/instancias`, para no perder lo que el usuario escribe mientras hay una instancia ocupada.
- Mapear el error de publicar al campo del archivo con un mensaje como «primero crea la instancia con Aceptar».

---

## 14. Referencias

- Oracle. *VirtualBox Manual*: `VBoxManage storageattach`, `export`/`import`, `guestproperty`, `showmediuminfo`. https://www.virtualbox.org/manual/
- Internet Systems Consortium. *BIND 9 Administrator Reference Manual*: zonas primarias, `update-policy`, `nsupdate`, `rndc freeze/thaw`. https://bind9.readthedocs.io/
- RFC 1035 — *Domain Names: Implementation and Specification*.
- RFC 2136 — *Dynamic Updates in the Domain Name System (DNS UPDATE)*.
- RFC 1123 — *Requirements for Internet Hosts*.
- Debian. *Debian Administrator's Handbook*: red, `systemd-resolved`, `nsswitch.conf`.
- The Go Authors. *Package `net`*, *Package `html/template`*, *Package `os/exec`*. https://pkg.go.dev/
- MDN Web Docs. *`<dialog>`*, *`fetch`*, *`FormData`*. https://developer.mozilla.org/
- OWASP. *Zip Slip Vulnerability*. https://owasp.org/www-community/attacks/

---

## 15. Anexo A: inventario de imágenes

> En `docs/capturas/` están las imágenes de P, A y B. Las capturas C-01 a C-11 se incluyeron en el documento entregado, que se envía aparte.

| Código | Sección | Pie propuesto | Estado |
|---|---|---|---|
| P-01 | 3.3 | Red host-only `192.168.10.1/24` sin DHCP | Existe |
| docs/Arquitectura.png | 3.5 | Diagrama de la arquitectura | Existe |
| A-01 | 6.2 | `make check` en verde | Existe |
| A-02 | 6.2 | Flujo `check` en verde en GitHub | Existe |
| A-03 | 6.2 | Línea de `operaciones.jsonl` | Existe |
| A-04 | 6.2 | Dashboard simulado con `paisajes` en `creando` | Existe |
| A-05 | 6.2 | Nombre `a;rm` rechazado (422) | Existe |
| A-06 | 6.2 | `VBoxManage list vms` con dos instancias | Existe |
| A-07 | 6.2 | Disco base con dos hijos diferenciales | Existe |
| A-08 | 6.2 | Tras eliminar: VM ausente y `.vdi` presente | Existe |
| A-09 | 6.2 | Formulario con `paisajes` y `paisajes.zip` | Existe |
| A-10 | 6.2 | Instancias `lista` con enlaces | Existe |
| A-11 | 9 | Dos instancias en `lista` | Existe |
| A-12 | 9 | Resolución en `ns` y en el host | Existe |
| A-13 | 9 | Página de `paisajes` | Existe |
| A-14 | 9 | Página de `negocios` | Existe |
| A-15 | 9 | Rechazo de `zipslip.zip` | Existe |
| A-16 | 9 | Salida de la prueba completa (26 OK) | Existe |
| B-01 | 7.2 | Interfaces de `ns` | Existe |
| B-02 | 7.2 | SSH sin contraseña | Existe |
| B-03 | 7.3 | `named-checkconf` y `named-checkzone` | Existe |
| B-04 | 7.3 | Zona directa, inversa y recursión | Existe |
| B-05 | 7.4 | Alta dinámica y `dig` | Existe |
| B-06 | 7.4 | Rechazo de `x;id` y de `sudo cat` | Existe |
| B-07 | 7.4 | Archivos `.jnl` | Existe |
| B-08 | 7.5 | El host resuelve `cloud.local` | Existe |
| B-09 | 7.7 | Dashboard vacío | Existe |
| B-10 | 7.7 | Dos instancias en el dashboard | Existe |
| B-11 | 7.7 | Dashboard a 400 px | Existe |
| B-12 | 7.7 | Banner con `ns` apagada | Existe |
| C-01 | 8.2 | `showvminfo` de `apache-plantilla` | En el entregado |
| C-02 | 8.2 | Página por defecto de Apache | En el entregado |
| C-03 | 8.2 | Guest properties en ambas direcciones | En el entregado |
| C-04 | 8.3 | `systemctl status httpaas-firstboot` | En el entregado |
| C-05 | 8.3 | Estado `listo` visto desde el host | En el entregado |
| C-06 | 8.3 | Página provisional con el `Host` correcto | En el entregado |
| C-07 | 8.4 | Publicación correcta | En el entregado |
| C-08 | 8.4 | Rechazo de `zipslip.zip` | En el entregado |
| C-09 | 8.5 | `showmediuminfo` con `Type: multiattach` | En el entregado |
| C-10 | 8.6 | Dos VMs corriendo con sus diferenciales | En el entregado |
| C-11 | 8.6 | Disco base intacto tras eliminar | En el entregado |

---

## 16. Glosario

| Término | Significado |
|---|---|
| AHCI / SATA | Controlador de disco que usan la plantilla y los clones (D16) |
| Dashboard | Página principal de la webapp, con el formulario y la lista de instancias |
| FQDN | Nombre completo, por ejemplo `paisajes.cloud.local` |
| Guest properties | Propiedades que el host deja en una VM y que ella puede responder |
| Headless | VM encendida sin ventana |
| Host | El PC anfitrión donde corren la webapp y VirtualBox |
| Instancia | Una VM Apache creada por el servicio |
| Journal (`.jnl`) | Archivo con los cambios dinámicos de una zona DNS |
| NRPT | Regla de Windows que dirige un sufijo DNS a un servidor concreto |
| Puertos y adaptadores | Organización en la que el centro solo conoce interfaces y cada parte implementa una |
| `VBoxManage` | Comando de VirtualBox para controlar VMs y discos |
