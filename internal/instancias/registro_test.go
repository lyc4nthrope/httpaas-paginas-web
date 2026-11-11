package instancias

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/oplog"
)

type bitacoraFalsa struct {
	mu       sync.Mutex
	entradas []oplog.Entry
	err      error
}

func (b *bitacoraFalsa) Add(e oplog.Entry) (oplog.Entry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entradas = append(b.entradas, e)
	return e, b.err
}

func (b *bitacoraFalsa) ultima(t *testing.T) oplog.Entry {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entradas) == 0 {
		t.Fatal("no operation was logged")
	}
	return b.entradas[len(b.entradas)-1]
}

func conBitacora(t *testing.T) (*entorno, *bitacoraFalsa) {
	e := nuevoEntorno(t, 3)
	b := &bitacoraFalsa{}
	e.s.o.Registro = b
	return e, b
}

func TestEveryJobIsLogged(t *testing.T) {
	e, b := conBitacora(t)
	if _, err := e.s.SolicitarCrear("paisajes"); err != nil {
		t.Fatal(err)
	}
	e.correr(t, 1)
	c := b.ultima(t)
	if c.Action != "Crear instancia" || c.Target != "paisajes" || !c.OK || c.Detail != "" || c.Message == "" {
		t.Fatalf("create entry = %+v", c)
	}

	zip := filepath.Join(t.TempDir(), "sitio.zip")
	if err := os.WriteFile(zip, []byte("zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.s.SolicitarPublicar("paisajes", zip); err != nil {
		t.Fatal(err)
	}
	e.correr(t, 1)
	if p := b.ultima(t); p.Action != "Publicar contenido" || !p.OK {
		t.Fatalf("publish entry = %+v", p)
	}

	if err := e.s.SolicitarEliminar("paisajes"); err != nil {
		t.Fatal(err)
	}
	e.correr(t, 1)
	if d := b.ultima(t); d.Action != "Eliminar instancia" || !d.OK {
		t.Fatalf("delete entry = %+v", d)
	}
}

// A failure keeps the message the user sees and, apart, the technical cause.
func TestFailedJobLogsMessageAndCause(t *testing.T) {
	e, b := conBitacora(t)
	e.hip.FallarEn("Crear", errors.New("disk locked"))
	if _, err := e.s.SolicitarCrear("paisajes"); err != nil {
		t.Fatal(err)
	}
	e.correr(t, 1)
	c := b.ultima(t)
	if c.OK || c.Message != "no se pudo crear la máquina virtual" || !strings.Contains(c.Detail, "disk locked") {
		t.Fatalf("entry = %+v", c)
	}
}

func TestFailedPublishIsLoggedAsFailed(t *testing.T) {
	e, b := conBitacora(t)
	if _, err := e.s.SolicitarCrear("paisajes"); err != nil {
		t.Fatal(err)
	}
	e.correr(t, 1)
	e.pub.FallarEn("Publicar", &dominio.ErrorValidacion{Campo: dominio.CampoArchivo, Mensaje: "falta index.html"})
	zip := filepath.Join(t.TempDir(), "sitio.zip")
	if err := os.WriteFile(zip, []byte("zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.s.SolicitarPublicar("paisajes", zip); err != nil {
		t.Fatal(err)
	}
	e.correr(t, 1)
	p := b.ultima(t)
	if p.OK || !strings.Contains(p.Message, "falta index.html") {
		t.Fatalf("entry = %+v", p)
	}
}

// The commands the adapters run during a job arrive through the job's context.
func TestJobStepsComeFromTheContext(t *testing.T) {
	e, b := conBitacora(t)
	e.s.o.Olvidar = func(ctx context.Context, ip string) error {
		oplog.Anotar(ctx, oplog.Step{Cmd: "ssh-keygen -R " + ip})
		return nil
	}
	if _, err := e.s.SolicitarCrear("paisajes"); err != nil {
		t.Fatal(err)
	}
	e.correr(t, 1)
	if err := e.s.SolicitarEliminar("paisajes"); err != nil {
		t.Fatal(err)
	}
	e.correr(t, 1)
	d := b.ultima(t)
	if len(d.Steps) != 1 || d.Steps[0].Cmd != "ssh-keygen -R 192.168.10.30" || d.Channel != "ssh-keygen" {
		t.Fatalf("entry = %+v", d)
	}
}

// If the log cannot be written, the job still finishes and the problem is reported.
func TestLogWriteErrorIsReported(t *testing.T) {
	e, b := conBitacora(t)
	b.err = errors.New("disk full")
	var avisos []string
	e.s.o.Logf = func(f string, a ...any) { avisos = append(avisos, f) }
	if _, err := e.s.SolicitarCrear("paisajes"); err != nil {
		t.Fatal(err)
	}
	e.correr(t, 1)
	if e.estado(t, "paisajes").Estado != dominio.Lista {
		t.Fatal("a log failure must not change the job's result")
	}
	if len(avisos) == 0 {
		t.Fatal("the log write error must be reported")
	}
}
