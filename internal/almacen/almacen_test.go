package almacen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
)

var t0 = time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)

func rango(primera, ultima int) []string {
	var r []string
	for n := primera; n <= ultima; n++ {
		r = append(r, fmt.Sprintf("192.168.10.%d", n))
	}
	return r
}

func abrir(t *testing.T, r []string) (*Almacen, string) {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "data", "instancias.json")
	a, err := Abrir(ruta, r)
	if err != nil {
		t.Fatal(err)
	}
	return a, ruta
}

func esValidacion(err error) bool {
	var ev *dominio.ErrorValidacion
	return errors.As(err, &ev)
}

func TestReservarGivesLowestFreeIP(t *testing.T) {
	a, _ := abrir(t, rango(30, 32))
	p, _ := a.Reservar("paisajes", 3, t0)
	n, _ := a.Reservar("negocios", 3, t0.Add(time.Minute))
	if p.IP != "192.168.10.30" || n.IP != "192.168.10.31" || p.Estado != dominio.Creando {
		t.Fatalf("got %+v and %+v", p, n)
	}
	if err := a.Quitar("paisajes"); err != nil {
		t.Fatal(err)
	}
	r, _ := a.Reservar("recetas", 3, t0.Add(2*time.Minute))
	if r.IP != "192.168.10.30" {
		t.Fatalf("a freed IP must be reused first, got %s", r.IP)
	}
}

func TestReservarRejects(t *testing.T) {
	a, _ := abrir(t, rango(30, 31))
	if _, err := a.Reservar("paisajes", 5, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Reservar("paisajes", 5, t0); !esValidacion(err) {
		t.Fatalf("repeated name: %v", err)
	}
	if _, err := a.Reservar("negocios", 1, t0); !esValidacion(err) {
		t.Fatalf("over max: %v", err)
	}
	if _, err := a.Reservar("negocios", 5, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Reservar("recetas", 5, t0); !esValidacion(err) {
		t.Fatalf("range full: %v", err)
	}
}

func TestPersistsAcrossRestarts(t *testing.T) {
	a, ruta := abrir(t, rango(30, 40))
	_, _ = a.Reservar("negocios", 3, t0.Add(time.Minute))
	_, _ = a.Reservar("paisajes", 3, t0)
	_, _ = a.Actualizar("paisajes", func(i *dominio.Instancia) { i.Estado = dominio.Lista })

	b, err := Abrir(ruta, rango(30, 40))
	if err != nil {
		t.Fatal(err)
	}
	l := b.Listar()
	if len(l) != 2 || l[0].Nombre != "paisajes" || l[0].Estado != dominio.Lista || l[1].Nombre != "negocios" {
		t.Fatalf("after reopen, oldest first: %+v", l)
	}
	if info, _ := os.Stat(ruta); info.Mode().Perm() != 0o600 {
		t.Fatalf("store permissions = %v, want 0600", info.Mode().Perm())
	}
}

func TestActualizarCannotChangeNameOrIP(t *testing.T) {
	a, _ := abrir(t, rango(30, 31))
	_, _ = a.Reservar("paisajes", 3, t0)
	got, _ := a.Actualizar("paisajes", func(i *dominio.Instancia) { i.Nombre, i.IP = "otro", "1.2.3.4" })
	if got.Nombre != "paisajes" || got.IP != "192.168.10.30" {
		t.Fatalf("identity changed: %+v", got)
	}
	if _, err := a.Actualizar("no-existe", func(*dominio.Instancia) {}); !errors.Is(err, ErrNoExiste) {
		t.Fatalf("missing: %v", err)
	}
	if err := a.Quitar("no-existe"); !errors.Is(err, ErrNoExiste) {
		t.Fatalf("missing: %v", err)
	}
}

func TestCorruptFileIsNotOverwritten(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "instancias.json")
	if err := os.WriteFile(ruta, []byte("{no es json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Abrir(ruta, rango(30, 31)); err == nil {
		t.Fatal("corrupt file must be an error")
	}
	if data, _ := os.ReadFile(ruta); string(data) != "{no es json" {
		t.Fatalf("corrupt file was modified: %q", data)
	}
}

func TestFailedSaveRollsBack(t *testing.T) {
	dir := t.TempDir()
	a, err := Abrir(filepath.Join(dir, "instancias.json"), rango(30, 31))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil { // read-only dir: the temp file cannot be created
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	if _, err := a.Reservar("paisajes", 3, t0); err == nil {
		t.Fatal("save into a read-only dir must fail")
	}
	if _, ok := a.Obtener("paisajes"); ok {
		t.Fatal("a failed save must not leave the instance in memory")
	}
}

func TestConcurrentReservationsNeverShareAnIP(t *testing.T) {
	a, _ := abrir(t, rango(30, 99))
	var wg sync.WaitGroup
	for n := 0; n < 40; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = a.Reservar(fmt.Sprintf("sitio%02d", n), 100, t0)
		}()
	}
	wg.Wait()
	vistas := map[string]bool{}
	for _, i := range a.Listar() {
		if vistas[i.IP] {
			t.Fatalf("IP %s given twice", i.IP)
		}
		vistas[i.IP] = true
	}
	if len(vistas) != 40 {
		t.Fatalf("got %d instances, want 40", len(vistas))
	}
}
