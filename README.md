# HTTPaaS — Servicio administrado de páginas y aplicaciones web

Parcial 3 de **Computación en la Nube 2026-2** (Universidad del Quindío).

**Equipo:** Cristhian Osorio (Parte A) · Daniel Pérez (Parte B) · Wiliam Carmona (Parte C)

Es una **mini nube local**. Desde una página web, el usuario escribe un nombre (por ejemplo `paisajes`) y sube un `.zip` con su sitio. Sin intervención humana, la aplicación:
1. Crea una VM Apache a partir de un **disco multiconexión** de VirtualBox.
2. La registra en un **DNS Bind9**, con el nombre `paisajes.cloud.local`.
3. Publica el contenido del zip.
4. Muestra la instancia en un dashboard, con su enlace.

```
 Navegador ─► Webapp Go (host, 127.0.0.1:8080) ── cola de trabajos (uno a la vez)
                 ├─ VBoxManage ──────────────► VirtualBox: clones de ApacheServer.vdi (multiconexión)
                 ├─ SSH ► ns (192.168.10.10) ─► httpaas-dns: registros A y PTR en cloud.local
                 └─ SSH ► instancia (.30+) ───► httpaas-publicar: zip → Apache
       red host-only 192.168.10.0/24 (vboxnet0) · cada VM tiene además NAT para salir a internet
```

| Pieza | Carpeta | Parte |
|---|---|---|
| Webapp, orquestador, VirtualBox, SSH, validación, registro | `cmd/httpaas`, `internal/{dominio,puertos,config,executor,oplog,almacen,instancias,vbox,web}` | A |
| VM `ns` (Bind9 con zona dinámica), adaptador DNS, resolución en el host, dashboard | `deploy/ns`, `internal/dns`, `scripts/dns-host-linux.sh`, `internal/web/templates` | B |
| Plantilla Apache (primer arranque, publicación), adaptador de publicación | `deploy/plantilla`, `internal/publicar`, `testdata/zips` | C |

El diseño completo (decisiones D1–D16, el contrato entre las partes y el cronograma) está en [docs/PLAN.md](docs/PLAN.md).

## Requisitos

- **Oracle VirtualBox 7.2** con `VBoxManage` en el `PATH`.
- **Go 1.24** o posterior, solo para compilar.
- `ssh`, `scp` y `ssh-keygen` (Linux, macOS, o Windows 10/11 con OpenSSH).
- Unos **10 GB libres**: VM `ns` (~3 GB), disco de la plantilla (~3 GB) e instancias (unos cientos de MB cada una).

## Preparación (una vez por PC)

