package publicar

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/executor"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/puertos"
)

var _ puertos.Publicador = (*Publicador)(nil) // fails to compile if the contract changes

// rutaSubida is the argument httpaas-publicar accepts (deploy/plantilla, C4.1.1).
var rutaSubida = regexp.MustCompile(`^/home/httpaas/subidas/[a-zA-Z0-9-]{8,64}\.zip$`)

type caso struct {
	fake      *executor.Fake
	conectado []string // IPs the publisher asked an executor for
	pedidos   []string // Host headers the check sent
	estado    int      // status the check answers
}

func nuevoCaso(t *testing.T, runFunc func(executor.Command) (executor.Result, error)) (*Publicador, *caso) {
	t.Helper()
	c := &caso{fake: &executor.Fake{RunFunc: runFunc}, estado: http.StatusOK}
	p := Nuevo(func(ip string) executor.Executor { c.conectado = append(c.conectado, ip); return c.fake }, "cloud.local")
	p.comprobar = func(_ context.Context, ip, host string) error {
		c.pedidos = append(c.pedidos, ip+" "+host)
		if c.estado != http.StatusOK {
			return errors.New("estado " + http.StatusText(c.estado))
		}
		return nil
	}
	return p, c
}

func TestPublicarSendsTheZipAndRunsTheScript(t *testing.T) {
	p, c := nuevoCaso(t, nil)
	if err := p.Publicar(context.Background(), "paisajes", "192.168.10.30", "/tmp/subida.zip"); err != nil {
		t.Fatal(err)
	}
	if len(c.conectado) != 1 || c.conectado[0] != "192.168.10.30" {
		t.Fatalf("executor requested for %v", c.conectado)
	}
	copias := c.fake.Copies()
	if len(copias) != 1 || !copias[0].ToVM || copias[0].Local != "/tmp/subida.zip" || !rutaSubida.MatchString(copias[0].Remote) {
		t.Fatalf("copies = %+v", copias)
	}
	llamadas := c.fake.Calls()
	if len(llamadas) != 1 {
		t.Fatalf("calls = %+v", llamadas)
	}
	got := llamadas[0]
	if got.Name != "sudo" || len(got.Args) != 2 || got.Args[0] != "/usr/local/sbin/httpaas-publicar" || got.Args[1] != copias[0].Remote {
		t.Fatalf("command = %+v: must be sudo + the script + the uploaded path, as separate arguments", got)
	}
	if len(c.pedidos) != 1 || c.pedidos[0] != "192.168.10.30 paisajes.cloud.local" {
		t.Fatalf("check requests = %v", c.pedidos)
	}
}

func TestPublicarUsesANewNameEachTime(t *testing.T) {
	p, c := nuevoCaso(t, nil)
	_ = p.Publicar(context.Background(), "paisajes", "192.168.10.30", "/tmp/a.zip")
	_ = p.Publicar(context.Background(), "paisajes", "192.168.10.30", "/tmp/a.zip")
	if cs := c.fake.Copies(); cs[0].Remote == cs[1].Remote {
		t.Fatal("two uploads must never share the remote name")
	}
}

func TestPublicarMapsTheScriptExitCodes(t *testing.T) {
	tests := []struct {
		name       string
		code       int
		validacion bool
		mensaje    string
	}{
		{"2 = invalid zip", 2, true, "no se encontro index.html"},
		{"3 = instance problem", 3, false, "la instancia no pudo publicar"},
		{"other code", 9, false, "código 9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, c := nuevoCaso(t, func(executor.Command) (executor.Result, error) {
				return executor.Result{ExitCode: tt.code, Stderr: "Error: no se encontro index.html en la raiz del sitio.\n"}, nil
			})
			err := p.Publicar(context.Background(), "paisajes", "192.168.10.30", "/tmp/a.zip")
			var ev *dominio.ErrorValidacion
			if got := errors.As(err, &ev); got != tt.validacion {
				t.Fatalf("err = %v: ErrorValidacion = %v, want %v", err, got, tt.validacion)
			}
			if err == nil || !strings.Contains(err.Error(), tt.mensaje) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.mensaje)
			}
			if len(c.pedidos) != 0 {
				t.Fatal("a failed publish must not be checked over HTTP")
			}
		})
	}
}

