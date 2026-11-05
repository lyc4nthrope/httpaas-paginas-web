package instancias

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/almacen"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/puertos"
)

type entorno struct {
	s        *Servicio
	hip      *puertos.FakeHipervisor
	dns      *puertos.FakeDNS
	pub      *puertos.FakePublicador
	alm      *almacen.Almacen
	mu       sync.Mutex
	olvidado []string
	olvErr   error
}

func nuevoEntorno(t *testing.T, max int) *entorno {
	t.Helper()
	var r []string
	for n := 30; n <= 99; n++ {
		r = append(r, fmt.Sprintf("192.168.10.%d", n))
	}
	alm, err := almacen.Abrir(filepath.Join(t.TempDir(), "instancias.json"), r)
	if err != nil {
		t.Fatal(err)
	}
	e := &entorno{hip: &puertos.FakeHipervisor{}, dns: &puertos.FakeDNS{}, pub: &puertos.FakePublicador{}, alm: alm}
	e.s = Nuevo(Opciones{
		Hipervisor: e.hip, DNS: e.dns, Publicador: e.pub, Almacen: alm, Max: max, Prefijo: 24,
		Olvidar: func(_ context.Context, ip string) error {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.olvidado = append(e.olvidado, ip)
			return e.olvErr
		},
		Ahora: func() time.Time { return time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC) },
		Logf:  func(string, ...any) {},
	})
	return e
}

// correr runs the queued jobs, like the worker does.
func (e *entorno) correr(t *testing.T, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for range n {
		if !e.s.siguiente(ctx) {
			t.Fatal("expected a queued job")
		}
	}
}

func (e *entorno) estado(t *testing.T, nombre string) dominio.Instancia {
	t.Helper()
	i, ok := e.alm.Obtener(nombre)
	if !ok {
		t.Fatalf("instance %q not stored", nombre)
	}
	return i
}

func zipTemporal(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "subida.zip")
	if err := os.WriteFile(p, []byte("zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func esValidacion(err error) bool {
	var ev *dominio.ErrorValidacion
	return errors.As(err, &ev)
}

func TestCrearHappyPath(t *testing.T) {
	e := nuevoEntorno(t, 3)
	inst, err := e.s.SolicitarCrear("paisajes")
	if err != nil || inst.Estado != dominio.Creando || inst.IP != "192.168.10.30" {
		t.Fatalf("SolicitarCrear = %+v, %v", inst, err)
	}
	e.correr(t, 1)
	if got := e.estado(t, "paisajes"); got.Estado != dominio.Lista || got.Error != "" {
		t.Fatalf("after create: %+v", got)
	}
	want := []string{"Crear paisajes 192.168.10.30/24", "EsperarLista paisajes"}
	if !slices.Equal(e.hip.Llamadas(), want) {
		t.Fatalf("hipervisor calls = %v", e.hip.Llamadas())
	}
	if !slices.Equal(e.dns.Llamadas(), []string{"Agregar paisajes 192.168.10.30", "Resolver paisajes"}) {
		t.Fatalf("dns calls = %v", e.dns.Llamadas())
	}
}

func TestCrearRejectsInvalidNameWithoutCallingAnything(t *testing.T) {
	e := nuevoEntorno(t, 3)
	if _, err := e.s.SolicitarCrear("a;rm"); !esValidacion(err) {
		t.Fatalf("err = %v", err)
	}
	if len(e.s.Listar()) != 0 || len(e.hip.Llamadas()) != 0 {
		t.Fatal("an invalid name must not reserve or call anything")
	}
}

func TestCrearFailures(t *testing.T) {
	tests := []struct {
		name    string
		preparo func(e *entorno)
		mensaje string
	}{
		{"vm creation fails", func(e *entorno) { e.hip.FallarEn("Crear", errors.New("disk locked")) }, "no se pudo crear la máquina virtual: disk locked"},
		{"vm never ready", func(e *entorno) { e.hip.FallarEn("EsperarLista", errors.New("error:linea-12")) }, "no terminó de configurarse"},
		{"dns add fails", func(e *entorno) { e.dns.FallarEn("Agregar", errors.New("ssh down")) }, "no se pudo registrar el nombre en el DNS"},
		{"dns does not resolve", func(e *entorno) { e.dns.FallarEn("Resolver", errors.New("nxdomain")) }, "el DNS no resuelve"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := nuevoEntorno(t, 3)
			tt.preparo(e)
			if _, err := e.s.SolicitarCrear("paisajes"); err != nil {
				t.Fatal(err)
			}
			e.correr(t, 1)
			got := e.estado(t, "paisajes")
			if got.Estado != dominio.Error || !strings.Contains(got.Error, tt.mensaje) {
				t.Fatalf("got %+v, want Error containing %q", got, tt.mensaje)
			}
			if !got.Estado.PuedeEliminar() {
				t.Fatal("a failed instance must be deletable")
			}
		})
	}
}

