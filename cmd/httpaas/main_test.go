package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/config"
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

func TestRealModeChecksTheFilesFirst(t *testing.T) {
	_, _, err := construir(context.Background(), ejemplo(t), false)
	if err == nil || !strings.Contains(err.Error(), "ssh.llave") {
		t.Fatalf("real mode without the key and the base disk must not start: %v", err)
	}
}

// With the files in place, real mode wires every adapter and serves the dashboard. At
// startup it asks VirtualBox for its VMs (Reconciliar) and checks ns. VBoxManage is a
// fake script here, so the test does not depend on VirtualBox being installed (CI has
// none), and nothing answers at the ns address.
func TestRealModeServes(t *testing.T) {
	cfg := ejemplo(t)
	dir := t.TempDir()
	cfg.SSH.Llave = filepath.Join(dir, "llave")
	cfg.SSH.KnownHosts = filepath.Join(dir, "known_hosts")
	cfg.VBox.DiscoBase = filepath.Join(dir, "ApacheServer.vdi")
	cfg.Datos = filepath.Join(dir, "instancias.json")
	cfg.Logs = filepath.Join(dir, "logs")
	cfg.DNS.IP = "127.0.0.2" // nothing answers there: the ns warning must appear, not a crash
	cfg.VBox.VBoxManage = filepath.Join(dir, "VBoxManage")
	if err := os.WriteFile(cfg.VBox.VBoxManage, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil { // "no VMs"
		t.Fatal(err)
	}
	for _, f := range []string{cfg.SSH.Llave, cfg.VBox.DiscoBase} {
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	h, trabajador, err := construir(ctx, cfg, false)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer func() { cancel(); <-trabajador }()
	req := httptest.NewRequest(http.MethodGet, "/salud", nil)
	req.Host = cfg.Listen
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestSimulatedModeServes(t *testing.T) {
	cfg := ejemplo(t)    // read before changing directory: the path is relative
	t.Chdir(t.TempDir()) // data/simulado.json goes to a temp dir
	ctx, cancel := context.WithCancel(context.Background())
	h, trabajador, err := construir(ctx, cfg, true)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer func() { cancel(); <-trabajador }() // before t.Chdir's temp dir is removed
	for host, want := range map[string]int{cfg.Listen: http.StatusOK, "localhost:8080": http.StatusOK, "evil.example:8080": http.StatusForbidden} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("Host %s: code=%d, want %d", host, rec.Code, want)
		}
		if want == http.StatusOK && !strings.Contains(rec.Body.String(), "Gestión de páginas y aplicaciones web") {
			t.Fatal("dashboard not rendered")
		}
	}
}

func TestHostsPermitidos(t *testing.T) {
	tests := map[string][]string{
		"127.0.0.1:8080":    {"127.0.0.1:8080", "127.0.0.1:8080", "localhost:8080"},
		"192.168.10.1:9000": {"192.168.10.1:9000"},
		"sin-puerto":        nil,
	}
	for listen, want := range tests {
		if got := hostsPermitidos(listen); !slices.Equal(got, want) {
			t.Errorf("hostsPermitidos(%q) = %q, want %q", listen, got, want)
		}
	}
}

// The service writes one line per operation to logs/operaciones.jsonl (phase A3).
func TestOperationsAreLogged(t *testing.T) {
	cfg := ejemplo(t)
	t.Chdir(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	h, trabajador, err := construir(ctx, cfg, true)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer func() { cancel(); <-trabajador }()
	req := httptest.NewRequest(http.MethodPost, "/instancias", strings.NewReader("nombre=web"))
	req.Host = cfg.Listen
	req.Header.Set("Origin", "http://"+cfg.Listen)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create: code=%d %s", rec.Code, rec.Body.String())
	}
	ruta := filepath.Join(cfg.Logs, "operaciones.jsonl")
	for fin := time.Now().Add(20 * time.Second); time.Now().Before(fin); time.Sleep(100 * time.Millisecond) {
		if b, _ := os.ReadFile(ruta); strings.Contains(string(b), `"action":"Crear instancia"`) && strings.Contains(string(b), `"target":"web"`) {
			return
		}
	}
	b, _ := os.ReadFile(ruta)
	t.Fatalf("the create operation was not logged in %s: %q", ruta, b)
}
