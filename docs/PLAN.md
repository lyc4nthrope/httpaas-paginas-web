# Plan — Parcial 3: HTTP como servicio (HTTPaaS)

**Equipo:** Cristhian Osorio (Parte A) · Daniel Pérez (Parte B) · Wiliam Carmona (Parte C)
**Entrega:** miércoles 11 de noviembre de 2026, 11:59 pm
**Fuente:** [`docs/enunciado/Enunciado_parcial3.pdf`](enunciado/Enunciado_parcial3.pdf) y [`Imagenes_enunciado_parcial3.pdf`](enunciado/Imagenes_enunciado_parcial3.pdf)

> **Cómo leer esto.** Todos leen este archivo completo una vez. Después, cada uno trabaja solo con **su** plan:
> [Parte A](parte-a/PLAN.md) · [Parte B](parte-b/PLAN.md) · [Parte C](parte-c/PLAN.md).
> Cada plan de parte empieza con un bloque **"Para el asistente de IA"** que se pega tal cual al inicio de la conversación con la IA.
> Una fase está **terminada (✅)** solo si su verificación pasa contra la VM real, tiene sus capturas y tiene commit.

---

## 1. Qué pide el enunciado y qué falta

El enunciado pide una **mini nube local**. El usuario entra a una web en Go, escribe un nombre de host y sube un `.zip`. El sistema, sin intervención humana, crea una VM Apache a partir de un disco multiconexión, la registra en el DNS (Bind9) y publica el contenido en `http://<nombre>.cloud.local`.

| # | Requisito del enunciado | Estado actual | Qué falta (dueño) |
|---|---|---|---|
| R1 | Arquitectura: red local, dominio, zona DNS (§1) | Lab DNS hecho con otra red (`intnet`, `dns-*`) | Red host-only `192.168.10.0/24` en cada PC (todos, H0) y diagrama (C) |
| R2 | Bind9 en VM dedicada, zona `cloud.local.` (§2) | Lab DNS: sabemos montar zonas a mano | VM `ns` 192.168.10.10 con zona dinámica (B) |
| R3 | Registro **automático** de hosts en la zona (§2) | — | `update-policy local` + script `httpaas-dns` + adaptador Go (B) |
| R4 | Debian 13 CLI con Apache, acceso remoto y red (§3) | — | VM plantilla `apache-plantilla` (C) |
| R5 | Cada instancia con hostname único, directorio propio y su zip (§3) | — | Script de primer arranque y script de publicación (C) |
| R6 | Disco **multiconexión** como plantilla (§3) | Lab #4: procedimiento con la GUI | `ApacheServer.vdi` en multiconexión y procedimiento por CLI (C) |
| R7 | Webapp Go: recibir solicitud, crear VM, actualizar DNS, desplegar, dashboard (§4) | Parcial 2: consola Go con `Executor` SSH, validación y registro | Orquestador y webapp nueva (A), dashboard (B), publicación (C) |
| R8 | Eliminar instancias (objetivo específico 2 + botón "Eliminar" de la figura) | — | Borrar VM, registro DNS y estado (A + B) |
| R9 | Todo sin interacción del usuario (§5) | Parcial 2: SSH con `BatchMode`, sin contraseñas | Reutilizar ese enfoque (A) |
| R10 | Código, documento con capturas paso a paso, problemas, mejoras y aprendizajes | Formato del Parcial 2 | Capturas por fase (todos) y documento consolidado (B) |

**Recursos del host (Cristhian):** 15 GB de RAM y ~13 GB libres en disco. Cada instancia usa 512 MB de RAM, y su disco diferencial empieza casi vacío y crece con lo que la VM escribe (se mide en C6). Por eso hay un **máximo de 3 instancias simultáneas** (configurable).

**Problemas ya detectados en el host (hay que resolverlos en H0):**
1. No existe `/etc/vbox/networks.conf`. En Linux, VirtualBox 7 solo deja crear redes host-only dentro de `192.168.56.0/21`, así que `192.168.10.x` falla si no se configura.
2. `/etc/nsswitch.conf` tiene `mdns4_minimal [NOTFOUND=return]` antes de `dns`. Todo nombre `*.local` se va a mDNS y nunca llega a Bind9, así que el navegador no abriría `paisajes.cloud.local`.

