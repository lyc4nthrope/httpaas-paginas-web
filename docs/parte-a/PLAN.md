# Parte A — Plataforma y orquestación

**Responsable:** Cristhian Osorio · **Rama:** `cristhian` · **También coordina:** revisa e integra los PR hacia `dev`
**Lee antes:** [plan general](../PLAN.md), §2 (arquitectura), §3 (decisiones) y §4 (contrato).

---

## Para el asistente de IA (copiar tal cual)

```text
Eres el asistente de Cristhian en el Parcial 3 de Computación en la Nube: un servicio HTTPaaS local
(webapp en Go que crea VMs Apache en VirtualBox desde un disco multiconexión, las registra en Bind9 y
publica un .zip del usuario). Repositorio: httpaas-paginas-web. Lee docs/PLAN.md y docs/parte-a/PLAN.md.

Tu parte (A) es dueña de: cmd/httpaas, internal/{dominio,puertos,config,executor,oplog,almacen,instancias,vbox,web}
(excepto web/templates/dashboard.html y web/static, que son de B), scripts/prueba-completa.sh, Makefile,
config.example.json. No edites carpetas de B (internal/dns, deploy/ns) ni de C (internal/publicar, deploy/plantilla).

Reglas obligatorias:
- Go 1.24, solo biblioteca estándar salvo acuerdo del equipo. Rutas con net/http (patrones "POST /x/{nombre}").
- Nunca concatenes datos del usuario en comandos: exec.CommandContext(nombre, args...) con args ya validados. Nunca sh -c.
- El contrato de docs/PLAN.md §4 (interfaces, guest properties, scripts, config) NO se cambia sin acuerdo del equipo.
- Trabaja por fases (A0..A8) en orden. Una fase termina cuando su "Verificación" pasa. No avances antes.
- Cada fase: pruebas de tabla para funciones puras, Fakes para los puertos, `make check` en verde, un commit convencional en inglés.
- Errores: envuelve con fmt.Errorf("contexto: %w", err). Al usuario, un mensaje claro; al registro (oplog), el detalle técnico.
- No inventes comandos de VBoxManage: usa los de docs/parte-c/clon-manual.md y verifica con `VBoxManage <cmd> --help`.
- Antes de escribir código, explica en 3-5 líneas qué vas a hacer y por qué. Después, muestra cómo verificarlo.
```

---

## Qué entrega esta parte

El "cerebro" de la nube: la webapp, la cola que crea, publica y elimina instancias, y el adaptador que maneja VirtualBox. También la integración de todo y la prueba de punta a punta.

---

## Fases

### A0. Esqueleto del repositorio ⬜ (H0)
**Meta:** que los tres puedan clonar, compilar y correr `make check` el primer día.
1. `go mod init github.com/lyc4nthrope/httpaas-paginas-web`.
2. `Makefile` con `build`, `run`, `simular`, `test`, `check` y `vm-test` (copiar del Parcial 2 y cambiar el nombre del binario a `httpaas`).
3. `cmd/httpaas/main.go` mínimo: lee `-config`, imprime `escuchando en ...` y sirve `GET /salud` → `ok`.
4. `.github/workflows/check.yml`: en cada PR hacia `dev` o `main` ejecuta `make check`.

**Verificación:** `make check` en verde. `curl -s 127.0.0.1:8080/salud` → `ok`. El workflow aparece en GitHub.
📸 **A-01** — `make check` en verde. 📸 **A-02** — workflow en verde en GitHub.
**Commit:** `chore: bootstrap go module, makefile and ci`

### A1. Contrato común ⬜ (H0, con B y C presentes o revisando el PR)
**Meta:** que los tres trabajen en paralelo sin VMs.
1. `internal/dominio`: tipo `Instancia {Nombre, IP, Estado, Creada, Error string}`, constantes de `Estado` (§4.6 del plan), `ValidarHostname(s) error` (D10) y `FQDN(host, dominio)`.
2. `internal/puertos`: las tres interfaces **exactamente** como en §4.2, más `fakes.go` (`FakeHipervisor`, `FakeDNS`, `FakePublicador`) que guardan las llamadas y aceptan un error programado por método.
3. Pruebas de tabla de `ValidarHostname`: válidos (`paisajes`, `mi-sitio2`) e inválidos (`ns`, `Paisajes`, `-x`, `a`, `x_y`, `a;rm`, 33 caracteres, vacío).

**Verificación:** `make check` en verde. PR a `dev` aprobado por Daniel y Wiliam.
**Commit:** `feat(dominio): define instance model, hostname rules and service ports`

### A2. Configuración, executor SSH y registro ⬜
**Meta:** ejecutar comandos remotos sin interacción y dejar rastro de cada uno.
1. `internal/config`: lee `config.json` (§4.5) y valida: IPs, rango `primera ≤ ultima`, que exista `disco_base`, que exista la llave y que `max_instancias ≥ 1`. Un error por campo.
2. `internal/executor`: copiar `ssh.go`, `executor.go` y `fake.go` del Parcial 2 (`consola-automatizacion-servicios/internal/executor`). Ajustar las opciones de SSH: `-i <llave> -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=<known_hosts> -o ConnectTimeout=10`. Agregar `OlvidarHost(ip)` → `ssh-keygen -R <ip> -f <known_hosts>`.
3. `internal/oplog`: copiar del Parcial 2. Una línea JSON por operación en `logs/operaciones.jsonl` (permisos 600, porque la salida puede tener secretos).