func TestCrearTimeout(t *testing.T) {
	e := nuevoEntorno(t, 3)
	e.s.o.TiempoCrear = time.Nanosecond
	_, _ = e.s.SolicitarCrear("paisajes")
	e.correr(t, 1)
	if got := e.estado(t, "paisajes"); got.Estado != dominio.Error {
		t.Fatalf("timeout must end in Error, got %+v", got)
	}
}

func TestPublicar(t *testing.T) {
	e := nuevoEntorno(t, 3)
	_, _ = e.s.SolicitarCrear("paisajes")

	zip := zipTemporal(t)
	if err := e.s.SolicitarPublicar("paisajes", zip); !esValidacion(err) {
		t.Fatalf("publishing while creando: %v", err)
	}
	if _, err := os.Stat(zip); !os.IsNotExist(err) {
		t.Fatal("a rejected zip must be deleted")
	}

	e.correr(t, 1)
	zip = zipTemporal(t)
	if err := e.s.SolicitarPublicar("paisajes", zip); err != nil {
		t.Fatal(err)
	}
	if got := e.estado(t, "paisajes"); got.Estado != dominio.Publicando {
		t.Fatalf("state = %s", got.Estado)
	}
	e.correr(t, 1)
	if got := e.estado(t, "paisajes"); got.Estado != dominio.Lista {
		t.Fatalf("after publish: %+v", got)
	}
	if want := "Publicar paisajes 192.168.10.30 " + zip; !slices.Contains(e.pub.Llamadas(), want) {
		t.Fatalf("publicador calls = %v", e.pub.Llamadas())
	}
	if _, err := os.Stat(zip); !os.IsNotExist(err) {
		t.Fatal("the zip must be deleted after publishing")
	}
}

func TestPublicarFailureKeepsInstanceUsable(t *testing.T) {
	e := nuevoEntorno(t, 3)
	_, _ = e.s.SolicitarCrear("paisajes")
	e.correr(t, 1)
	e.pub.FallarEn("Publicar", errors.New("zip inválido"))
	_ = e.s.SolicitarPublicar("paisajes", zipTemporal(t))
	e.correr(t, 1)
	got := e.estado(t, "paisajes")
	if got.Estado != dominio.Lista || !strings.Contains(got.Error, "la última publicación falló") {
		t.Fatalf("got %+v", got)
	}
	e.pub.FallarEn("Publicar", nil)
	_ = e.s.SolicitarPublicar("paisajes", zipTemporal(t))
	e.correr(t, 1)
	if got := e.estado(t, "paisajes"); got.Error != "" {
		t.Fatalf("a successful retry must clear the error: %+v", got)
	}
}

