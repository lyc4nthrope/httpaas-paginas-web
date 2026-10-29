// Package puertos defines the ports the orchestrator depends on (docs/PLAN.md §4.2).
// Each part implements one adapter: vbox (A), dns (B) and publicar (C). The fakes in
// this package let every part work and test without real VMs.
package puertos

import "context"

// Hipervisor creates, inspects and deletes instance VMs. Implemented by internal/vbox.
type Hipervisor interface {
	// Crear registers the VM on the multiattach disk, writes the guest properties and starts it.
	Crear(ctx context.Context, spec SpecVM) error
	// EsperarLista blocks until /httpaas/estado == "listo" or ctx ends; "error:*" is returned as an error.
	EsperarLista(ctx context.Context, nombre string) error
	// Eliminar powers off (if running) and deletes the VM with its differencing disk.
	// A VM that does not exist is not an error.
	Eliminar(ctx context.Context, nombre string) error
	// Listar returns the names of the VMs registered in VirtualBox.
	Listar(ctx context.Context) ([]string, error)
}

// SpecVM describes the VM to create. Values are already validated.
type SpecVM struct {
	Nombre  string // hostname and VM name
	IP      string // e.g. "192.168.10.30"
	Prefijo int    // e.g. 24
}

// DNS adds and removes A records in the zone. Implemented by internal/dns.
type DNS interface {
	Agregar(ctx context.Context, host, ip string) error        // idempotent: replaces an existing record
	Quitar(ctx context.Context, host string) error             // idempotent: missing record is not an error
	Resolver(ctx context.Context, host string) (string, error) // asks the zone's server directly
}

// Publicador deploys an already validated zip to an instance. Implemented by internal/publicar.
type Publicador interface {
	Publicar(ctx context.Context, host, ip, zipLocal string) error
}