---

## 2. Arquitectura

```
                       HOST (PC de cada integrante) — 192.168.10.1 en vboxnet0
 ┌────────────────────────────────────────────────────────────────────────────┐
 │  Navegador ──http──► Webapp Go (127.0.0.1:8080)                             │
 │                       │  cola de trabajos (1 a la vez)                      │
 │                       ├─ VBoxManage ─────────────► VirtualBox               │
 │                       ├─ ssh httpaas@192.168.10.10 ► sudo httpaas-dns ...    │
 │                       └─ scp/ssh httpaas@<ip> ─────► sudo httpaas-publicar   │
 └──────────────────────────────┬─────────────────────────────────────────────┘
                                │ red host-only vboxnet0  192.168.10.0/24
       ┌────────────────────────┼─────────────────────────────┐
       ▼                        ▼                             ▼
 VM "ns" (Bind9)          VM "paisajes" (Apache)       VM "negocios" (Apache)
 192.168.10.10            192.168.10.30                192.168.10.31
 zona cloud.local.        disco diferencial ─┐         disco diferencial ─┐
 A paisajes → .30                            └──── ApacheServer.vdi ◄─────┘
 A negocios → .31                                  (multiconexión, solo lectura)
```

Todas las VMs tienen además un segundo adaptador **NAT**, solo para salir a internet (`apt`). Toda la comunicación del proyecto va por la red host-only.

**Plan de direcciones (fijo, igual en los tres PCs):**

| IP | Quién |
|---|---|
| 192.168.10.1 | Host (interfaz `vboxnet0`) |
| 192.168.10.10 | VM `ns` (Bind9) |
| 192.168.10.20 | VM `apache-plantilla`, solo mientras se prepara (después queda sin disco) |
| 192.168.10.30 – .99 | Instancias Apache, asignadas por la webapp |

### 2.1 Flujo "Aceptar" (crear instancia)

1. La webapp valida el nombre (`paisajes`): formato, nombre reservado, que no esté repetido y que no supere el máximo de instancias.
2. Guarda la instancia en estado `creando` con la siguiente IP libre (`.30`) y responde de inmediato. El dashboard se refresca solo.
3. La cola ejecuta: `VBoxManage createvm` → red → disco multiconexión → *guest properties* (`/httpaas/hostname`, `/httpaas/ip`…) → `startvm --type headless`.
4. Dentro de la VM, el servicio `httpaas-firstboot` lee esas propiedades y se configura solo: hostname, IP, llaves SSH nuevas, `machine-id` nuevo, VirtualHost y directorio `/var/www/paisajes`. Al terminar escribe `/httpaas/estado = listo`.
5. La webapp espera `listo` (`VBoxManage guestproperty wait`) y comprueba el puerto 22.
6. Registra el nombre en DNS: `ssh httpaas@192.168.10.10 sudo httpaas-dns agregar paisajes 192.168.10.30`.
7. Verifica que `paisajes.cloud.local` resuelva a `.30` consultando a 192.168.10.10, y marca la instancia como `lista`.

### 2.2 Flujo "Publicar" (subir contenido)

1. La webapp recibe nombre y `.zip` (tamaño máximo configurable) y lo **valida en el host**: sin rutas `..`, sin rutas absolutas, sin enlaces simbólicos, con límite de archivos y de tamaño descomprimido, y con `index.html` en la raíz o dentro de una única carpeta.
2. `scp` del zip a `/home/httpaas/subidas/<id>.zip` en la instancia.
3. `ssh ... sudo httpaas-publicar /home/httpaas/subidas/<id>.zip`: descomprime en una carpeta temporal, comprueba `index.html`, cambia la carpeta de forma atómica y recarga Apache.
4. La webapp comprueba `GET http://<ip>/` con cabecera `Host: paisajes.cloud.local` → 200.

