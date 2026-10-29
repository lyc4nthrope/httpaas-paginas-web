// Package dominio holds the core model of the service: an HTTP instance, its lifecycle
// states and the naming rules. It has no dependencies on VirtualBox, DNS or HTTP.
package dominio

import (
	"fmt"
	"regexp"
	"time"
)

// Estado is the lifecycle state of an instance (docs/PLAN.md §4.6).
type Estado string

const (
	Creando    Estado = "creando"
	Lista      Estado = "lista"
	Publicando Estado = "publicando"
	Eliminando Estado = "eliminando"
	Error      Estado = "error"
	Perdida    Estado = "perdida" // in the JSON store but no longer in VirtualBox
)

// PuedePublicar reports whether content may be deployed in this state.
func (e Estado) PuedePublicar() bool { return e == Lista }

// PuedeEliminar reports whether a delete may start in this state. An instance that is
// still being created or already being deleted is owned by the worker.
func (e Estado) PuedeEliminar() bool { return e != Creando && e != Eliminando }

// Instancia is one provisioned Apache VM. Nombre is the hostname and the VM name.
type Instancia struct {
	Nombre string    `json:"nombre"`
	IP     string    `json:"ip"`
	Estado Estado    `json:"estado"`
	Creada time.Time `json:"creada"`
	Error  string    `json:"error,omitempty"` // user-facing detail when Estado is Error
}

// Form field names shared by the HTML templates and the handlers, so they cannot drift.
const (
	CampoNombre  = "nombre"
	CampoArchivo = "archivo"
)

// hostnameRE is a lowercase RFC 1123 label of 3 to 32 characters (decision D10).
var hostnameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`)

// reservados are names used by the infrastructure itself.
var reservados = map[string]bool{
	"ns":               true,
	"www":              true,
	"localhost":        true,
	"apache-plantilla": true,
}

// ErrorValidacion is an invalid user input. Handlers answer it with HTTP 422.
type ErrorValidacion struct {
	Campo   string
	Mensaje string
}

func (e *ErrorValidacion) Error() string { return e.Campo + ": " + e.Mensaje }

// ValidarHostname checks a requested hostname before anything is executed with it.
func ValidarHostname(s string) error {
	if !hostnameRE.MatchString(s) {
		return &ErrorValidacion{
			Campo:   CampoNombre,
			Mensaje: "usa de 3 a 32 caracteres: minúsculas, números y guiones; empieza con letra y no termines en guion",
		}
	}
	if reservados[s] {
		return &ErrorValidacion{Campo: CampoNombre, Mensaje: fmt.Sprintf("%q está reservado para la infraestructura", s)}
	}
	return nil
}

// FQDN returns the fully qualified name of a host inside the domain, e.g. paisajes.cloud.local.
func FQDN(host, dominio string) string { return host + "." + dominio }