**Verificación:** pruebas con `-race`. Prueba `vm` contra la VM `ns` de B cuando exista: `Run(ctx, Command{Name:"hostname"})` → `ns`.
📸 **A-03** — línea del registro de un comando real.
**Commit:** `feat(executor): run remote commands over non-interactive ssh`

### A3. Almacén y asignación de IPs ⬜
**Meta:** que el estado sobreviva a reinicios y que nunca se repita una IP.
1. `internal/almacen`: `Cargar`, `Guardar` (archivo temporal en la misma carpeta + `os.Rename`), `Agregar`, `Actualizar`, `Quitar` y `Listar` (ordenado por fecha). Protegido con `sync.Mutex`.
2. `SiguienteIP()`: la menor IP libre entre `primera` y `ultima`. Error claro si no hay.
3. Pruebas: archivo inexistente → lista vacía; JSON corrupto → error (no lo sobrescribe); IPs `.30` y `.32` ocupadas → `.31`; rango lleno → error; escrituras concurrentes con `-race`.

**Verificación:** `make check`. Matar el proceso a mitad de `Guardar` no deja el JSON a medias (se revisa la prueba del `rename`).
**Commit:** `feat(almacen): persist instances atomically and allocate free ips`

### A4. Servicio orquestador con cola ⬜
**Meta:** toda la lógica de crear, publicar y eliminar, probada solo con Fakes.
1. `internal/instancias.Servicio` recibe `Hipervisor`, `DNS`, `Publicador`, el almacén y la config.
2. Un solo goroutine consume un canal de trabajos (D4). `Solicitar*` valida, guarda el estado y encola. No bloquea la petición HTTP.
3. `SolicitarCrear(nombre)`: valida (formato, repetido, máximo) → `creando` + IP → trabajo: `Crear` → `EsperarLista` (tiempo máximo 3 min) → `DNS.Agregar` → `DNS.Resolver == IP` → `lista`.
4. `SolicitarPublicar(nombre, zipLocal)`: solo si está `lista` → `publicando` → `Publicador.Publicar` → `lista`. Borra el zip temporal siempre.
5. `SolicitarEliminar(nombre)`: desde cualquier estado excepto `creando` o `eliminando` → `DNS.Quitar` → `Hipervisor.Eliminar` → `OlvidarHost` → quitar del almacén. Si un paso falla, sigue con los demás y deja `error` con el detalle.
6. `Reconciliar()` al arrancar: las instancias del JSON que no están en `Hipervisor.Listar` pasan a `perdida`; las que quedaron en `creando` o `publicando` pasan a `error` ("la app se reinició durante la operación").

**Verificación:** pruebas de tabla con Fakes para cada camino feliz y cada fallo (por ejemplo, si `DNS.Agregar` falla: estado `error`, y Eliminar luego llama a `Hipervisor.Eliminar`). Pruebas con `-race`.
**Commit:** `feat(instancias): orchestrate create, publish and delete through a single worker`

### A5. Servidor web y rutas ⬜ (H1)
**Meta:** la webapp completa con Fakes (`make simular`).
1. Rutas:
   - `GET /` → dashboard (la plantilla de B; mientras no exista, usar una tabla simple).
   - `POST /instancias` (campo `nombre`) → botón **Aceptar**.
   - `POST /instancias/{nombre}/contenido` (campo `archivo`, multipart) → botón **Publicar**. Usa `http.MaxBytesReader(max_zip_mb)`, guarda en un archivo temporal y llama a `publicar.ValidarZip` de C (mientras no exista, a un stub).
   - `POST /instancias/{nombre}/eliminar` → botón **Eliminar**.
   - `GET /api/instancias` → JSON para que el dashboard se actualice solo.
2. Patrón POST → redirección 303 → GET, con un mensaje flash ("Instancia paisajes en creación"). Si hay un error de validación, 422 y se vuelve a mostrar el formulario con el error en el campo.
3. Protección CSRF simple: rechazar los POST cuyo `Origin` no sea el de la propia app.
4. `-simular` en `main.go`: usa los tres Fakes, con un retardo de 2 s para ver los estados.
5. Nombres de campos HTML como constantes compartidas (`dominio.CampoNombre`, `dominio.CampoArchivo`). En el Parcial 2 hubo formularios que no coincidían con el código.

**Verificación:** `make simular` → crear, publicar y eliminar desde el navegador. Pruebas con `httptest`: nombre inválido → 422; Origin ajeno → 403; zip de más de 50 MB → 413.
📸 **A-04** — dashboard en modo simulado con una instancia en `creando`. 📸 **A-05** — error 422 mostrado en el formulario.
**Commit:** `feat(web): serve dashboard and instance routes`

