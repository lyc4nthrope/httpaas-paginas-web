package puertos

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// registro records calls and returns programmed errors. Safe for concurrent use.
type registro struct {
	mu       sync.Mutex
	llamadas []string
	errores  map[string]error
}

// FallarEn makes every later call to metodo return err (nil clears it).
func (r *registro) FallarEn(metodo string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.errores == nil {
		r.errores = map[string]error{}
	}
	r.errores[metodo] = err
}

// Llamadas returns the calls made so far, formatted as "Metodo arg1 arg2".
func (r *registro) Llamadas() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.llamadas...)
}

func (r *registro) anotar(metodo string, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.llamadas = append(r.llamadas, strings.TrimSpace(metodo+" "+strings.Join(args, " ")))
	return r.errores[metodo]
}

// FakeHipervisor keeps the VMs in memory.
type FakeHipervisor struct {
	registro
	vms map[string]SpecVM
}

var _ Hipervisor = (*FakeHipervisor)(nil)

func (f *FakeHipervisor) Crear(_ context.Context, spec SpecVM) error {
	if err := f.anotar("Crear", spec.Nombre, fmt.Sprintf("%s/%d", spec.IP, spec.Prefijo)); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.vms == nil {
		f.vms = map[string]SpecVM{}
	}
	f.vms[spec.Nombre] = spec
	return nil
}

func (f *FakeHipervisor) EsperarLista(ctx context.Context, nombre string) error {
	if err := f.anotar("EsperarLista", nombre); err != nil {
		return err
	}
	return ctx.Err()
}

func (f *FakeHipervisor) Eliminar(_ context.Context, nombre string) error {
	if err := f.anotar("Eliminar", nombre); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.vms, nombre)
	return nil
}

func (f *FakeHipervisor) Listar(_ context.Context) ([]string, error) {
	if err := f.anotar("Listar"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	nombres := make([]string, 0, len(f.vms))
	for n := range f.vms {
		nombres = append(nombres, n)
	}
	return nombres, nil
}

// FakeDNS keeps the zone's A records in memory.
type FakeDNS struct {
	registro
	registros map[string]string
}

var _ DNS = (*FakeDNS)(nil)

func (f *FakeDNS) Agregar(_ context.Context, host, ip string) error {
	if err := f.anotar("Agregar", host, ip); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.registros == nil {
		f.registros = map[string]string{}
	}
	f.registros[host] = ip
	return nil
}

func (f *FakeDNS) Quitar(_ context.Context, host string) error {
	if err := f.anotar("Quitar", host); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.registros, host)
	return nil
}

func (f *FakeDNS) Resolver(_ context.Context, host string) (string, error) {
	if err := f.anotar("Resolver", host); err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	ip, ok := f.registros[host]
	if !ok {
		return "", fmt.Errorf("resolve %s: no such host", host)
	}
	return ip, nil
}

// FakePublicador only records what would be deployed.
type FakePublicador struct {
	registro
}

var _ Publicador = (*FakePublicador)(nil)

func (f *FakePublicador) Publicar(_ context.Context, host, ip, zipLocal string) error {
	return f.anotar("Publicar", host, ip, zipLocal)
}