### 2.3 Flujo "Eliminar"

1. Marca la instancia como `eliminando`.
2. `httpaas-dns quitar paisajes`.
3. `VBoxManage controlvm paisajes poweroff` → espera a que se apague → `VBoxManage unregistervm paisajes --delete` (borra solo el disco diferencial).
4. Quita la llave de la instancia de `keys/known_hosts` y la borra del estado. La IP vuelve a quedar libre.
5. Comprueba que `ApacheServer.vdi` **sigue existiendo**.

Si un paso falla, la instancia queda en estado `error` con el detalle en el registro, y el botón **Eliminar** limpia lo que se haya alcanzado a crear.

---

## 3. Decisiones técnicas

Cada una es una recomendación. Si el equipo cambia alguna, se cambia aquí primero.

| # | Decisión | Por qué |
|---|---|---|
| D1 | La webapp corre **en el host** | Solo ahí existe `VBoxManage`. Es el "plano de control", como la consola de AWS |
| D2 | Red **host-only** `vboxnet0` 192.168.10.0/24 sin servidor DHCP, más un adaptador **NAT** en cada VM | El host (navegador y webapp) llega a las VMs con IPs fijas que no dependen del wifi de la universidad. Sin DHCP no hay choques de IP. El NAT da internet para `apt` |
| D3 | Cada clon se configura solo con **guest properties + un servicio de primer arranque** (`httpaas-firstboot`) | Es la misma idea que el *user data* de una instancia EC2, que cloud-init ejecuta solo en el primer arranque. La webapp deja los datos antes de arrancar y no necesita entrar a una VM que todavía tiene la IP de otra. Funciona aunque se creen varias seguidas |
| D4 | La webapp procesa los trabajos de a **uno a la vez** (una cola) | Evita carreras al asignar IPs y al registrar el disco multiconexión en VirtualBox. Crear tarda ~1 min, así que no hace falta más |
| D5 | DNS dinámico con `update-policy local` y **`nsupdate -l` ejecutado dentro de `ns`** por un script fijo (`httpaas-dns`) | No hay que instalar herramientas de BIND en el host (sirve en Windows) ni copiar llaves TSIG. Basta con SSH |
| D6 | Las zonas viven en `/var/lib/bind/` y nunca se editan a mano con el servidor encendido (`rndc freeze/thaw`) | Con actualizaciones dinámicas, Bind escribe un journal `.jnl`. Editar el archivo a mano lo corrompe. AppArmor solo deja escribir en `/var/lib/bind` |
| D7 | Acceso remoto con el usuario **`httpaas` y llaves**, sin contraseñas. `sudo` permitido **solo** para `httpaas-dns` o `httpaas-publicar` | Mínimo privilegio: aunque alguien robe la llave, solo puede ejecutar esos dos scripts, que validan sus argumentos |
| D8 | SSH con `BatchMode=yes`, `StrictHostKeyChecking=accept-new` y un `keys/known_hosts` propio de la app. Al eliminar una instancia se ejecuta `ssh-keygen -R` | Cada clon genera llaves de host nuevas. `accept-new` fija la llave la primera vez y detecta cambios después. Sin `-R`, la siguiente VM con la misma IP sería rechazada |
| D9 | Datos del usuario **siempre** como argumentos separados (`exec.Command(name, args...)`); nunca concatenados en un `sh -c` | Evita inyección de comandos (`paisajes; rm -rf /`). Fue el error principal del Parcial 2 |
| D10 | Nombre de host: `^[a-z][a-z0-9-]{1,30}[a-z0-9]$`; reservados `ns`, `www`, `apache-plantilla`, `localhost`; el nombre de la VM es el mismo hostname | Es una etiqueta DNS válida (RFC 1123). Usar el mismo nombre para VM y host es lo que muestra la figura del profesor |
| D11 | El zip se valida **en el host, en Go**, antes de enviarlo, y `httpaas-publicar` vuelve a validar | Defensa en dos capas contra *zip slip* (`../../etc/passwd`), enlaces simbólicos y bombas zip |
| D12 | Estado en `data/instancias.json`, escrito de forma atómica (archivo temporal + `rename`). Al arrancar, se compara con `VBoxManage list vms` | Sin base de datos, y sobrevive a reinicios. La comparación detecta VMs borradas a mano (estado `perdida`) |
| D13 | Arquitectura de **puertos y adaptadores**: el servicio `instancias` solo conoce las interfaces `Hipervisor`, `DNS` y `Publicador`, y cada una tiene una versión **Fake** | Los tres trabajan en paralelo desde el día 1 sin esperar las VMs reales, y las pruebas corren sin VirtualBox |
| D14 | La configuración de `ns` y de la plantilla se hace con **scripts versionados** (`deploy/ns/instalar.sh`, `deploy/plantilla/instalar.sh`) | Cualquiera reconstruye las VMs igual en su PC. Además es "infraestructura como código", buen material para el documento |
| D15 | Interfaz con `html/template` en un solo binario (`go:embed`), sin frameworks de frontend | Igual que en el Parcial 2. Menos piezas, menos errores |
| D16 | Firmware **BIOS** y controlador **SATA (AHCI)** en la plantilla y en los clones, ambos en `config.json` | El clon debe tener el mismo firmware y controlador que la VM donde se instaló el disco, o no arranca |

