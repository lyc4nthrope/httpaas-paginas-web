// Package instancias orchestrates the lifecycle of HTTP instances: create, publish and
// delete. Requests only validate, record the new state and enqueue a job; a single
// worker runs the jobs one at a time (decision D4), so HTTP handlers never block on a
// VM and two jobs never touch VirtualBox at once.
package instancias

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/almacen"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/puertos"
)

// ErrOcupado means the job queue is full; the user should retry later.
var ErrOcupado = errors.New("el servicio está ocupado; intenta de nuevo en unos segundos")

// Almacen is the instance store the service needs (implemented by internal/almacen).
type Almacen interface {
	Reservar(nombre string, max int, ahora time.Time) (dominio.Instancia, error)
	Actualizar(nombre string, cambio func(*dominio.Instancia)) (dominio.Instancia, error)
	Quitar(nombre string) error
	Obtener(nombre string) (dominio.Instancia, bool)
	Listar() []dominio.Instancia
}

// Opciones are the dependencies and limits of the service.
type Opciones struct {
	Hipervisor puertos.Hipervisor
	DNS        puertos.DNS
	Publicador puertos.Publicador
	Almacen    Almacen
	Max        int // maximum instances at once
	Prefijo    int // network prefix, e.g. 24
	// Olvidar removes the instance's host key from the app's known_hosts (decision D8).
	Olvidar func(ctx context.Context, ip string) error
	// TiempoCrear bounds how long a VM may take to report "listo". Default 3 minutes.
	TiempoCrear time.Duration
	// TiempoTrabajo bounds a whole job, so one stuck call cannot block the queue. Default 10 minutes.
	TiempoTrabajo time.Duration
	// Ahora and Logf are replaceable in tests.
	Ahora func() time.Time
	Logf  func(format string, args ...any)
}

type tipoTrabajo int

const (
	crear tipoTrabajo = iota
	publicar
	eliminar
)

type trabajo struct {
	tipo   tipoTrabajo
	nombre string
	zip    string // only for publicar
}

// Servicio is safe for concurrent use.
type Servicio struct {
	o        Opciones
	trabajos chan trabajo
}

func Nuevo(o Opciones) *Servicio {
	if o.TiempoCrear == 0 {
		o.TiempoCrear = 3 * time.Minute
	}
	if o.TiempoTrabajo == 0 {
		o.TiempoTrabajo = 10 * time.Minute
	}
	if o.Ahora == nil {
		o.Ahora = time.Now
	}
	if o.Logf == nil {
		o.Logf = log.Printf
	}
	if o.Olvidar == nil {
		o.Olvidar = func(context.Context, string) error { return nil }
	}
	return &Servicio{o: o, trabajos: make(chan trabajo, 32)}
}

// Iniciar runs the worker until ctx ends.
func (s *Servicio) Iniciar(ctx context.Context) {
	go func() {
		for s.siguiente(ctx) {
		}
	}()
}

// siguiente runs one queued job. It returns false when ctx ends.
func (s *Servicio) siguiente(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case t := <-s.trabajos:
		ctx, cancel := context.WithTimeout(ctx, s.o.TiempoTrabajo)
		defer cancel()
		switch t.tipo {
		case crear:
			s.crear(ctx, t.nombre)
		case publicar:
			s.publicar(ctx, t.nombre, t.zip)
		case eliminar:
			s.eliminar(ctx, t.nombre)
		}
		return true
	}
}

// Listar returns every instance for the dashboard.
func (s *Servicio) Listar() []dominio.Instancia { return s.o.Almacen.Listar() }

// SolicitarCrear validates the name, reserves an IP and queues the creation.
func (s *Servicio) SolicitarCrear(nombre string) (dominio.Instancia, error) {
	if err := dominio.ValidarHostname(nombre); err != nil {
		return dominio.Instancia{}, err
	}
	if err := s.nombreLibreEnVirtualBox(nombre); err != nil {
		return dominio.Instancia{}, err
	}
	inst, err := s.o.Almacen.Reservar(nombre, s.o.Max, s.o.Ahora())
	if err != nil {
		return dominio.Instancia{}, err
	}
	if err := s.encolar(trabajo{tipo: crear, nombre: nombre}); err != nil {
		_ = s.o.Almacen.Quitar(nombre) // nothing was created yet: release the name and IP
		return dominio.Instancia{}, err
	}
	return inst, nil
}

