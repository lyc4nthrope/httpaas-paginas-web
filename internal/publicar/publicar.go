package publicar

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/executor"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/puertos"
)

const (
	tiempoMaximo = 5 * time.Minute
	script       = "/usr/local/sbin/httpaas-publicar"
	subidas      = "/home/httpaas/subidas/" // the only folder httpaas-publicar reads from
)

// Publicador implements puertos.Publicador over SSH.
type Publicador struct {
	conectar  func(ip string) executor.Executor // one executor per instance IP
	dominio   string
	comprobar func(ctx context.Context, ip, host string) error // replaceable in tests
}

var _ puertos.Publicador = (*Publicador)(nil)

// Nuevo builds the publisher. conectar returns the executor that reaches an instance, for
// example an executor.SSH with that IP; dominio is cfg.Dominio (e.g. cloud.local).
func Nuevo(conectar func(ip string) executor.Executor, dominio string) *Publicador {
	return &Publicador{conectar: conectar, dominio: dominio, comprobar: comprobarSitio}
}

// Publicar copies an already validated zip to the instance, runs httpaas-publicar and
// checks that Apache serves the site under its own name.
func (p *Publicador) Publicar(ctx context.Context, host, ip, zipLocal string) error {
	if err := dominio.ValidarHostname(host); err != nil {
		return fmt.Errorf("publish: invalid host: %w", err)
	}
	if net.ParseIP(ip).To4() == nil {
		return fmt.Errorf("publish: invalid instance ip %q", ip)
	}
	id, err := idAleatorio()
	if err != nil {
		return err
	}
	remoto := subidas + id + ".zip"
	ex := p.conectar(ip)
	// Copying up to 200 MB, unzipping and reloading Apache must end even when the caller
	// gives no deadline: the single job queue cannot wait forever.
	ctx, cancel := context.WithTimeout(ctx, tiempoMaximo)
	defer cancel()

	if err := ex.CopyTo(ctx, zipLocal, remoto); err != nil {
		return err // a *executor.ChannelError stays as it is
	}
	res, err := ex.Run(ctx, executor.Command{Name: "sudo", Args: []string{script, remoto}})
	if err != nil {
		return err
	}
	detalle := strings.TrimSpace(res.Stderr)
	switch res.ExitCode {
	case 0:
	case 2: // the VM rejected the zip: the user can fix it
		return &dominio.ErrorValidacion{Campo: dominio.CampoArchivo, Mensaje: "la instancia rechazó el zip: " + mensajeCorto(detalle)}
	case 3:
		return fmt.Errorf("la instancia no pudo publicar (Apache o primer arranque): %s", mensajeCorto(detalle))
	default:
		return fmt.Errorf("httpaas-publicar terminó con código %d: %s", res.ExitCode, mensajeCorto(detalle))
	}

	return p.comprobar(ctx, ip, dominio.FQDN(host, p.dominio))
}

// comprobarSitio asks Apache for the site with the instance's name as Host and accepts
// only 200. It never uses a proxy: the instance lives on the local host-only network.
func comprobarSitio(ctx context.Context, direccion, host string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+direccion+"/", nil)
	if err != nil {
		return err
	}
	req.Host = host
	cliente := &http.Client{
		Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true},
		// A redirect is not the site: only a direct 200 counts.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := cliente.Do(req)
	if err != nil {
		return fmt.Errorf("check site %s: %w", host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("check site %s: Apache answered %s", host, resp.Status)
	}
	return nil
}

// idAleatorio is 32 hex characters, which httpaas-publicar's argument rule accepts.
func idAleatorio() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random upload id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// mensajeCorto keeps the script's first line, so a message stays readable on the dashboard.
func mensajeCorto(s string) string {
	linea, _, _ := strings.Cut(s, "\n")
	if linea == "" {
		return "sin detalle"
	}
	return strings.TrimPrefix(linea, "Error: ")
}