---

## 4. Contrato común (se acuerda en H0, antes de repartir)

Es el único punto donde las tres partes se tocan. **Nadie lo cambia solo:** un cambio al contrato requiere un PR a `dev` aprobado por los otros dos.

### 4.1 Estructura del repositorio y dueño de cada carpeta

```
httpaas-paginas-web/
├── cmd/httpaas/main.go              A   arranque, config, inyección de dependencias
├── internal/
│   ├── dominio/                     A   Instancia, Estado, ValidarHostname (contrato, H0)
│   ├── puertos/                     A   interfaces Hipervisor, DNS, Publicador + fakes (contrato, H0)
│   ├── config/                      A   carga y validación de config.json
│   ├── executor/                    A   SSH no interactivo (reutilizado del Parcial 2)
│   ├── oplog/                       A   registro JSON por operación (reutilizado del Parcial 2)
│   ├── almacen/                     A   data/instancias.json + asignación de IPs
│   ├── instancias/                  A   servicio orquestador + cola
│   ├── vbox/                        A   adaptador Hipervisor (VBoxManage)
│   ├── dns/                         B   adaptador DNS (ssh → httpaas-dns) + verificación
│   ├── publicar/                    C   validación del zip + adaptador Publicador
│   └── web/                         A   servidor y rutas;  B: templates/dashboard.html y static/
├── deploy/
│   ├── llaves/<nombre>.pub          todos  llave pública SSH de cada integrante (nunca la privada)
│   ├── ns/                          B   instalar.sh, named.conf.*, db.*, httpaas-dns, sudoers
│   └── plantilla/                   C   instalar.sh, httpaas-firstboot(.service), httpaas-publicar, vhost, sudoers
├── scripts/                         A   prueba-completa.sh;  B: dns-host-linux.sh
├── testdata/zips/                   C   zips de ejemplo (paisajes, negocios) y zips maliciosos de prueba
├── docs/                            plan, partes, capturas/, diagrama (C), documento final (B)
├── config.example.json              A
└── Makefile                         A   build, run, simular, test, check
```

### 4.2 Interfaces Go (paquete `internal/puertos`)

