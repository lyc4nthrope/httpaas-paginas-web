package main

import (
	"archive/zip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/config"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
)

func ejemplo(t *testing.T) config.Config {
	t.Helper()
	c, err := config.Cargar(filepath.Join("..", "..", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRunRejectsMissingConfig(t *testing.T) {
	err := run(context.Background(), []string{"-config", filepath.Join(t.TempDir(), "no-existe.json")})
	if err == nil {
		t.Fatal("a missing config must stop the app")
	}
}

func TestRealModeIsNotWiredYet(t *testing.T) {
	_, err := construir(context.Background(), ejemplo(t), false)
	if err == nil {
		t.Fatal("real mode must refuse to start until phase A7")
	}
}

func TestSimulatedModeServes(t *testing.T) {
	cfg := ejemplo(t)    // read before changing directory: the path is relative
	t.Chdir(t.TempDir()) // data/simulado.json goes to a temp dir
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := construir(ctx, cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Gestión de páginas y aplicaciones web") {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestValidarZipBasico(t *testing.T) {
	dir := t.TempDir()
	noZip := filepath.Join(dir, "a.zip")
	_ = os.WriteFile(noZip, []byte("not a zip"), 0o600)
	var ev *dominio.ErrorValidacion
	if err := validarZipBasico(noZip); !errors.As(err, &ev) {
		t.Fatalf("not a zip: %v", err)
	}

	ok := filepath.Join(dir, "b.zip")
	f, _ := os.Create(ok)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("index.html")
	_, _ = w.Write([]byte("<h1>hola</h1>"))
	_ = zw.Close()
	_ = f.Close()
	if err := validarZipBasico(ok); err != nil {
		t.Fatalf("valid zip: %v", err)
	}
}
