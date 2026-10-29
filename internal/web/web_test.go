package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/almacen"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/instancias"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/puertos"
)

type prueba struct {
	h        http.Handler
	svc      *instancias.Servicio
	pub      *puertos.FakePublicador
	cancel   context.CancelFunc
	validado []string // paths given to ValidarZip
	zipErr   error
}

func nueva(t *testing.T) *prueba {
	t.Helper()
	var r []string
	for n := 30; n <= 40; n++ {
		r = append(r, fmt.Sprintf("192.168.10.%d", n))
	}
	alm, err := almacen.Abrir(filepath.Join(t.TempDir(), "i.json"), r)
	if err != nil {
		t.Fatal(err)
	}
	p := &prueba{pub: &puertos.FakePublicador{}}
	p.svc = instancias.Nuevo(instancias.Opciones{
		Hipervisor: &puertos.FakeHipervisor{}, DNS: &puertos.FakeDNS{}, Publicador: p.pub,
		Almacen: alm, Max: 3, Prefijo: 24, Logf: func(string, ...any) {},
	})
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	t.Cleanup(cancel)
	p.svc.Iniciar(ctx)
	p.h = Nuevo(Opciones{
		Servicio: p.svc, Dominio: "cloud.local", Prefijo: 24, MaxZipMB: 1, Logf: func(string, ...any) {},
		ValidarZip: func(ruta string) error {
			p.validado = append(p.validado, ruta)
			return p.zipErr
		},
	})
	return p
}

func (p *prueba) do(t *testing.T, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	p.h.ServeHTTP(rec, req)
	return rec
}