func TestPublicarUnknownInstance(t *testing.T) {
	e := nuevoEntorno(t, 3)
	if err := e.s.SolicitarPublicar("no-existe", zipTemporal(t)); !esValidacion(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestEliminarHappyPath(t *testing.T) {
	e := nuevoEntorno(t, 3)
	_, _ = e.s.SolicitarCrear("paisajes")
	e.correr(t, 1)
	if err := e.s.SolicitarEliminar("paisajes"); err != nil {
		t.Fatal(err)
	}
	e.correr(t, 1)
	if _, ok := e.alm.Obtener("paisajes"); ok {
		t.Fatal("a deleted instance must leave the store")
	}
	if !slices.Contains(e.dns.Llamadas(), "Quitar paisajes") || !slices.Contains(e.hip.Llamadas(), "Eliminar paisajes") {
		t.Fatalf("dns=%v hip=%v", e.dns.Llamadas(), e.hip.Llamadas())
	}
	if !slices.Equal(e.olvidado, []string{"192.168.10.30"}) {
		t.Fatalf("olvidado = %v", e.olvidado)
	}
}

func TestEliminarRejectedWhileCreating(t *testing.T) {
	e := nuevoEntorno(t, 3)
	_, _ = e.s.SolicitarCrear("paisajes")
	if err := e.s.SolicitarEliminar("paisajes"); !esValidacion(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestEliminarTriesEveryStepAndCanBeRetried(t *testing.T) {
	e := nuevoEntorno(t, 3)
	_, _ = e.s.SolicitarCrear("paisajes")
	e.correr(t, 1)
	e.dns.FallarEn("Quitar", errors.New("ns apagado"))
	_ = e.s.SolicitarEliminar("paisajes")
	e.correr(t, 1)
	got := e.estado(t, "paisajes")
	if got.Estado != dominio.Error || !strings.Contains(got.Error, "ns apagado") {
		t.Fatalf("got %+v", got)
	}
	if !slices.Contains(e.hip.Llamadas(), "Eliminar paisajes") || len(e.olvidado) != 1 {
		t.Fatal("a DNS failure must not skip the VM deletion or the host key cleanup")
	}
	e.dns.FallarEn("Quitar", nil)
	if err := e.s.SolicitarEliminar("paisajes"); err != nil {
		t.Fatal(err)
	}
	e.correr(t, 1)
	if _, ok := e.alm.Obtener("paisajes"); ok {
		t.Fatal("retry must finish the deletion")
	}
}

func TestQueueFullReleasesTheName(t *testing.T) {
	e := nuevoEntorno(t, 100)
	for n := range cap(e.s.trabajos) {
		if _, err := e.s.SolicitarCrear(fmt.Sprintf("sitio%02d", n)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.s.SolicitarCrear("extra"); !errors.Is(err, ErrOcupado) {
		t.Fatalf("err = %v, want ErrOcupado", err)
	}
	if _, ok := e.alm.Obtener("extra"); ok {
		t.Fatal("a rejected request must release its name and IP")
	}
}

func TestReconciliar(t *testing.T) {
	e := nuevoEntorno(t, 3)
	_, _ = e.s.SolicitarCrear("paisajes") // VM created below, left mid-creation
	_, _ = e.s.SolicitarCrear("negocios") // VM never created
	_ = e.hip.Crear(context.Background(), puertos.SpecVM{Nombre: "paisajes"})
	if err := e.s.Reconciliar(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := e.estado(t, "paisajes"); got.Estado != dominio.Error || !strings.Contains(got.Error, "se reinició") {
		t.Fatalf("paisajes = %+v", got)
	}
	if got := e.estado(t, "negocios"); got.Estado != dominio.Perdida {
		t.Fatalf("negocios = %+v", got)
	}
}

func TestWorkerRunsUntilCanceled(t *testing.T) {
	e := nuevoEntorno(t, 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.s.Iniciar(ctx)
	_, _ = e.s.SolicitarCrear("paisajes")
	deadline := time.Now().Add(2 * time.Second)
	for e.estado(t, "paisajes").Estado != dominio.Lista {
		if time.Now().After(deadline) {
			t.Fatal("worker did not process the job")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