### A6. Adaptador VirtualBox ⬜ (pasos 1–5 desde H1; paso 6 y verificación en cuanto C entregue `clon-manual.md`, a más tardar en H3)
**Meta:** crear y borrar VMs reales a partir del disco multiconexión.
> **Por qué en dos tiempos:** C6 y A6 caen en el mismo hito. Los pasos 1–5 son funciones puras con pruebas y no necesitan VMs, así que se adelantan. Cuando llegue `clon-manual.md`, solo hay que comparar los argumentos con ese documento y correr la prueba real.
1. `internal/vbox` implementa `Hipervisor` llamando a `VBoxManage` con `exec.CommandContext` y los argumentos separados, siguiendo **exactamente** los comandos de `clon-manual.md`:
   `createvm --name N --ostype Debian_64 --register` → `modifyvm N --memory M --cpus C --firmware bios --nic1 hostonly --host-only-adapter1 vboxnet0 --nic2 nat --audio-enabled off` → `storagectl N --name SATA --add sata --controller IntelAhci` → `storageattach N --storagectl SATA --port 0 --device 0 --type hdd --medium <disco_base>` → `guestproperty set` ×4 (§4.3) → `startvm N --type headless`.
2. Si un paso de `Crear` falla después de `createvm`, se deshace con `unregistervm --delete`.
3. `EsperarLista`: ciclo de `guestproperty get N /httpaas/estado` cada 2 s hasta `listo` o `error:*` (o `guestproperty wait`), más una prueba TCP al puerto 22 de la IP.
4. `Eliminar`: `controlvm N poweroff` (ignora "not running") → esperar `VMState="poweroff"` en `showvminfo --machinereadable` → `unregistervm N --delete`.
5. **Funciones puras** que arman los argumentos (`argsCrear(spec, cfg) [][]string`), con prueba de tabla. Así se prueba sin VirtualBox.
6. **Prueba crítica de seguridad (D-riesgo 3):** crear una instancia, eliminarla cuando es la **única** que usa el disco, y comprobar que `ApacheServer.vdi` sigue en `VBoxManage list hdds` y en el disco. Si desaparece, aplicar el plan B del §11 antes de seguir.

**Verificación:** prueba `vm`: `Crear("prueba-a6")` → `EsperarLista` → `ssh httpaas@192.168.10.30 hostname` = `prueba-a6` → `Eliminar` → ya no aparece en `VBoxManage list vms` → el `.vdi` base existe.
📸 **A-06** — `VBoxManage list vms` con la instancia creada. 📸 **A-07** — `VBoxManage showhdinfo` del disco base mostrando su hijo diferencial. 📸 **A-08** — después de eliminar: la VM no está y el `.vdi` base sí.
**Commit:** `feat(vbox): create and delete instances from the multiattach disk`

### A7. Integración en `dev` ⬜ (H3)
**Meta:** que funcione de punta a punta con las piezas reales de B y C.
1. Unir los PR de B (`internal/dns`, dashboard) y C (`internal/publicar`) en `dev`, resolver conflictos y correr `make check`.
2. `main.go` real: `vbox` + `dns` + `publicar`. Llamar a `Reconciliar()` al arrancar.
3. Crear `paisajes`, publicar `testdata/zips/paisajes.zip` y abrir el enlace del dashboard.

**Verificación:** el flujo completo en el navegador, sin tocar la terminal.
📸 **A-09** — formulario con `paisajes` y `paisajes.zip`. 📸 **A-10** — dashboard con la instancia `lista` y su enlace.
**Commit:** `feat: wire real adapters into the http service`

### A8. Prueba completa y cierre ⬜ (H4)
**Meta:** demostrar que todo funciona en los tres PCs.
1. `scripts/prueba-completa.sh`: los 7 pasos del §10 del plan, con `curl` contra la webapp (incluye la cabecera `Origin`).
2. Correrlo en los tres PCs y anotar el resultado en `docs/PRUEBA-COMPLETA.md`.
3. `README.md`: requisitos, preparación (enlaces a P1–P4 y B5), compilar, configurar y usar.
4. Escribir la sección "Aplicación web de gestión" para el documento de B (½ página + capturas A-xx).

**Verificación:** el script imprime `OK` en los 7 pasos en los tres PCs.
📸 **A-11 … A-16** — dos instancias en el dashboard; `dig` de las dos; la página de cada una abierta desde su enlace (pestaña nueva); el rechazo de `zipslip.zip`; el resultado del script.
**Commit:** `test: add end-to-end check script`

---

## Errores comunes en esta parte

- **`VBoxManage` en Windows** no está en el `PATH`: usar `vbox.vboxmanage` con la ruta completa.
- **`--audio-enabled`** existe desde VirtualBox 7.0. Confirmar con `VBoxManage modifyvm --help`.
- **Clon que no arranca:** el firmware o el controlador no coinciden con la plantilla (D16). Comparar con `VBoxManage showvminfo apache-plantilla`.
- **"Host key verification failed"** al reutilizar una IP: faltó `OlvidarHost` al eliminar (D8).
- **La petición HTTP se queda colgada creando la VM:** el trabajo se ejecutó en el handler y no en la cola (A4.2).