```go
package puertos

// Hipervisor crea, consulta y elimina las VMs de las instancias. Lo implementa internal/vbox (A).
type Hipervisor interface {
    // Crear registra la VM sobre el disco multiconexión, escribe las guest properties y la arranca.
    Crear(ctx context.Context, spec SpecVM) error
    // EsperarLista bloquea hasta que /httpaas/estado == "listo" o vence ctx; si es "error:*", devuelve ese error.
    EsperarLista(ctx context.Context, nombre string) error
    // Eliminar apaga (si está encendida) y borra la VM con su disco diferencial. Si no existe, no es error.
    Eliminar(ctx context.Context, nombre string) error
    // Listar devuelve los nombres de las VMs registradas en VirtualBox.
    Listar(ctx context.Context) ([]string, error)
}

type SpecVM struct {
    Nombre  string // = hostname, ya validado
    IP      string // "192.168.10.30"
    Prefijo int    // 24
}

// DNS registra y quita registros A en la zona. Lo implementa internal/dns (B).
type DNS interface {
    Agregar(ctx context.Context, host, ip string) error // idempotente: reemplaza si ya existe
    Quitar(ctx context.Context, host string) error      // idempotente: no falla si no existe
    Resolver(ctx context.Context, host string) (string, error) // consulta directa a 192.168.10.10
}

// Publicador despliega un zip ya validado en una instancia. Lo implementa internal/publicar (C).
type Publicador interface {
    Publicar(ctx context.Context, host, ip, zipLocal string) error
}
```

Cada interfaz tiene un `Fake` en `internal/puertos/fakes.go`, que guarda las llamadas y puede devolver un error programado. **Ese archivo se escribe en H0.**

### 4.3 Guest properties (contrato entre A y C)

| Propiedad | La escribe | Valor de ejemplo | Para qué |
|---|---|---|---|
| `/httpaas/hostname` | Host (A), antes de `startvm` | `paisajes` | Hostname y VirtualHost |
| `/httpaas/ip` | Host (A) | `192.168.10.30/24` | IP fija de `enp0s3` |
| `/httpaas/dominio` | Host (A) | `cloud.local` | `ServerName paisajes.cloud.local` |
| `/httpaas/dns` | Host (A) | `192.168.10.10` | Resolver de la instancia |
| `/httpaas/estado` | VM (C, `httpaas-firstboot`) | `configurando` → `listo` o `error:<detalle>` | El host sabe cuándo puede seguir |

### 4.4 Scripts dentro de las VMs (contrato entre A, B y C)

| Script (en la VM) | Uso exacto | Salida | Dueño |
|---|---|---|---|
| `/usr/local/sbin/httpaas-dns` (en `ns`) | `sudo httpaas-dns agregar <host> <ip>` · `quitar <host>` · `listar` | Código 0 si salió bien; mensaje en stderr si no. `listar` imprime `host ip` por línea | B |
| `/usr/local/sbin/httpaas-publicar` (en cada instancia) | `sudo httpaas-publicar /home/httpaas/subidas/<id>.zip` | Código 0 y `publicado <n> archivos`; códigos 2 (zip inválido) y 3 (falló Apache) | C |

Los dos scripts **validan otra vez** sus argumentos con las mismas expresiones regulares que Go (D10).

### 4.5 `config.example.json`

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

En Windows, `red_hostonly` es `VirtualBox Host-Only Ethernet Adapter` y `vboxmanage` es la ruta completa a `VBoxManage.exe`.

### 4.6 Estados de una instancia

`creando` → `lista` → (`publicando` → `lista`) → `eliminando` → (se borra). Cualquier paso puede terminar en `error`. Una instancia que está en el JSON pero no existe en VirtualBox pasa a `perdida`. Desde `error` y `perdida` solo se puede **Eliminar**.

---

## 5. Reparto en tres partes

Puntos de esfuerzo: 1 = pocas horas; 3 = lo más difícil de la parte. Cada parte suma **11**.

| Parte | Persona | Tema | Pieza difícil | Funcionalidades | Cierre | Total |
|---|---|---|---|---|---|---|
| **A** | Cristhian | Plataforma y orquestación | Orquestador + adaptador VirtualBox (5) | Base web, executor, almacén (4) | Integración y prueba completa (2) | 11 |
| **B** | Daniel | DNS y experiencia de usuario | VM `ns` + zona dinámica (4) | Adaptador DNS, resolución en el host, dashboard (6) | Documento final (1) | 11 |
| **C** | Wiliam | Plantilla Apache y publicación | Plantilla + primer arranque + multiconexión (5) | Publicación del zip (Go y script) (4) | Diagrama y zips de demostración (2) | 11 |

### Dependencias entre partes

