package instancias

// Regression tests for the findings of the PR #1 reviews (2026-11-06). Each one fails on
// the code before its fix.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/puertos"
)

// Finding 1: a name already used by another VirtualBox VM must be refused before anything
// is reserved; otherwise "Eliminar" on the failed instance would delete that other VM.
func TestCrearRejectsTheNameOfAnExistingVM(t *testing.T) {
	e := nuevoEntorno(t, 3)
	_ = e.hip.Crear(context.Background(), puertos.SpecVM{Nombre: "servidor1"}) // a VM the app did not create
	_, err := e.s.SolicitarCrear("servidor1")
	if !esValidacion(err) || !strings.Contains(err.Error(), "ya existe una máquina virtual") {
		t.Fatalf("err = %v", err)
	}
	if _, ok := e.alm.Obtener("servidor1"); ok {
		t.Fatal("nothing may be reserved for a name VirtualBox already uses")
	}
}

func TestCrearFailsClosedWhenVirtualBoxCannotBeListed(t *testing.T) {
	e := nuevoEntorno(t, 3)
	e.hip.FallarEn("Listar", errors.New("VBoxSVC not running"))
	if _, err := e.s.SolicitarCrear("paisajes"); err == nil {
		t.Fatal("without the VM list the name cannot be checked, so the request must fail")
	}
	if len(e.s.Listar()) != 0 {
		t.Fatal("nothing may be reserved")
	}
}

// Finding 4: while publishing, the worker owns the instance.
func TestEliminarRejectedWhilePublishing(t *testing.T) {
	e := nuevoEntorno(t, 3)
	_, _ = e.s.SolicitarCrear("paisajes")
	e.correr(t, 1)
	_ = e.s.SolicitarPublicar("paisajes", zipTemporal(t))
	if err := e.s.SolicitarEliminar("paisajes"); !esValidacion(err) {
		t.Fatalf("delete during publish: %v", err)
	}
}

func TestPublicarDoesNotOverwriteAnotherState(t *testing.T) {
	e := nuevoEntorno(t, 3)
	_, _ = e.s.SolicitarCrear("paisajes")
	e.correr(t, 1)
	_ = e.s.SolicitarPublicar("paisajes", zipTemporal(t))
	_, _ = e.alm.Actualizar("paisajes", func(i *dominio.Instancia) { i.Estado = dominio.Eliminando })
	e.correr(t, 1)
	if got := e.estado(t, "paisajes"); got.Estado != dominio.Eliminando {
		t.Fatalf("a finished publish must not overwrite %s, got %s", dominio.Eliminando, got.Estado)
	}
}

// Finding 10: the dashboard shows a clear message; the technical detail goes to the log.
func TestUserMessagesHideTechnicalDetail(t *testing.T) {
	e := nuevoEntorno(t, 3)
	var mu sync.Mutex
	var logs []string
	e.s.o.Logf = func(f string, a ...any) { mu.Lock(); logs = append(logs, fmt.Sprintf(f, a...)); mu.Unlock() }
	detalle := "VBoxManage storageattach --medium /home/lycan/VirtualBox VMs/x.vdi: exit status 1"
	e.hip.FallarEn("Crear", errors.New(detalle))
	_, _ = e.s.SolicitarCrear("paisajes")
	e.correr(t, 1)
	if got := e.estado(t, "paisajes"); strings.Contains(got.Error, "/home/lycan") || got.Error == "" {
		t.Fatalf("user message = %q", got.Error)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(strings.Join(logs, "\n"), detalle) {
		t.Fatalf("the technical detail must reach the log: %v", logs)
	}
}

func TestPublicarShowsValidationMessagesOnly(t *testing.T) {
	tests := []struct {
		name, quiero, noQuiero string
		err                    error
	}{
		{"invalid zip", "no tiene index.html", "", &dominio.ErrorValidacion{Campo: dominio.CampoArchivo, Mensaje: "no tiene index.html"}},
		{"server failure", "revisa el registro", "httpaas@192.168.10.30", errors.New("ssh httpaas@192.168.10.30: exit 3")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := nuevoEntorno(t, 3)
			_, _ = e.s.SolicitarCrear("paisajes")
			e.correr(t, 1)
			e.pub.FallarEn("Publicar", tt.err)
			_ = e.s.SolicitarPublicar("paisajes", zipTemporal(t))
			e.correr(t, 1)
			got := e.estado(t, "paisajes").Error
			if !strings.Contains(got, tt.quiero) || (tt.noQuiero != "" && strings.Contains(got, tt.noQuiero)) {
				t.Fatalf("message = %q", got)
			}
		})
	}
}

