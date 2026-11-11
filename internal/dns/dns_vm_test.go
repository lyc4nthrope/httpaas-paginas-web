//go:build vm

// Tests that need the real ns VM. Run with: make vm-test
package dns

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/config"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/executor"
)

// configVM loads the config the app would use. It skips the test when there is
// none, so a machine without the real environment still runs make vm-test.
func configVM(t *testing.T) config.Config {
	t.Helper()

	ruta := os.Getenv("HTTPAAS_CONFIG")
	if ruta == "" {
		ruta = "../../config.json"
	}
	cfg, err := config.Cargar(ruta)
	if err != nil {
		t.Skipf("no usable config at %s: %v", ruta, err)
	}
	return cfg
}

// nuevoVM builds the adapter with the real SSH channel to the ns VM.
func nuevoVM(cfg config.Config) *DNS {
	ssh := &executor.SSH{
		Host:       cfg.DNS.IP,
		User:       cfg.DNS.Usuario,
		KeyPath:    cfg.SSH.Llave,
		KnownHosts: cfg.SSH.KnownHosts,
	}
	return Nuevo(ssh, cfg)
}

// TestVMRegistrarResolverQuitar walks the whole adapter against the real server:
// the helper writes the record over SSH and the private resolver reads it back.
func TestVMRegistrarResolverQuitar(t *testing.T) {
	cfg := configVM(t)
	d := nuevoVM(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	host := "prueba-vm"
	ip := cfg.IP(cfg.Red.Ultima - 1)

	// Leave the zone as it was even when the test fails half way.
	t.Cleanup(func() {
		limpieza, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelar()
		if err := d.Quitar(limpieza, host); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})

	if err := d.Agregar(ctx, host, ip); err != nil {
		t.Fatalf("Agregar: %v", err)
	}

	got, err := d.Resolver(ctx, host)
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}
	if got != ip {
		t.Errorf("Resolver = %q, want %q", got, ip)
	}

	if err := d.Quitar(ctx, host); err != nil {
		t.Fatalf("Quitar: %v", err)
	}
	if _, err := d.Resolver(ctx, host); err == nil {
		t.Error("the name still resolves after Quitar")
	}
}

// TestVMComprobar is the B6.5 check with ns running: the diagnostic must pass.
// The failing half is covered by the unit tests with an injected lookup, because
// powering the VM off inside a test would make the suite destructive.
func TestVMComprobar(t *testing.T) {
	cfg := configVM(t)
	d := nuevoVM(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := d.Comprobar(ctx); err != nil {
		t.Fatalf("Comprobar: %v", err)
	}
}
