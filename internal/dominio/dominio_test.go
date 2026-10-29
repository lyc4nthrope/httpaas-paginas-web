package dominio

import (
	"errors"
	"strings"
	"testing"
)

func TestValidarHostname(t *testing.T) {
	tests := []struct {
		in string
		ok bool
	}{
		{"paisajes", true},
		{"mi-sitio2", true},
		{"abc", true},
		{"a" + strings.Repeat("b", 31), true}, // 32 characters
		{"", false},
		{"a", false},
		{"ab", false},
		{"Paisajes", false},
		{"-sitio", false},
		{"sitio-", false},
		{"2sitio", false},
		{"x_y", false},
		{"a;rm", false},
		{"pai sajes", false},
		{"paisajes.cloud", false},
		{"a" + strings.Repeat("b", 32), false}, // 33 characters
		{"ns", false},
		{"www", false},
		{"localhost", false},
		{"apache-plantilla", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			err := ValidarHostname(tt.in)
			if tt.ok && err != nil {
				t.Fatalf("ValidarHostname(%q) = %v, want nil", tt.in, err)
			}
			if !tt.ok {
				var ev *ErrorValidacion
				if !errors.As(err, &ev) || ev.Campo != CampoNombre {
					t.Fatalf("ValidarHostname(%q) = %v, want *ErrorValidacion on %q", tt.in, err, CampoNombre)
				}
			}
		})
	}
}

func TestEstadoPermisos(t *testing.T) {
	tests := []struct {
		e                  Estado
		publicar, eliminar bool
	}{
		{Creando, false, false},
		{Lista, true, true},
		{Publicando, false, true},
		{Eliminando, false, false},
		{Error, false, true},
		{Perdida, false, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.e), func(t *testing.T) {
			if got := tt.e.PuedePublicar(); got != tt.publicar {
				t.Errorf("PuedePublicar() = %v, want %v", got, tt.publicar)
			}
			if got := tt.e.PuedeEliminar(); got != tt.eliminar {
				t.Errorf("PuedeEliminar() = %v, want %v", got, tt.eliminar)
			}
		})
	}
}

func TestFQDN(t *testing.T) {
	if got := FQDN("paisajes", "cloud.local"); got != "paisajes.cloud.local" {
		t.Fatalf("FQDN = %q", got)
	}
}
