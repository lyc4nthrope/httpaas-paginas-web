// Command httpaas is the management web app of the local HTTP-as-a-Service platform.
// It provisions Apache VMs from a multiattach disk, registers them in Bind9 and
// deploys user content.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/almacen"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/config"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/instancias"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/web"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("httpaas", flag.ContinueOnError)
	ruta := fs.String("config", "config.json", "configuration file")
	simular := fs.Bool("simular", false, "use simulated VMs, DNS and publishing (no VirtualBox needed)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Cargar(*ruta)
	if err != nil {
		return err
	}
	handler, err := construir(ctx, cfg, *simular)
	if err != nil {
		return err
	}

	srv := &http.Server{Addr: cfg.Listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		apagar, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(apagar)
	}()
	modo := "real"
	if *simular {
		modo = "simulado"
	}
	fmt.Printf("escuchando en http://%s (modo %s)\n", cfg.Listen, modo)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve http: %w", err)
	}
	return nil
}

// construir wires the service. Real adapters arrive in phase A7 (vbox from A6,
// dns from B6, publicar from C7); until then only simulated mode runs.
func construir(ctx context.Context, cfg config.Config, simular bool) (http.Handler, error) {
	if !simular {
		if err := cfg.VerificarArchivos(); err != nil {
			return nil, err
		}
		return nil, errors.New("el modo real se conecta en la fase A7; por ahora usa -simular")
	}

	var rango []string
	for n := cfg.Red.Primera; n <= cfg.Red.Ultima; n++ {
		rango = append(rango, cfg.IP(n))
	}
	alm, err := almacen.Abrir("data/simulado.json", rango) // never mixed with the real store
	if err != nil {
		return nil, err
	}
	hip, dns, pub := simulados(2 * time.Second)
	svc := instancias.Nuevo(instancias.Opciones{
		Hipervisor: hip, DNS: dns, Publicador: pub, Almacen: alm,
		Max: cfg.MaxInstancias, Prefijo: cfg.Red.Prefijo,
	})
	if err := svc.Reconciliar(ctx); err != nil {
		return nil, err
	}
	svc.Iniciar(ctx)
	return web.Nuevo(web.Opciones{
		Servicio: svc, Dominio: cfg.Dominio, Prefijo: cfg.Red.Prefijo, MaxZipMB: cfg.MaxZipMB,
		ValidarZip: validarZipBasico,
	}), nil
}