func formulario(t *testing.T, ruta string, campos map[string]string, archivo []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range campos {
		_ = mw.WriteField(k, v)
	}
	if archivo != nil {
		fw, _ := mw.CreateFormFile(dominio.CampoArchivo, "sitio.zip")
		_, _ = fw.Write(archivo)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, ruta, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func (p *prueba) esperarEstado(t *testing.T, nombre string, e dominio.Estado) {
	t.Helper()
	limite := time.Now().Add(2 * time.Second)
	for time.Now().Before(limite) {
		for _, i := range p.svc.Listar() {
			if i.Nombre == nombre && i.Estado == e {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s never reached %s", nombre, e)
}

func TestDashboardEmpty(t *testing.T) {
	p := nueva(t)
	rec := p.do(t, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Aún no hay instancias") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("security headers missing")
	}
}

func TestCrearRedirectsAndShowsTheInstance(t *testing.T) {
	p := nueva(t)
	rec := p.do(t, formulario(t, "/instancias", map[string]string{dominio.CampoNombre: "paisajes"}, nil))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/?aviso=creada&nombre=paisajes" {
		t.Fatalf("code=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	p.esperarEstado(t, "paisajes", dominio.Lista)
	body := p.do(t, httptest.NewRequest(http.MethodGet, "/?aviso=creada&nombre=paisajes", nil)).Body.String()
	for _, want := range []string{"Instancia paisajes en creación", `href="http://paisajes.cloud.local/"`, `target="_blank"`, "192.168.10.30/24"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard lacks %q", want)
		}
	}
}

func TestCrearInvalidNameIs422AndEscaped(t *testing.T) {
	p := nueva(t)
	rec := p.do(t, formulario(t, "/instancias", map[string]string{dominio.CampoNombre: "<script>x</script>"}, nil))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<script>x") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("the refilled value must be HTML-escaped")
	}
	if !strings.Contains(body, "minúsculas") {
		t.Fatal("the validation message must be shown")
	}
}

func TestAvisoNeverShowsFreeText(t *testing.T) {
	p := nueva(t)
	body := p.do(t, httptest.NewRequest(http.MethodGet, "/?aviso=creada&nombre=Hackeado%20llama%20al%20555", nil)).Body.String()
	if strings.Contains(body, "Hackeado") {
		t.Fatal("a redirect message must only accept valid hostnames")
	}
}

func TestForeignOriginIsRefused(t *testing.T) {
	p := nueva(t)
	req := formulario(t, "/instancias", map[string]string{dominio.CampoNombre: "paisajes"}, nil)
	req.Header.Set("Origin", "http://evil.example")
	if rec := p.do(t, req); rec.Code != http.StatusForbidden {
		t.Fatalf("foreign origin: code = %d", rec.Code)
	}
	req = formulario(t, "/instancias", map[string]string{dominio.CampoNombre: "paisajes"}, nil)
	req.Header.Set("Origin", "http://"+req.Host)
	if rec := p.do(t, req); rec.Code != http.StatusSeeOther {
		t.Fatalf("same origin: code = %d", rec.Code)
	}
}

func TestPublicar(t *testing.T) {
	p := nueva(t)
	p.do(t, formulario(t, "/instancias", map[string]string{dominio.CampoNombre: "paisajes"}, nil))
	p.esperarEstado(t, "paisajes", dominio.Lista)

	rec := p.do(t, formulario(t, "/contenido", map[string]string{dominio.CampoNombre: "paisajes"}, []byte("PK zip")))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	p.esperarEstado(t, "paisajes", dominio.Lista)
	if len(p.pub.Llamadas()) != 1 || !strings.HasPrefix(p.pub.Llamadas()[0], "Publicar paisajes 192.168.10.30 ") {
		t.Fatalf("publicador calls = %v", p.pub.Llamadas())
	}
}

func TestPublicarRejections(t *testing.T) {
	p := nueva(t)
	if rec := p.do(t, formulario(t, "/contenido", map[string]string{dominio.CampoNombre: "paisajes"}, nil)); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("no file: code = %d", rec.Code)
	}

	p.zipErr = &dominio.ErrorValidacion{Campo: dominio.CampoArchivo, Mensaje: "el zip contiene rutas peligrosas"}
	rec := p.do(t, formulario(t, "/contenido", map[string]string{dominio.CampoNombre: "paisajes"}, []byte("PK")))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "rutas peligrosas") {
		t.Fatalf("invalid zip: code=%d", rec.Code)
	}
	if _, err := os.Stat(p.validado[len(p.validado)-1]); !os.IsNotExist(err) {
		t.Fatal("a rejected upload must be deleted")
	}

	grande := bytes.Repeat([]byte("x"), 3<<20) // MaxZipMB is 1
	if rec := p.do(t, formulario(t, "/contenido", map[string]string{dominio.CampoNombre: "paisajes"}, grande)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large: code = %d", rec.Code)
	}
}

func TestEliminar(t *testing.T) {
	p := nueva(t)
	p.do(t, formulario(t, "/instancias", map[string]string{dominio.CampoNombre: "paisajes"}, nil))
	p.esperarEstado(t, "paisajes", dominio.Lista)
	rec := p.do(t, httptest.NewRequest(http.MethodPost, "/instancias/paisajes/eliminar", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code = %d", rec.Code)
	}
	limite := time.Now().Add(2 * time.Second)
	for len(p.svc.Listar()) > 0 && time.Now().Before(limite) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(p.svc.Listar()) != 0 {
		t.Fatal("instance not deleted")
	}
	if rec := p.do(t, httptest.NewRequest(http.MethodPost, "/instancias/no-existe/eliminar", nil)); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown instance: code = %d", rec.Code)
	}
}

func TestAPI(t *testing.T) {
	p := nueva(t)
	rec := p.do(t, httptest.NewRequest(http.MethodGet, "/api/instancias", nil))
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("empty api = %q", rec.Body.String())
	}
	p.do(t, formulario(t, "/instancias", map[string]string{dominio.CampoNombre: "paisajes"}, nil))
	var filas []Fila
	if err := json.NewDecoder(p.do(t, httptest.NewRequest(http.MethodGet, "/api/instancias", nil)).Body).Decode(&filas); err != nil {
		t.Fatal(err)
	}
	if len(filas) != 1 || filas[0].URL != "http://paisajes.cloud.local/" {
		t.Fatalf("api = %+v", filas)
	}
}