// Finding 7: if only the final store write fails, the instance must stay retryable.
type almacenQuitarFalla struct {
	Almacen
	fallos int
}

func (a *almacenQuitarFalla) Quitar(nombre string) error {
	if a.fallos > 0 {
		a.fallos--
		return errors.New("disk full")
	}
	return a.Almacen.Quitar(nombre)
}

func TestEliminarStoreFailureIsRetryable(t *testing.T) {
	e := nuevoEntorno(t, 3)
	_, _ = e.s.SolicitarCrear("paisajes")
	e.correr(t, 1)
	falla := &almacenQuitarFalla{Almacen: e.alm, fallos: 1}
	e.s.o.Almacen = falla
	_ = e.s.SolicitarEliminar("paisajes")
	e.correr(t, 1)
	if got := e.estado(t, "paisajes"); got.Estado != dominio.Error {
		t.Fatalf("after a failed final write the state must be Error (retryable), got %s", got.Estado)
	}
	if err := e.s.SolicitarEliminar("paisajes"); err != nil {
		t.Fatal(err)
	}
	e.correr(t, 1)
	if _, ok := e.alm.Obtener("paisajes"); ok {
		t.Fatal("the retry must finish the deletion")
	}
}

// Finding 11: one stuck call cannot block the single queue forever.
type hipColgado struct{ *puertos.FakeHipervisor }

func (h hipColgado) Crear(ctx context.Context, _ puertos.SpecVM) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestJobsHaveATimeLimit(t *testing.T) {
	e := nuevoEntorno(t, 3)
	e.s.o.Hipervisor = hipColgado{e.hip}
	e.s.o.TiempoTrabajo = 20 * time.Millisecond
	_, _ = e.s.SolicitarCrear("paisajes")
	inicio := time.Now()
	e.correr(t, 1) // correr's own context allows 1 s
	if d := time.Since(inicio); d > 500*time.Millisecond {
		t.Fatalf("the job ran %v: the job time limit was not applied", d)
	}
	if got := e.estado(t, "paisajes"); got.Estado != dominio.Error {
		t.Fatalf("state = %s", got.Estado)
	}
}

// almacenLectorViejo returns an outdated state from Obtener, like a read that happened
// just before another operation changed the instance.
type almacenLectorViejo struct{ Almacen }

func (a almacenLectorViejo) Obtener(nombre string) (dominio.Instancia, bool) {
	i, ok := a.Almacen.Obtener(nombre)
	i.Estado = dominio.Lista
	return i, ok
}

// Finding 12: the state restored after a rejected delete must be the one replaced under
// the store's lock, not one read separately before it (the old code read it with Obtener).
func TestEliminarRestoresTheStateItReplaced(t *testing.T) {
	e := nuevoEntorno(t, 100)
	e.hip.FallarEn("Crear", errors.New("boom"))
	_, _ = e.s.SolicitarCrear("paisajes")
	e.correr(t, 1) // paisajes is now in Error
	e.hip.FallarEn("Crear", nil)
	for n := range cap(e.s.trabajos) {
		if _, err := e.s.SolicitarCrear(fmt.Sprintf("sitio%02d", n)); err != nil {
			t.Fatal(err)
		}
	}
	e.s.o.Almacen = almacenLectorViejo{e.alm}
	if err := e.s.SolicitarEliminar("paisajes"); !errors.Is(err, ErrOcupado) {
		t.Fatalf("err = %v", err)
	}
	if got := e.estado(t, "paisajes"); got.Estado != dominio.Error {
		t.Fatalf("state after a rejected delete = %s, want %s", got.Estado, dominio.Error)
	}
}
