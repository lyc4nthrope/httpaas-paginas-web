package web

// Regression tests for the findings of the PR #1 review (2026-11-06).

import (
	"bytes"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
)

// Finding 5: the auto-refresh must reload "/", never the URL of the POST that rendered
// an error page (that route is POST-only and would answer 405).
func TestRefreshAlwaysGoesToTheDashboard(t *testing.T) {
	tmpl := template.Must(template.ParseFS(archivos, "templates/*.html"))
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "dashboard.html", Vista{Refrescar: true, Errores: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `content="3;url=/"`) {
		t.Fatalf("refresh tag must point to /: %s", buf.String()[:400])
	}
}

// Finding 6: DNS rebinding. Only the hosts the app is served on are accepted, for every
// method, and a POST's Origin must be exactly that host.
func TestOnlyAllowedHostsAreServed(t *testing.T) {
	p := nueva(t)
	tests := []struct {
		name, method, ruta, host, origin string
		want                             int
	}{
		{"rebinding GET", http.MethodGet, "/api/instancias", "evil.example:8080", "", http.StatusForbidden},
		{"rebinding POST with matching origin", http.MethodPost, "/instancias", "evil.example:8080", "http://evil.example:8080", http.StatusForbidden},
		{"allowed host", http.MethodGet, "/api/instancias", "example.com", "", http.StatusOK},
		{"allowed host, foreign origin", http.MethodPost, "/instancias", "example.com", "http://evil.example", http.StatusForbidden},
	}
	for _, sitio := range []struct {
		valor string
		want  int
	}{{"cross-site", http.StatusForbidden}, {"same-origin", http.StatusUnprocessableEntity}} {
		req := httptest.NewRequest(http.MethodPost, "/instancias", strings.NewReader("nombre=a"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Sec-Fetch-Site", sitio.valor) // no Origin header
		if rec := p.do(t, req); rec.Code != sitio.want {
			t.Fatalf("Sec-Fetch-Site %s without Origin: code = %d, want %d", sitio.valor, rec.Code, sitio.want)
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.ruta, nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if rec := p.do(t, req); rec.Code != tt.want {
				t.Fatalf("code = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestNoAllowedHostsRefusesEverything(t *testing.T) {
	h := Nuevo(Opciones{Servicio: nueva(t).svc, Dominio: "cloud.local", MaxZipMB: 1, ValidarZip: func(string) error { return nil }})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("without a host allowlist the app must fail closed, got %d", rec.Code)
	}
}

// Finding 9: pressing Aceptar with a too large file must say so, not "invalid name".
func TestCrearWithTooLargeBodyIs413(t *testing.T) {
	p := nueva(t)
	grande := bytes.Repeat([]byte("x"), 3<<20) // MaxZipMB is 1
	rec := p.do(t, formulario(t, "/instancias", map[string]string{dominio.CampoNombre: "paisajes"}, grande))
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "supera el máximo") {
		t.Fatalf("code = %d", rec.Code)
	}
	if len(p.svc.Listar()) != 0 {
		t.Fatal("nothing may be created from a rejected request")
	}
}

// Review 2, finding 5: an unreadable create form is reported on the name field, the only
// field of that action, not on the publish form's file input.
func TestUnreadableCreateFormReportsTheNameField(t *testing.T) {
	p := nueva(t)
	req := httptest.NewRequest(http.MethodPost, "/instancias", strings.NewReader("--x\r\nbroken"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	rec := p.do(t, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d", rec.Code)
	}
	cuerpo := rec.Body.String()
	nombre := strings.Index(cuerpo, `id="nombre"`)
	archivo := strings.Index(cuerpo, `id="archivo"`)
	msg := strings.Index(cuerpo, "no se pudo leer el formulario")
	if msg < nombre || msg > archivo {
		t.Fatalf("the error must be shown under the name field (positions nombre=%d msg=%d archivo=%d)", nombre, msg, archivo)
	}
}

// The ns warning (phase B6.5) reaches the template data on every render.
func TestVistaCarriesTheDNSWarning(t *testing.T) {
	p := nueva(t)
	aviso := "no se contacta la VM DNS en 192.168.10.10"
	s := &servidor{o: Opciones{Servicio: p.svc, DNSAviso: func() string { return aviso }}}
	if got := s.vista().DNSAviso; got != aviso {
		t.Fatalf("Vista.DNSAviso = %q", got)
	}
	s.o.DNSAviso = nil
	if got := s.vista().DNSAviso; got != "" {
		t.Fatalf("without a checker the warning must be empty, got %q", got)
	}
}

// B-12: when ns does not answer, the dashboard shows the warning as an alert, and only then.
func TestDashboardShowsTheDNSWarning(t *testing.T) {
	tmpl := template.Must(template.ParseFS(archivos, "templates/*.html"))
	render := func(aviso string) string {
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "dashboard.html", Vista{DNSAviso: aviso, Errores: map[string]string{}}); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	con := render("no se contacta la VM DNS en 192.168.10.10")
	if !strings.Contains(con, `<p class="aviso-dns" role="alert">no se contacta la VM DNS en 192.168.10.10</p>`) {
		t.Fatal("the DNS warning must be shown as an alert")
	}
	if strings.Contains(render(""), `class="aviso-dns"`) {
		t.Fatal("without a problem there must be no DNS warning")
	}
}