func TestPublicarKeepsChannelErrors(t *testing.T) {
	canal := &executor.ChannelError{Msg: "No se pudo comunicar por SSH", Err: errors.New("connection refused")}
	t.Run("copy", func(t *testing.T) {
		p, c := nuevoCaso(t, nil)
		c.fake.CopyErr = canal
		err := p.Publicar(context.Background(), "paisajes", "192.168.10.30", "/tmp/a.zip")
		var ce *executor.ChannelError
		if !errors.As(err, &ce) || len(c.fake.Calls()) != 0 {
			t.Fatalf("err = %v, calls = %v: a failed copy must stop before the script", err, c.fake.Calls())
		}
	})
	t.Run("run", func(t *testing.T) {
		p, _ := nuevoCaso(t, func(executor.Command) (executor.Result, error) { return executor.Result{ExitCode: 255}, canal })
		var ce *executor.ChannelError
		if err := p.Publicar(context.Background(), "paisajes", "192.168.10.30", "/tmp/a.zip"); !errors.As(err, &ce) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPublicarFailsWhenTheSiteDoesNotAnswer(t *testing.T) {
	p, c := nuevoCaso(t, nil)
	c.estado = http.StatusNotFound
	if err := p.Publicar(context.Background(), "paisajes", "192.168.10.30", "/tmp/a.zip"); err == nil {
		t.Fatal("a site that does not answer 200 must be an error")
	}
}

func TestPublicarRejectsAnInvalidHostOrIP(t *testing.T) {
	for _, tt := range []struct{ host, ip string }{{"a;rm", "192.168.10.30"}, {"paisajes", "192.168.10.30;id"}, {"paisajes", ""}} {
		p, c := nuevoCaso(t, nil)
		if err := p.Publicar(context.Background(), tt.host, tt.ip, "/tmp/a.zip"); err == nil || len(c.conectado) != 0 {
			t.Fatalf("Publicar(%q, %q) = %v, connected %v: must fail before connecting", tt.host, tt.ip, err, c.conectado)
		}
	}
}

// limite records whether the context of each call has a deadline.
type limite struct {
	executor.Fake
	sinLimite bool
}

func (l *limite) CopyTo(ctx context.Context, a, b string) error {
	if _, ok := ctx.Deadline(); !ok {
		l.sinLimite = true
	}
	return l.Fake.CopyTo(ctx, a, b)
}

func (l *limite) Run(ctx context.Context, c executor.Command) (executor.Result, error) {
	if _, ok := ctx.Deadline(); !ok {
		l.sinLimite = true
	}
	return l.Fake.Run(ctx, c)
}

// A copy or a remote unzip that hangs must not block the single job queue forever.
func TestPublicarHasATimeLimit(t *testing.T) {
	ex := &limite{}
	p := Nuevo(func(string) executor.Executor { return ex }, "cloud.local")
	p.comprobar = func(context.Context, string, string) error { return nil }
	if err := p.Publicar(context.Background(), "paisajes", "192.168.10.30", "/tmp/a.zip"); err != nil {
		t.Fatal(err)
	}
	if ex.sinLimite {
		t.Fatal("CopyTo and Run must run with a deadline even when the caller gives none")
	}
}

// comprobarSitio talks real HTTP: it must send the instance's Host header and accept 200 only.
func TestComprobarSitioSendsTheInstanceHost(t *testing.T) {
	var host string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host = r.Host
		if r.Host != "paisajes.cloud.local" {
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := strings.TrimPrefix(srv.URL, "http://")
	if err := comprobarSitio(context.Background(), dir, "paisajes.cloud.local"); err != nil || host != "paisajes.cloud.local" {
		t.Fatalf("err = %v, Host sent = %q", err, host)
	}
	if err := comprobarSitio(context.Background(), dir, "otro.cloud.local"); err == nil {
		t.Fatal("a 404 must be an error")
	}
	// A redirect is not the site: only a direct 200 counts.
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/otra", http.StatusFound)
		}
	}))
	defer redir.Close()
	if err := comprobarSitio(context.Background(), strings.TrimPrefix(redir.URL, "http://"), "paisajes.cloud.local"); err == nil {
		t.Fatal("a redirect that ends in 200 must not count as the site")
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	cerrado := l.Addr().String()
	l.Close()
	if err := comprobarSitio(context.Background(), cerrado, "paisajes.cloud.local"); err == nil {
		t.Fatal("a closed port must be an error")
	}
}
