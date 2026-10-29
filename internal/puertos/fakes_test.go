package puertos

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
)

func TestFakeHipervisor(t *testing.T) {
	ctx := context.Background()
	f := &FakeHipervisor{}
	if err := f.Crear(ctx, SpecVM{Nombre: "paisajes", IP: "192.168.10.30", Prefijo: 24}); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Listar(ctx); !slices.Equal(got, []string{"paisajes"}) {
		t.Fatalf("Listar = %v", got)
	}
	if err := f.Eliminar(ctx, "paisajes"); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Listar(ctx); len(got) != 0 {
		t.Fatalf("Listar after Eliminar = %v", got)
	}
	want := []string{"Crear paisajes 192.168.10.30/24", "Listar", "Eliminar paisajes", "Listar"}
	if got := f.Llamadas(); !slices.Equal(got, want) {
		t.Fatalf("Llamadas = %v, want %v", got, want)
	}
}

func TestFakeFallarEn(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")
	f := &FakeHipervisor{}
	f.FallarEn("Crear", boom)
	if err := f.Crear(ctx, SpecVM{Nombre: "x"}); !errors.Is(err, boom) {
		t.Fatalf("Crear = %v, want boom", err)
	}
	if got, _ := f.Listar(ctx); len(got) != 0 {
		t.Fatalf("a failed Crear must not register the VM, got %v", got)
	}
	f.FallarEn("Crear", nil)
	if err := f.Crear(ctx, SpecVM{Nombre: "x"}); err != nil {
		t.Fatalf("after clearing, Crear = %v", err)
	}
}

func TestFakeDNS(t *testing.T) {
	ctx := context.Background()
	f := &FakeDNS{}
	if _, err := f.Resolver(ctx, "paisajes"); err == nil {
		t.Fatal("Resolver of a missing host must fail")
	}
	_ = f.Agregar(ctx, "paisajes", "192.168.10.30")
	if ip, err := f.Resolver(ctx, "paisajes"); err != nil || ip != "192.168.10.30" {
		t.Fatalf("Resolver = %q, %v", ip, err)
	}
	_ = f.Agregar(ctx, "paisajes", "192.168.10.31") // idempotent replace
	if ip, _ := f.Resolver(ctx, "paisajes"); ip != "192.168.10.31" {
		t.Fatalf("Agregar must replace, got %q", ip)
	}
	if err := f.Quitar(ctx, "no-existe"); err != nil {
		t.Fatalf("Quitar of a missing host must not fail: %v", err)
	}
}

func TestFakesConcurrent(t *testing.T) {
	ctx := context.Background()
	d := &FakeDNS{}
	p := &FakePublicador{}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = d.Agregar(ctx, "h", "192.168.10.30")
			_ = p.Publicar(ctx, "h", "192.168.10.30", "/tmp/a.zip")
		}()
	}
	wg.Wait()
	if len(d.Llamadas()) != 20 || len(p.Llamadas()) != 20 {
		t.Fatalf("calls lost: dns=%d publicar=%d", len(d.Llamadas()), len(p.Llamadas()))
	}
}
