package dns

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/config"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/executor"
)

// cfgPrueba mirrors config.example.json: the instance range is .30-.99, so the
// infrastructure addresses .1, .10 and .20 are outside it.
func cfgPrueba() config.Config {
	return config.Config{
		Dominio: "cloud.local",
		Red:     config.Red{Base: "192.168.10.", Prefijo: 24, Primera: 30, Ultima: 99},
		DNS:     config.DNS{IP: "192.168.10.10", Usuario: "httpaas"},
	}
}

func nuevoFalso(t *testing.T) (*DNS, *executor.Fake) {
	t.Helper()
	f := &executor.Fake{}
	return Nuevo(f, cfgPrueba()), f
}

func TestAgregarConstruyeElComandoExacto(t *testing.T) {
	d, f := nuevoFalso(t)

	if err := d.Agregar(context.Background(), "paisajes", "192.168.10.30"); err != nil {
		t.Fatalf("Agregar: %v", err)
	}

	llamadas := f.Calls()
	if len(llamadas) != 1 {
		t.Fatalf("calls = %d, want 1", len(llamadas))
	}
	quiere := executor.Command{
		Name: "sudo",
		Args: []string{"/usr/local/sbin/httpaas-dns", "agregar", "paisajes", "192.168.10.30"},
	}
	if !reflect.DeepEqual(llamadas[0], quiere) {
		t.Errorf("command = %#v, want %#v", llamadas[0], quiere)
	}
}

func TestQuitarConstruyeElComandoExacto(t *testing.T) {
	d, f := nuevoFalso(t)

	if err := d.Quitar(context.Background(), "paisajes"); err != nil {
		t.Fatalf("Quitar: %v", err)
	}

	llamadas := f.Calls()
	if len(llamadas) != 1 {
		t.Fatalf("calls = %d, want 1", len(llamadas))
	}
	quiere := executor.Command{
		Name: "sudo",
		Args: []string{"/usr/local/sbin/httpaas-dns", "quitar", "paisajes"},
	}
	if !reflect.DeepEqual(llamadas[0], quiere) {
		t.Errorf("command = %#v, want %#v", llamadas[0], quiere)
	}
}

// TestAgregarRechazaAntesDeEjecutar is the security half of the adapter: an
// invalid name or an address outside the range must never reach sudo.
func TestAgregarRechazaAntesDeEjecutar(t *testing.T) {
	hosts := []string{"", "ns", "www", "localhost", "apache-plantilla", "a.b", "Web1", "web1-", "ab", "x;id", "web 1"}
	ips := []string{"", "192.168.10.1", "192.168.10.10", "192.168.10.20", "192.168.10.29", "192.168.10.100", "10.0.0.5", "abc", "192.168.10.3x"}

	for _, host := range hosts {
		t.Run("host "+host, func(t *testing.T) {
			d, f := nuevoFalso(t)
			err := d.Agregar(context.Background(), host, "192.168.10.30")
			var validacion *dominio.ErrorValidacion
			if !errors.As(err, &validacion) {
				t.Fatalf("err = %v, want *dominio.ErrorValidacion", err)
			}
			if llamadas := f.Calls(); len(llamadas) != 0 {
				t.Errorf("the executor was called with an invalid host: %#v", llamadas)
			}
		})
	}

	for _, ip := range ips {
		t.Run("ip "+ip, func(t *testing.T) {
			d, f := nuevoFalso(t)
			if err := d.Agregar(context.Background(), "paisajes", ip); err == nil {
				t.Fatalf("Agregar accepted ip %q", ip)
			}
			if llamadas := f.Calls(); len(llamadas) != 0 {
				t.Errorf("the executor was called with an invalid ip: %#v", llamadas)
			}
		})
	}
}

func TestQuitarRechazaHostInvalido(t *testing.T) {
	for _, host := range []string{"", "ns", "www", "a.b", "Web1"} {
		t.Run(host, func(t *testing.T) {
			d, f := nuevoFalso(t)
			if err := d.Quitar(context.Background(), host); err == nil {
				t.Fatalf("Quitar accepted host %q", host)
			}
			if llamadas := f.Calls(); len(llamadas) != 0 {
				t.Errorf("the executor was called with an invalid host: %#v", llamadas)
			}
		})
	}
}

