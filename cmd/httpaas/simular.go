package main

import (
	"context"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/puertos"
)

// simulados returns the fakes with a delay, so the dashboard shows the states change.
func simulados(d time.Duration) (puertos.Hipervisor, puertos.DNS, puertos.Publicador) {
	return &hipLento{Hipervisor: &puertos.FakeHipervisor{}, d: d}, &puertos.FakeDNS{}, &pubLento{Publicador: &puertos.FakePublicador{}, d: d}
}

type hipLento struct {
	puertos.Hipervisor
	d time.Duration
}

func (h *hipLento) EsperarLista(ctx context.Context, nombre string) error {
	if err := esperar(ctx, h.d); err != nil {
		return err
	}
	return h.Hipervisor.EsperarLista(ctx, nombre)
}

type pubLento struct {
	puertos.Publicador
	d time.Duration
}

func (p *pubLento) Publicar(ctx context.Context, host, ip, zip string) error {
	if err := esperar(ctx, p.d); err != nil {
		return err
	}
	return p.Publicador.Publicar(ctx, host, ip, zip)
}

func esperar(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