```
H0 contrato (todos)
 ├── A: web + orquestador con Fakes ─────────────────────────┐
 ├── B: VM ns + httpaas-dns ── adaptador dns ─────────────────┤
 └── C: plantilla + firstboot ── multiconexión + clon-manual.md ┤ (A usa clon-manual.md como especificación de vbox)
                                  └── adaptador publicar ─────┤
                                                              ▼
                                                   H3 integración en dev (A)
```

Lo único que bloquea a otro: **C debe entregar `docs/parte-c/clon-manual.md` y el `.vdi` (H2)** para que A pruebe el adaptador VirtualBox con VMs reales. Mientras tanto, A trabaja con `FakeHipervisor`.

---

## 6. Cronograma

Hoy es jueves 29 de octubre. Hay 13 días.

| Hito | Fecha | Qué debe estar listo (verificable) |
|---|---|---|
| **H0** | vie 30 oct | ✅ Equipo registrado en Classroom (29 oct). Los tres: red host-only funcionando, llave SSH generada con su `.pub` en `deploy/llaves/`, Go 1.24 instalado. A: contrato (§4) en `dev` y `make check` en verde |
| **H1** | lun 2 nov | A: `make simular` crea, publica y elimina con Fakes desde el navegador. B: `dig @192.168.10.10 ns.cloud.local` responde y `httpaas-dns agregar` funciona por SSH. C: plantilla que se reconfigura sola con guest properties (probado con snapshot) |
| **H2** | mié 4 nov | C (**en la mañana**): `ApacheServer.vdi` en multiconexión, `clon-manual.md` en `dev` y `.vdi` compartido con el equipo. B: adaptador `dns` con pruebas y prueba en VM. A: argumentos de `vbox` con pruebas (desde H1) comparados con `clon-manual.md`; prueba con VM real si el documento llegó |
| **H3** | vie 6 nov | A: `vbox` crea y elimina una instancia real (si no se hizo en H2). Integración en `dev` (A): crear → DNS → publicar → dashboard → eliminar, de punta a punta en un PC. B: dashboard final. C: adaptador `publicar` integrado |
| **H4** | dom 8 nov | `scripts/prueba-completa.sh` pasa en los **tres** PCs. Todas las capturas tomadas. Errores corregidos |
| **H5** | mar 10 nov | Documento final (B) revisado por los tres. `dev` → `main` y etiqueta `v1.0` |
| Entrega | mié 11 nov | Día de reserva. Subir código y documento antes de las 11:59 pm |

---

## 7. Preparación de cada PC (los tres, H0)

Cada paso tiene su verificación. No se sigue al siguiente si la verificación falla.

**P1. Permitir la red 192.168.10.0/24 en VirtualBox (solo Linux y macOS):**
```bash
sudo mkdir -p /etc/vbox
echo '* 192.168.10.0/24 192.168.56.0/21' | sudo tee /etc/vbox/networks.conf
```
Verificación: `cat /etc/vbox/networks.conf` muestra la línea.

**P2. Crear la red host-only sin DHCP:**
```bash
VBoxManage hostonlyif create                      # crea vboxnet0 (anota el nombre que imprime)
VBoxManage hostonlyif ipconfig vboxnet0 --ip 192.168.10.1 --netmask 255.255.255.0
VBoxManage dhcpserver remove --interface vboxnet0 2>/dev/null || true
```
En Windows: Administrador de red de VirtualBox → Host-only → adaptador con IP 192.168.10.1/24 y DHCP desactivado.
Verificación: `ip -br addr show vboxnet0` muestra `192.168.10.1/24`. En Windows, `ipconfig` muestra esa IP en el adaptador host-only.
📸 **P-01** — salida de la verificación.

**P3. Llave SSH de la webapp (una por persona):**
```bash
mkdir -p keys && ssh-keygen -t ed25519 -N '' -C "httpaas-<nombre>" -f keys/httpaas_ed25519
cp keys/httpaas_ed25519.pub deploy/llaves/<nombre>.pub   # <nombre>: cristhian, daniel o wiliam
```
Verificación: `git status` muestra `deploy/llaves/<nombre>.pub` y **no** muestra `keys/`.