// nombreLibreEnVirtualBox refuses a name that another VirtualBox VM already uses. It runs
// before anything is reserved: if creation failed on such a name, "Eliminar" would delete
// that other VM. A name the store already holds is left to Reservar, which says so.
func (s *Servicio) nombreLibreEnVirtualBox(nombre string) error {
	if _, ok := s.o.Almacen.Obtener(nombre); ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vms, err := s.o.Hipervisor.Listar(ctx)
	if err != nil {
		return fmt.Errorf("list vms to check the name: %w", err)
	}
	for _, vm := range vms {
		if strings.EqualFold(vm, nombre) {
			return &dominio.ErrorValidacion{Campo: dominio.CampoNombre,
				Mensaje: fmt.Sprintf("ya existe una máquina virtual llamada %q en VirtualBox; elige otro nombre", vm)}
		}
	}
	return nil
}

// SolicitarPublicar queues the deployment of an already validated zip. The service
// owns zip from here on and always deletes it.
func (s *Servicio) SolicitarPublicar(nombre, zip string) error {
	_, err := s.cambiarEstado(nombre, dominio.Estado.PuedePublicar, dominio.Publicando,
		"solo se puede publicar en una instancia lista")
	if err == nil {
		err = s.encolar(trabajo{tipo: publicar, nombre: nombre, zip: zip})
		if err != nil {
			s.actualizar(nombre, func(i *dominio.Instancia) { i.Estado = dominio.Lista })
		}
	}
	if err != nil {
		os.Remove(zip)
	}
	return err
}

// SolicitarEliminar queues the deletion. It is allowed from every state except while
// another job owns the instance (creando, publicando, eliminando).
func (s *Servicio) SolicitarEliminar(nombre string) error {
	anterior, err := s.cambiarEstado(nombre, dominio.Estado.PuedeEliminar, dominio.Eliminando,
		"la instancia está ocupada; espera a que termine la operación en curso")
	if err != nil {
		return err
	}
	if err := s.encolar(trabajo{tipo: eliminar, nombre: nombre}); err != nil {
		s.actualizar(nombre, func(i *dominio.Instancia) { i.Estado = anterior })
		return err
	}
	return nil
}

// Reconciliar runs at startup: instances whose VM no longer exists become Perdida, and
// operations cut by a restart become Error, so the user can delete and retry.
func (s *Servicio) Reconciliar(ctx context.Context) error {
	vms, err := s.o.Hipervisor.Listar(ctx)
	if err != nil {
		return fmt.Errorf("list vms: %w", err)
	}
	existe := map[string]bool{}
	for _, v := range vms {
		existe[v] = true
	}
	for _, i := range s.o.Almacen.Listar() {
		switch {
		case !existe[i.Nombre]:
			s.fallar(i.Nombre, dominio.Perdida, "la máquina virtual ya no existe en VirtualBox", nil)
		case i.Estado == dominio.Creando || i.Estado == dominio.Publicando || i.Estado == dominio.Eliminando:
			s.fallar(i.Nombre, dominio.Error, "la aplicación se reinició durante la operación; elimina la instancia y créala de nuevo", nil)
		}
	}
	return nil
}

func (s *Servicio) crear(ctx context.Context, nombre string) {
	inst, ok := s.o.Almacen.Obtener(nombre)
	if !ok {
		return
	}
	spec := puertos.SpecVM{Nombre: nombre, IP: inst.IP, Prefijo: s.o.Prefijo}
	if err := s.o.Hipervisor.Crear(ctx, spec); err != nil {
		s.fallar(nombre, dominio.Error, "no se pudo crear la máquina virtual", err)
		return
	}
	espera, cancel := context.WithTimeout(ctx, s.o.TiempoCrear)
	defer cancel()
	if err := s.o.Hipervisor.EsperarLista(espera, nombre); err != nil {
		s.fallar(nombre, dominio.Error, "la máquina virtual no terminó de configurarse", err)
		return
	}
	if err := s.o.DNS.Agregar(ctx, nombre, inst.IP); err != nil {
		s.fallar(nombre, dominio.Error, "no se pudo registrar el nombre en el DNS", err)
		return
	}
	ip, err := s.o.DNS.Resolver(ctx, nombre)
	if err == nil && ip != inst.IP {
		err = fmt.Errorf("resolves to %s, want %s", ip, inst.IP)
	}
	if err != nil {
		s.fallar(nombre, dominio.Error, "el DNS no resuelve el nombre a la IP de la instancia", err)
		return
	}
	s.actualizar(nombre, func(i *dominio.Instancia) { i.Estado, i.Error = dominio.Lista, "" })
}

