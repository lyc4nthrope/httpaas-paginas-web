// Package dns will drive deploy/ns/httpaas-dns over SSH (phase B6). Until that
// adapter exists, this file pins the contract of the script itself: it is the sudo
// boundary on the ns VM (decision D7), so its argument validation is part of the
// security surface, not an implementation detail.
package dns

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// bashRuta returns a bash that actually runs. exec.LookPath alone is not enough on
// Windows: the PATH entry resolves to the WSL stub, which fails without a distro.
func bashRuta(t *testing.T) string {
	t.Helper()

	candidatos := []string{"bash"}
	if runtime.GOOS == "windows" {
		candidatos = append(candidatos,
			`C:\Program Files\Git\bin\bash.exe`,
			`C:\Program Files\Git\usr\bin\bash.exe`,
		)
	}
	for _, candidato := range candidatos {
		ruta, err := exec.LookPath(candidato)
		if err != nil {
			continue
		}
		if err := exec.Command(ruta, "-c", "exit 0").Run(); err == nil {
			return ruta
		}
	}
	t.Skip("no working bash on this machine; CI runs this test on Linux")
	return ""
}

// scriptRuta locates the script relative to this source file, so the test does not
// depend on the working directory go test happens to run in.
func scriptRuta(t *testing.T) string {
	t.Helper()

	_, este, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	ruta := filepath.Join(filepath.Dir(este), "..", "..", "deploy", "ns", "httpaas-dns")
	if _, err := os.Stat(ruta); err != nil {
		t.Fatalf("script under test not found at %s: %v", ruta, err)
	}
	return ruta
}

// ejecutar runs the script with args. PATH is replaced by path so the script can
// never reach a nameserver or a zone transfer from a test.
func ejecutar(t *testing.T, bash, script, path string, args []string) (int, string) {
	t.Helper()

	cmd := exec.Command(bash, append([]string{script}, args...)...)
	if path != "" {
		cmd.Env = append(os.Environ(), "PATH="+path)
	}
	var salida strings.Builder
	cmd.Stderr = &salida

	err := cmd.Run()
	codigo := 0
	if err != nil {
		var salidaErr *exec.ExitError
		if !errors.As(err, &salidaErr) {
			t.Fatalf("running %v: %v", args, err)
		}
		codigo = salidaErr.ExitCode()
	}
	return codigo, salida.String()
}

// rechazo reports whether the script died inside its own validation. die() prints
// "httpaas-dns: ..." at the start of a line; bash's own "command not found" errors
// prefix the whole script path, so only the line prefix tells them apart.
func rechazo(salida string) bool {
	for _, linea := range strings.Split(salida, "\n") {
		if strings.HasPrefix(linea, "httpaas-dns: ") {
			return true
		}
	}
	return false
}

// TestValidacionDeArgumentos pins every rejection path. These cases all exit before
// calling nsupdate or dig, so they are deterministic on any machine.
func TestValidacionDeArgumentos(t *testing.T) {
	bash := bashRuta(t)
	script := scriptRuta(t)

	casos := []struct {
		nombre  string
		args    []string
		codigo  int
		mensaje string
	}{
		// The reserved infrastructure addresses are inside IP_RE, so only the
		// explicit list can stop them. Losing that list breaks the DNS server's
		// own PTR in the reverse zone, silently.
		{"ip reservada del dns", []string{"agregar", "prueba", "192.168.10.10"}, 1, "ip '192.168.10.10' is reserved"},
		{"ip reservada del host", []string{"agregar", "prueba", "192.168.10.1"}, 1, "ip '192.168.10.1' is reserved"},
		{"ip reservada de la plantilla", []string{"agregar", "prueba", "192.168.10.20"}, 1, "ip '192.168.10.20' is reserved"},

		{"ip de red", []string{"agregar", "prueba", "192.168.10.0"}, 1, "invalid ip"},
		{"ip de broadcast", []string{"agregar", "prueba", "192.168.10.255"}, 1, "invalid ip"},
		{"ip de otra red", []string{"agregar", "prueba", "10.0.0.5"}, 1, "invalid ip"},
		{"ip con letras", []string{"agregar", "prueba", "192.168.10.3x"}, 1, "invalid ip"},

		{"host reservado", []string{"agregar", "ns", "192.168.10.30"}, 1, "host 'ns' is reserved"},
		{"host con punto", []string{"agregar", "a.b", "192.168.10.30"}, 1, "invalid host"},
		{"host con mayusculas", []string{"agregar", "Web1", "192.168.10.30"}, 1, "invalid host"},
		{"host terminado en guion", []string{"agregar", "web1-", "192.168.10.30"}, 1, "invalid host"},

		{"falta la ip", []string{"agregar", "prueba"}, 2, "usage: httpaas-dns"},
		{"sobra un argumento", []string{"agregar", "prueba", "192.168.10.30", "extra"}, 2, "usage: httpaas-dns"},
		{"quitar sin host", []string{"quitar"}, 2, "usage: httpaas-dns"},
		{"subcomando desconocido", []string{"borrar", "prueba"}, 2, "usage: httpaas-dns"},
		{"sin argumentos", nil, 2, "usage: httpaas-dns"},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			codigo, salida := ejecutar(t, bash, script, "", caso.args)
			if codigo != caso.codigo {
				t.Errorf("exit = %d, want %d (stderr: %s)", codigo, caso.codigo, salida)
			}
			if !strings.Contains(salida, caso.mensaje) {
				t.Errorf("stderr = %q, want it to contain %q", salida, caso.mensaje)
			}
		})
	}
}

// TestIpValidaAceptada pins the other half of the contract: a legal instance
// address must reach nsupdate, otherwise a broken validator that rejects
// everything would still pass every case above.
//
// PATH is pointed at an empty directory so nsupdate cannot be resolved and the
// test can never touch a real nameserver. Reaching "command not found" is the
// proof that validar_ip accepted the address.
func TestIpValidaAceptada(t *testing.T) {
	bash := bashRuta(t)
	script := scriptRuta(t)
	sinHerramientas := t.TempDir()

	for _, ip := range []string{"192.168.10.30", "192.168.10.99", "192.168.10.254"} {
		t.Run(ip, func(t *testing.T) {
			codigo, salida := ejecutar(t, bash, script, sinHerramientas, []string{"agregar", "prueba", ip})
			if rechazo(salida) {
				t.Errorf("a legal address was rejected by validation: %s", salida)
			}
			if codigo == 0 {
				t.Errorf("exit = 0 while nsupdate is absent; the script must not report success")
			}
			if !strings.Contains(salida, "nsupdate: command not found") {
				t.Errorf("validation did not reach nsupdate (exit %d): %s", codigo, salida)
			}
		})
	}
}