**P4. Herramientas:** `go version` ≥ 1.24, `VBoxManage --version` 7.2.x y `ssh -V`.

**P5. Sistema operativo de cada uno.** En H0, cada uno escribe en el PR del contrato qué sistema usa. Si alguien usa **Windows**:

| En Linux | Equivalente en Windows | Por qué |
|---|---|---|
| P1 (`networks.conf`) | No hace falta | La restricción de rangos solo existe en Linux y macOS |
| `make check` | En PowerShell: `gofmt -l .` (debe salir vacío) → `go vet ./...` → `go test ./...` → `go build ./cmd/httpaas` | Windows no trae `make`. `go test -race` exige CGO y un compilador C, así que en Windows se corre sin `-race`; la prueba con `-race` la hace la CI de GitHub (A0) en cada PR |
| `make vm-test` | `go test -tags vm -count=1 ./...` | Lo mismo, sin `make` |
| `scripts/*.sh` en el host | Ejecutarlos con **Git Bash** (viene con Git for Windows) | Son scripts bash |
| `VBoxManage` | `"C:\Program Files\Oracle\VirtualBox\VBoxManage.exe"` (ponerlo en `config.json`, `vbox.vboxmanage`) | No está en el `PATH` por defecto |
| `ssh`, `scp`, `ssh-keygen` | Vienen con Windows 10/11 (OpenSSH). Comprobar con `ssh -V` | — |
| Resolver `*.cloud.local` | Regla NRPT (fase B5, punto 2) | Windows no usa `nsswitch.conf` |

Los scripts que corren **dentro** de las VMs (`httpaas-dns`, `httpaas-firstboot`, `httpaas-publicar`) no cambian: las VMs son Debian en cualquier host. Antes de subirlos desde Windows, revisar que tengan finales de línea LF (el `.gitattributes` del repo lo fuerza), porque con CRLF bash falla con `$'\r': command not found`.

La resolución de `*.cloud.local` en el host (el problema de `nsswitch`) la resuelve **B** en su fase B5 y la documenta para los tres.

---

## 8. Reglas de trabajo

**Git (cinco ramas, igual que en el Parcial 2):**

| Rama | Para qué |
|---|---|
| `cristhian`, `daniel`, `wiliam` | Trabajo diario; solo la toca su dueño |
| `dev` | Integración. Se llega solo por **Pull Request** con `make check` en verde |
| `main` | Versión entregable. Recibe `dev` solo en H5 |

- Antes de cada fase: `git switch <tu-rama> && git pull origin dev`.
- Al terminar una fase verificada: `git push` y PR de `<tu-rama>` hacia `dev`. A revisa e integra.

**Código:**
1. `make check` en verde antes de **cada** commit (gofmt, vet, pruebas con `-race` y build). Si falla, no se sube. En Windows, la secuencia equivalente de P5.
2. Nunca armar comandos con texto del usuario (D9). Los argumentos van separados en `exec.Command`, o como `$1`, `$2` de un script fijo.
3. Todo dato del formulario se valida **antes** de ejecutar algo. Si es inválido, se responde 422 con un mensaje claro y no se ejecuta nada.
4. Toda función que genera texto o configuración (vhost, nsupdate, argumentos) es **pura** y tiene prueba de tabla.
5. Commits convencionales en inglés que digan qué cambió: `feat(dns): register A records through httpaas-dns`. Nunca mensajes como `f`, `cambios` o `no se xd`.
6. Nunca se suben `keys/`, `data/`, `logs/`, `config.json`, `*.vdi` ni llaves privadas (ya están en `.gitignore`).
7. Las pruebas que necesitan una VM real llevan la etiqueta de compilación `vm` (`//go:build vm`), y se corren con `make vm-test`.

**Lecciones del Parcial 2 que no se repiten:** un commit que no compilaba bloqueó a todos; los formularios enviaban nombres de campo distintos a los que leía el código; hubo inyección de comandos con `echo '<dato>' > archivo`; un archivo de configuración se reemplazaba sin leerlo primero. Las reglas 1 a 4 existen por eso.