Los pasos están detallados, con su verificación, en el [§7 del plan](docs/PLAN.md#7-preparación-de-cada-pc-los-tres-h0). En resumen:

1. **Permitir la red `192.168.10.0/24` en VirtualBox** (Linux y macOS; Windows no lo necesita):
   ```bash
   echo '* 192.168.10.0/24 192.168.56.0/21' | sudo tee /etc/vbox/networks.conf
   ```
2. **Crear la red host-only sin DHCP:**
   ```bash
   VBoxManage hostonlyif create
   VBoxManage hostonlyif ipconfig vboxnet0 --ip 192.168.10.1 --netmask 255.255.255.0
   VBoxManage dhcpserver remove --interface=vboxnet0
   ```
3. **Llave SSH de la app** (cada integrante la suya; la pública va en `deploy/llaves/`):
   ```bash
   mkdir -p keys && ssh-keygen -t ed25519 -N '' -f keys/httpaas_ed25519
   ```
4. **VM `ns` (DNS):** importarla y verificarla con la guía [docs/IMPORTAR-NS.md](docs/IMPORTAR-NS.md):
   ```bash
   scripts/importar-ns.sh ruta/a/ns.ova && VBoxManage startvm ns --type headless
   scripts/verificar-ns.sh           # 15 verificaciones de B1–B3
   ```
   También se puede reconstruir desde cero con `deploy/ns/instalar.sh` dentro de una Debian 13 nueva.
5. **Disco de la plantilla Apache:** ⏳ *pendiente de la Parte C.* `ApacheServer.vdi` en modo multiconexión, preparado con `deploy/plantilla/instalar.sh` y el procedimiento de [docs/parte-c/PLAN.md](docs/parte-c/PLAN.md) (C1–C6) y [clon-manual.md](docs/parte-c/clon-manual.md).
6. **Que el navegador resuelva `*.cloud.local`:**
   - **Linux:** `sudo scripts/dns-host-linux.sh` (se deshace con `--undo`).
   - **Windows:** regla NRPT.

   Los dos casos están en [docs/parte-b/dns-host.md](docs/parte-b/dns-host.md).

## Configurar

```bash
cp config.example.json config.json     # config.json no se sube al repositorio
```

| Campo | Qué es |
|---|---|
| `listen` | Dónde escucha la app. Debe ser una IP concreta (`127.0.0.1:8080`), nunca `0.0.0.0` |
| `dominio` | `cloud.local` |
| `red` | `base` `192.168.10.`, `prefijo` 24 y el rango `primera`–`ultima` que se reparte a las instancias (`.30`–`.99`) |
| `max_instancias`, `max_zip_mb` | Límites: 3 instancias a la vez, zips de hasta 50 MB |
| `dns` | IP de la VM `ns` y usuario SSH (`httpaas`) |
| `ssh` | Usuario de las instancias, llave privada y `known_hosts` propio de la app |
| `vbox` | `VBoxManage`, ruta a `ApacheServer.vdi`, red host-only (`vboxnet0`), y el firmware y controlador **iguales a los de la plantilla** |
| `datos` | Archivo donde se guardan las instancias (`data/instancias.json`) |

Si un campo está mal, la app **no arranca**: muestra todos los errores juntos, con su explicación.

## Ejecutar

```bash
make build                       # un solo binario, con las páginas incluidas
./httpaas                        # modo real: VirtualBox, ns y la plantilla
make simular                     # modo simulado: sin VMs, para conocer la interfaz
```

Abre `http://127.0.0.1:8080`. En Windows no hay `make`: usa `go build ./cmd/httpaas` y `httpaas.exe -simular -config config.example.json`.

## Usar

1. Escribe el **nombre del host** (de 3 a 32 caracteres: minúsculas, números y guiones) y pulsa **Aceptar**. La instancia aparece en `creando` y pasa a `lista` en un minuto, aproximadamente.
2. Elige el **.zip** (con `index.html` en la raíz o dentro de una sola carpeta) y pulsa **Publicar**.
3. Abre el enlace `http://<nombre>.cloud.local`. Se abre en una pestaña nueva.
4. **Eliminar** borra la VM, su registro DNS y su llave SSH. El disco base no se toca.

La app revisa `ns` cada 30 s. Si no responde, entrega un aviso con la forma de solucionarlo, que se quita solo cuando `ns` vuelve (fase B6.5). ⏳ *El dashboard lo mostrará cuando la plantilla incluya `{{with .DNSAviso}}`, pendiente de la Parte B.*

## Probar

| Qué | Comando |
|---|---|
| Formato, análisis, pruebas (con `-race`) y compilación. Es lo que corre la CI en cada PR | `make check` |
| Adaptador DNS contra la VM `ns` real | `HTTPAAS_CONFIG=$PWD/config.json go test -tags vm ./internal/dns/` |
| VM `ns` (B1–B3) | `scripts/verificar-ns.sh` |
| **Prueba completa** de punta a punta (§10 del plan): crear, DNS, publicar, contenido servido, rechazos, eliminar y disco base intacto | `scripts/prueba-completa.sh` (con la app corriendo) |
| Lo mismo, contra el modo simulado | `SIMULADO=1 scripts/prueba-completa.sh` |

## Seguridad, en breve

- **Sin contraseñas:** todo va por SSH con llaves (`BatchMode`), con un usuario `httpaas` cuyo `sudo` permite **solo** `httpaas-dns` o `httpaas-publicar`.
- **Los datos del usuario nunca se concatenan en un comando de shell:** viajan como argumentos separados y se validan antes de ejecutar nada.
- **El zip se valida dos veces** (en Go y dentro de la VM, con las mismas reglas): sin `..`, sin rutas absolutas, sin enlaces simbólicos, hasta 2000 archivos y 200 MB.
- **La app solo acepta su propio `Host` y su propio `Origin`** (contra DNS rebinding y CSRF), y solo borra VMs que ella misma creó.

Reglas para quien programe en el proyecto (y para su asistente de IA): [AGENTS.md](AGENTS.md) y [.claude/claude-security-guidance.md](.claude/claude-security-guidance.md).

## Documentación

| Documento | Contenido |
|---|---|
| [docs/PLAN.md](docs/PLAN.md) | Plan general: requisitos, arquitectura, decisiones, contrato y cronograma |
| [Parte A](docs/parte-a/PLAN.md) · [Parte B](docs/parte-b/PLAN.md) · [Parte C](docs/parte-c/PLAN.md) | El plan de cada parte, fase por fase, con su verificación |
| [docs/DOCUMENTO.md](docs/DOCUMENTO.md) | Documento final (capturas, problemas, mejoras y aprendizajes) |
| [docs/problemas.md](docs/problemas.md) | Problemas encontrados y cómo se resolvieron |
| [docs/enunciado/](docs/enunciado/) | Enunciado del profesor |