// publicar keeps the instance Lista even when the deployment fails: Apache still serves
// the previous content and the user can try again. The error is shown on the dashboard.
func (s *Servicio) publicar(ctx context.Context, nombre, zip string) {
	defer os.Remove(zip)
	inst, ok := s.o.Almacen.Obtener(nombre)
	if !ok {
		return
	}
	err := s.o.Publicador.Publicar(ctx, nombre, inst.IP, zip)
	msg := ""
	if err != nil {
		s.o.Logf("publicar %s: %v", nombre, err)
		msg = "la última publicación falló; revisa el registro de la aplicación"
		var ev *dominio.ErrorValidacion
		if errors.As(err, &ev) {
			msg = "la última publicación falló: " + ev.Mensaje
		}
	}
	s.actualizar(nombre, func(i *dominio.Instancia) {
		if i.Estado != dominio.Publicando {
			return // another operation took over; do not overwrite it
		}
		i.Estado, i.Error = dominio.Lista, msg
	})
}

// eliminar tries every step even if one fails. Each step is idempotent, so when
// something fails the instance stays in Error and deleting it again is safe.
func (s *Servicio) eliminar(ctx context.Context, nombre string) {
	inst, ok := s.o.Almacen.Obtener(nombre)
	if !ok {
		return
	}
	var fallos []string
	paso := func(que string, err error) {
		if err != nil {
			s.o.Logf("eliminar %s: %s: %v", nombre, que, err)
			fallos = append(fallos, que)
		}
	}
	paso("quitar el nombre del DNS", s.o.DNS.Quitar(ctx, nombre))
	paso("borrar la máquina virtual", s.o.Hipervisor.Eliminar(ctx, nombre))
	paso("olvidar la llave SSH", s.o.Olvidar(ctx, inst.IP))
	if len(fallos) > 0 {
		s.fallar(nombre, dominio.Error, "no se pudo eliminar por completo (falló: "+strings.Join(fallos, ", ")+"); vuelve a intentarlo", nil)
		return
	}
	if err := s.o.Almacen.Quitar(nombre); err != nil {
		s.fallar(nombre, dominio.Error, "se eliminó la máquina virtual, pero no se pudo actualizar el registro de instancias; vuelve a intentarlo", err)
	}
}

func (s *Servicio) encolar(t trabajo) error {
	select {
	case s.trabajos <- t:
		return nil
	default:
		return ErrOcupado
	}
}

// cambiarEstado moves an instance to nuevo only if permitido(current) holds, and returns
// the state it had. Check and change happen under the store's lock, so the returned
// state is exactly the one replaced.
func (s *Servicio) cambiarEstado(nombre string, permitido func(dominio.Estado) bool, nuevo dominio.Estado, motivo string) (dominio.Estado, error) {
	var rechazado bool
	var anterior dominio.Estado
	_, err := s.o.Almacen.Actualizar(nombre, func(i *dominio.Instancia) {
		anterior = i.Estado
		if !permitido(i.Estado) {
			rechazado = true
			return
		}
		i.Estado = nuevo
	})
	if errors.Is(err, almacen.ErrNoExiste) {
		return "", &dominio.ErrorValidacion{Campo: dominio.CampoNombre, Mensaje: fmt.Sprintf("no existe una instancia llamada %q", nombre)}
	}
	if err != nil {
		return "", err
	}
	if rechazado {
		return "", &dominio.ErrorValidacion{Campo: dominio.CampoNombre, Mensaje: motivo}
	}
	return anterior, nil
}

// fallar records a failure: msg is for the user (shown on the dashboard), err (optional)
// is the technical detail and goes only to the log.
func (s *Servicio) fallar(nombre string, estado dominio.Estado, msg string, err error) {
	if err != nil {
		s.o.Logf("%s: %s: %v", nombre, msg, err)
	}
	s.actualizar(nombre, func(i *dominio.Instancia) { i.Estado, i.Error = estado, msg })
}

func (s *Servicio) actualizar(nombre string, cambio func(*dominio.Instancia)) {
	if _, err := s.o.Almacen.Actualizar(nombre, cambio); err != nil {
		s.o.Logf("update %s: %v", nombre, err)
	}
}