---

## 9. Capturas para el documento

El enunciado pide "capturas de pantalla paso a paso". Cada fase de cada plan tiene sus capturas numeradas (📸 `A-03`, `B-05`…). Se guardan en `docs/capturas/<código>.png` el mismo día en que se hace la fase. Si se dejan para el final, las VMs ya no estarán en ese estado.

| Sección del documento | Capturas |
|---|---|
| 1. Arquitectura y red | P-01, C-12 (diagrama) |
| 2. Servidor DNS Bind9 | B-01 … B-08 |
| 3. Plantilla Apache y disco multiconexión | C-01 … C-11 |
| 4. Aplicación web de gestión | A-01 … A-10, B-09 … B-11 |
| 5. Prueba de punta a punta | A-11 … A-16 |
| 6. Problemas, mejoras y trabajo futuro | — (cada uno anota en `docs/problemas.md` cuando le pasa algo) |
| 7. Aprendizajes | — (cada uno escribe su párrafo en H5) |

---

## 10. Prueba completa (H4, en los tres PCs)

`scripts/prueba-completa.sh` (lo escribe A) hace, con la webapp corriendo:

1. Crear `paisajes` y `negocios` → esperar que ambas queden en `lista`.
2. `dig @192.168.10.10 +short paisajes.cloud.local` → `192.168.10.30`.
3. Publicar `testdata/zips/paisajes.zip` y `negocios.zip`.
4. `curl -s http://paisajes.cloud.local/ | grep -q '<título esperado>'`, y lo mismo con `negocios`.
5. Intentar crear `paisajes` otra vez → 422. Subir `testdata/zips/zipslip.zip` → 422.
6. Eliminar `paisajes` → la VM ya no está en `VBoxManage list vms`, `dig` no devuelve nada y `ApacheServer.vdi` sigue existiendo.
7. Eliminar `negocios`.

Imprime `OK`/`FALLA` por paso y termina con código distinto de 0 si algo falló.

---

## 11. Riesgos y plan B

| Riesgo | Señal | Plan B |
|---|---|---|
| Las Guest Additions no compilan en Debian 13 | `VBoxControl` no existe en la plantilla | Plan B de D3: la plantilla arranca con la IP de mantenimiento `.20`; la webapp crea los clones de a uno, entra por SSH a `.20`, aplica la configuración y reinicia. La cola única (D4) hace que esto sea seguro |
| `*.cloud.local` no resuelve en el navegador del host | `getent hosts ns.cloud.local` vacío | Usar a `ns` como DNS principal del host, con recursión y forwarders a internet (B4). Último recurso: el dashboard también muestra la IP |
| `unregistervm --delete` borra el disco base | `ApacheServer.vdi` desaparece en la prueba de A6 | Respaldo de C5 (`clonemedium`). Cambiar Eliminar por: quitar el disco → `closemedium` del diferencial → `unregistervm` sin `--delete` |
| Poca RAM o poco disco | El host se pone lento | `max_instancias` en 2; instancias con 384 MB |
| Un integrante se atrasa | Hito no cumplido | En la reunión del hito se reparte su fase pendiente; los Fakes permiten seguir mientras tanto |

---

## 12. Dudas para el profesor (preguntar antes de H1)

1. ¿El dominio debe ser exactamente `cloud.local` y las IPs las de la figura (`.10` para DNS y `.30` en adelante)?
2. ¿La webapp puede correr en el host, o debe estar en una VM?
3. ¿Hay sustentación? ¿En qué PC y con cuántas instancias?
4. ¿Cómo se entrega el código fuente: enlace al repositorio o archivo `.zip` en Classroom?
5. El enunciado dice Debian **13.0**; la versión actual es 13.7. ¿Sirve cualquier 13.x? (El plan asume que sí.)

Eliminar instancias **no** es una duda: el objetivo específico 2 pide automatizar la "creación, configuración y eliminación", y la figura tiene el botón "Eliminar".
