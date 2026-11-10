// Package dns implements puertos.DNS over SSH: it drives deploy/ns/httpaas-dns on
// the ns VM and asks that same server for the records it wrote.
//
// Two independent channels are used on purpose. Writes go through the helper over
// SSH, which is the only thing the httpaas user may run with sudo (decision D7).
// Reads go straight to the zone's server with a private resolver, so the answer
// never depends on how the machine running the webapp resolves names (B5).
package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/config"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/executor"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/puertos"
)

// RutaScript is where deploy/ns/instalar.sh installs the helper on the ns VM.
const RutaScript = "/usr/local/sbin/httpaas-dns"

// TiempoConsulta bounds one DNS lookup. A server that does not answer must not
// block the worker that is creating an instance.
const TiempoConsulta = 5 * time.Second

// DNS is safe for concurrent use: the executor serialises the SSH channel and
// net.Resolver is safe for concurrent lookups.
type DNS struct {
	cfg      config.Config
	ex       executor.Executor
	servidor string // host:port of the zone's server, built once from the config
	resolver *net.Resolver
	lookup   func(ctx context.Context, fqdn string) ([]string, error)
}

var _ puertos.DNS = (*DNS)(nil)

// Nuevo builds the adapter. It does not connect: the SSH channel and the DNS
// server are only reached when a method is called.
func Nuevo(ex executor.Executor, cfg config.Config) *DNS {
	d := &DNS{
		cfg:      cfg,
		ex:       ex,
		servidor: net.JoinHostPort(cfg.DNS.IP, "53"),
	}
	dialer := &net.Dialer{}
	d.resolver = &net.Resolver{
		PreferGo: true,
		// Ignore the address the resolver would pick and always dial the zone's
		// server. PreferGo keeps the pure Go client, which honours this Dial.
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "udp", d.servidor)
		},
	}
	d.lookup = func(ctx context.Context, fqdn string) ([]string, error) {
		return d.resolver.LookupHost(ctx, fqdn)
	}
	return d
}

// Agregar registers host -> ip. Both values are validated before the SSH command
// is built, so an invalid name or an address outside the instance range never
// reaches the helper. It is idempotent: an existing record is replaced.
func (d *DNS) Agregar(ctx context.Context, host, ip string) error {
	if err := dominio.ValidarHostname(host); err != nil {
		return err
	}
	if !d.cfg.EnRango(ip) {
		return fmt.Errorf("ip %q is outside the instance range %s%d-%s%d",
			ip, d.cfg.Red.Base, d.cfg.Red.Primera, d.cfg.Red.Base, d.cfg.Red.Ultima)
	}
	return d.ejecutar(ctx, "agregar", host, ip)
}

// Quitar removes host from both zones. A record that does not exist is not an
// error, which is what makes a retried deletion safe.
func (d *DNS) Quitar(ctx context.Context, host string) error {
	if err := dominio.ValidarHostname(host); err != nil {
		return err
	}
	return d.ejecutar(ctx, "quitar", host)
}

// Resolver returns the address the zone's server reports for host. host is the
// bare label (the same value Agregar takes), not a FQDN: the domain is appended
// here so callers cannot build "name.cloud.local.cloud.local".
//
// It deliberately does not call dominio.ValidarHostname: the startup check
// (B6.5) resolves "ns", which is reserved for Agregar and Quitar but must still
// be readable. The name only reaches net.Resolver, never a command line.
func (d *DNS) Resolver(ctx context.Context, host string) (string, error) {
	if host == "" {
		return "", fmt.Errorf("resolver: empty host")
	}
	ctx, cancel := context.WithTimeout(ctx, TiempoConsulta)
	defer cancel()

	fqdn := dominio.FQDN(host, d.cfg.Dominio)
	ips, err := d.lookup(ctx, fqdn)
	if err != nil {
		return "", fmt.Errorf("resolver %s: %w", fqdn, err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("resolver %s: no address", fqdn)
	}
	return ips[0], nil
}

// ejecutar runs one subcommand of the helper. The command is built argument by
// argument; nothing is ever joined into a shell line.
func (d *DNS) ejecutar(ctx context.Context, sub string, args ...string) error {
	c := executor.Command{
		Name: "sudo",
		Args: append([]string{RutaScript, sub}, args...),
	}
	res, err := d.ex.Run(ctx, c)
	if err != nil {
		return err
	}
	// A remote non-zero exit is a result, not a transport error: the executor only
	// returns an error when the command could not be delivered.
	if res.ExitCode != 0 {
		detalle := strings.TrimSpace(res.Stderr)
		if detalle == "" {
			detalle = fmt.Sprintf("exit code %d", res.ExitCode)
		}
		return fmt.Errorf("httpaas-dns %s: %s", sub, detalle)
	}
	return nil
}