// TestExitCodeDistintoDeCeroEsError pins the trap of this adapter: a remote
// failure is a Result, not an error, so the adapter must inspect the code.
func TestExitCodeDistintoDeCeroEsError(t *testing.T) {
	d, f := nuevoFalso(t)
	f.RunFunc = func(executor.Command) (executor.Result, error) {
		return executor.Result{ExitCode: 1, Stderr: "httpaas-dns: ip '192.168.10.10' is reserved\n"}, nil
	}

	err := d.Agregar(context.Background(), "paisajes", "192.168.10.30")
	if err == nil {
		t.Fatal("a non-zero exit was reported as success")
	}
	if quiere := "is reserved"; !strings.Contains(err.Error(), quiere) {
		t.Errorf("err = %q, want it to contain %q", err, quiere)
	}
}

func TestExitCodeSinStderr(t *testing.T) {
	d, f := nuevoFalso(t)
	f.RunFunc = func(executor.Command) (executor.Result, error) {
		return executor.Result{ExitCode: 3}, nil
	}

	err := d.Quitar(context.Background(), "paisajes")
	if err == nil {
		t.Fatal("a non-zero exit was reported as success")
	}
	if quiere := "exit code 3"; !strings.Contains(err.Error(), quiere) {
		t.Errorf("err = %q, want it to contain %q", err, quiere)
	}
}

func TestErrorDeTransporteSePropaga(t *testing.T) {
	d, f := nuevoFalso(t)
	caida := &executor.ChannelError{Msg: "no channel to ns"}
	f.RunFunc = func(executor.Command) (executor.Result, error) {
		return executor.Result{}, caida
	}

	err := d.Agregar(context.Background(), "paisajes", "192.168.10.30")
	if !errors.Is(err, caida) {
		t.Fatalf("err = %v, want the channel error", err)
	}
}

func TestNuevoApuntaAlServidorDeLaZona(t *testing.T) {
	d := Nuevo(&executor.Fake{}, cfgPrueba())
	if quiere := "192.168.10.10:53"; d.servidor != quiere {
		t.Errorf("servidor = %q, want %q", d.servidor, quiere)
	}
}

func TestResolverPideElFqdn(t *testing.T) {
	d, _ := nuevoFalso(t)
	var pedido string
	d.lookup = func(_ context.Context, fqdn string) ([]string, error) {
		pedido = fqdn
		return []string{"192.168.10.30"}, nil
	}

	ip, err := d.Resolver(context.Background(), "paisajes")
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}
	if quiere := "paisajes.cloud.local"; pedido != quiere {
		t.Errorf("lookup asked for %q, want %q", pedido, quiere)
	}
	if ip != "192.168.10.30" {
		t.Errorf("ip = %q, want 192.168.10.30", ip)
	}
}

// TestResolverAceptaNs is the B6.5 contract: the startup check resolves the
// reserved name, so Resolver must not apply the Agregar validation.
func TestResolverAceptaNs(t *testing.T) {
	d, _ := nuevoFalso(t)
	var pedido string
	d.lookup = func(_ context.Context, fqdn string) ([]string, error) {
		pedido = fqdn
		return []string{"192.168.10.10"}, nil
	}

	if _, err := d.Resolver(context.Background(), "ns"); err != nil {
		t.Fatalf("Resolver(ns): %v", err)
	}
	if quiere := "ns.cloud.local"; pedido != quiere {
		t.Errorf("lookup asked for %q, want %q", pedido, quiere)
	}
}

func TestResolverErrorSeEnvuelve(t *testing.T) {
	d, _ := nuevoFalso(t)
	fallo := errors.New("server misbehaving")
	d.lookup = func(context.Context, string) ([]string, error) { return nil, fallo }

	_, err := d.Resolver(context.Background(), "paisajes")
	if !errors.Is(err, fallo) {
		t.Fatalf("err = %v, want the lookup error", err)
	}
	if quiere := "paisajes.cloud.local"; !strings.Contains(err.Error(), quiere) {
		t.Errorf("err = %q, want it to name the fqdn", err)
	}
}

func TestResolverSinDireccion(t *testing.T) {
	d, _ := nuevoFalso(t)
	d.lookup = func(context.Context, string) ([]string, error) { return nil, nil }

	if _, err := d.Resolver(context.Background(), "paisajes"); err == nil {
		t.Fatal("an empty answer was reported as an address")
	}
}

func TestResolverHostVacioNoConsulta(t *testing.T) {
	d, _ := nuevoFalso(t)
	consultado := false
	d.lookup = func(context.Context, string) ([]string, error) {
		consultado = true
		return []string{"192.168.10.30"}, nil
	}

	if _, err := d.Resolver(context.Background(), ""); err == nil {
		t.Fatal("an empty host was accepted")
	}
	if consultado {
		t.Error("the resolver was asked for an empty host")
	}
}
