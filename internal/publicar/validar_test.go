package publicar

import (
	"archive/zip"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
)

// entrada describes one file of a zip built for a test.
type entrada struct {
	nombre string
	modo   fs.FileMode
	datos  string
}

func zipCon(t *testing.T, entradas ...entrada) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "sitio.zip")
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entradas {
		h := &zip.FileHeader{Name: e.nombre, Method: zip.Deflate}
		if e.modo != 0 {
			h.SetMode(e.modo)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.datos)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func indexEn(nombre string) entrada { return entrada{nombre: nombre, datos: "<h1>hola</h1>"} }

func esValidacion(err error) bool {
	var ev *dominio.ErrorValidacion
	return errors.As(err, &ev) && ev.Campo == dominio.CampoArchivo
}

// The zips Wiliam built (testdata/zips/generar.sh) get the verdict httpaas-publicar gives
// them inside the VM. The parity cases below were compared against real unzip 6.0.
func TestValidarTestdataZips(t *testing.T) {
	tests := []struct {
		archivo string
		ok      bool
		raiz    string
	}{
		{"paisajes.zip", true, "."},
		{"negocios.zip", true, "negocios"}, // index.html inside a single folder (C4.1.4)
		{"sin-index.zip", false, ""},
		{"zipslip.zip", false, ""},
		{"symlink.zip", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.archivo, func(t *testing.T) {
			r, err := Validar(filepath.Join("..", "..", "testdata", "zips", tt.archivo), LimitesPorDefecto)
			if tt.ok && (err != nil || r.Raiz != tt.raiz) {
				t.Fatalf("Validar = %+v, %v; want root %q", r, err, tt.raiz)
			}
			if !tt.ok && !esValidacion(err) {
				t.Fatalf("Validar = %v, want an ErrorValidacion on %s", err, dominio.CampoArchivo)
			}
		})
	}
}

func TestValidarRejects(t *testing.T) {
	muchos := make([]entrada, 0, 2002)
	muchos = append(muchos, indexEn("index.html"))
	for n := range 2001 {
		muchos = append(muchos, entrada{nombre: fmt.Sprintf("f%04d.txt", n), datos: "x"})
	}
	tests := []struct {
		name     string
		entradas []entrada
		mensaje  string
	}{
		{"absolute path", []entrada{indexEn("index.html"), {nombre: "/etc/passwd", datos: "x"}}, "rutas peligrosas"},
		{"parent directory", []entrada{indexEn("index.html"), {nombre: "../../tmp/x", datos: "x"}}, "rutas peligrosas"},
		{"dots inside a name, like the VM script", []entrada{indexEn("index.html"), {nombre: "a..b.txt", datos: "x"}}, "rutas peligrosas"},
		{"backslash path", []entrada{indexEn("index.html"), {nombre: `..\..\x`, datos: "x"}}, "rutas peligrosas"},
		{"backslash without dots", []entrada{indexEn("index.html"), {nombre: `css\estilo.css`, datos: "x"}}, "rutas peligrosas"},
		{"symlink", []entrada{indexEn("index.html"), {nombre: "link", modo: fs.ModeSymlink | 0o777, datos: "/etc/passwd"}}, "enlace"},
		{"named pipe", []entrada{indexEn("index.html"), {nombre: "fifo", modo: fs.ModeNamedPipe | 0o644}}, "tipo"},
		{"more than 2000 entries", muchos, "2000"},
		{"no index.html", []entrada{{nombre: "otra.html", datos: "x"}}, "index.html"},
		{"index.html two levels down", []entrada{indexEn("a/b/index.html")}, "index.html"},
		{"two top-level folders", []entrada{indexEn("a/index.html"), indexEn("b/index.html")}, "index.html"},
		{"empty zip", nil, "vacío"},
		// Review 2026-11-10: each of these was accepted here and rejected by the VM.
		{"symlink named like a folder", []entrada{indexEn("index.html"), {nombre: "link/", modo: fs.ModeSymlink | 0o777}}, "enlace"},
		{"file and folder with the same name", []entrada{{nombre: "sitio", datos: "x"}, indexEn("sitio/index.html")}, "archivo y carpeta"},
		{"folder before file with the same name", []entrada{indexEn("sitio/index.html"), {nombre: "sitio", datos: "x"}}, "archivo y carpeta"},
		{"folder mode without a trailing slash", []entrada{{nombre: "sitio", modo: fs.ModeDir | 0o755}, indexEn("sitio/index.html")}, "carpeta"},
		{"duplicate name", []entrada{indexEn("index.html"), indexEn("index.html")}, "repetido"},
		{"duplicate after cleaning", []entrada{indexEn("index.html"), indexEn("./index.html")}, "repetido"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Validar(zipCon(t, tt.entradas...), LimitesPorDefecto)
			if !esValidacion(err) || !strings.Contains(err.Error(), tt.mensaje) {
				t.Fatalf("Validar = %v, want an ErrorValidacion containing %q", err, tt.mensaje)
			}
		})
	}
}

func TestValidarSizeLimitUsesTheUncompressedSize(t *testing.T) {
	grande := strings.Repeat("a", 2048) // compresses to a few bytes
	ruta := zipCon(t, indexEn("index.html"), entrada{nombre: "grande.txt", datos: grande})
	if _, err := Validar(ruta, Limites{MaxArchivos: 10, MaxBytes: 1024}); !esValidacion(err) || !strings.Contains(err.Error(), "MB") {
		t.Fatalf("a zip over the uncompressed limit must be rejected: %v", err)
	}
	if _, err := Validar(ruta, Limites{MaxArchivos: 10, MaxBytes: 4096}); err != nil {
		t.Fatalf("the same zip under the limit: %v", err)
	}
}

func TestValidarAccepts(t *testing.T) {
	tests := []struct {
		name     string
		entradas []entrada
		raiz     string
		archivos int
	}{
		{"index at the root", []entrada{indexEn("index.html"), {nombre: "css/a.css", datos: "b{}"}}, ".", 2},
		{"single folder", []entrada{{nombre: "sitio/", modo: fs.ModeDir | 0o755}, indexEn("sitio/index.html")}, "sitio", 1},
		{"single folder plus a hidden file", []entrada{indexEn("sitio/index.html"), {nombre: ".DS_Store", datos: "x"}}, "sitio", 2},
		// unzip cleans these names, and the VM accepts them; so must we.
		{"dot slash at the root", []entrada{indexEn("./index.html")}, ".", 1},
		{"dot slash in a folder", []entrada{indexEn("./sitio/index.html")}, "sitio", 1},
		{"double slash", []entrada{indexEn("sitio//index.html")}, "sitio", 1},
		{"dot segment", []entrada{indexEn("sitio/./index.html")}, "sitio", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Validar(zipCon(t, tt.entradas...), LimitesPorDefecto)
			if err != nil || r.Raiz != tt.raiz || r.Archivos != tt.archivos {
				t.Fatalf("Validar = %+v, %v; want root %q and %d files", r, err, tt.raiz, tt.archivos)
			}
		})
	}
}

func TestValidarEntryLimitBoundary(t *testing.T) {
	entradas := []entrada{indexEn("index.html")}
	for n := range 1999 {
		entradas = append(entradas, entrada{nombre: fmt.Sprintf("f%04d.txt", n), datos: "x"})
	}
	if _, err := Validar(zipCon(t, entradas...), LimitesPorDefecto); err != nil {
		t.Fatalf("exactly 2000 entries must be accepted: %v", err)
	}
	entradas = append(entradas, entrada{nombre: "f9999.txt", datos: "x"})
	if _, err := Validar(zipCon(t, entradas...), LimitesPorDefecto); !esValidacion(err) {
		t.Fatalf("2001 entries must be rejected: %v", err)
	}
}

func TestValidarNotAZip(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "no.zip")
	_ = os.WriteFile(ruta, []byte("esto no es un zip"), 0o600)
	if _, err := Validar(ruta, LimitesPorDefecto); !esValidacion(err) {
		t.Fatalf("Validar = %v", err)
	}
}

// ValidarZip is the function the web layer receives (web.Opciones.ValidarZip).
func TestValidarZipFitsTheWebLayer(t *testing.T) {
	var f func(string) error = ValidarZip
	if err := f(filepath.Join("..", "..", "testdata", "zips", "paisajes.zip")); err != nil {
		t.Fatal(err)
	}
	if err := f(filepath.Join("..", "..", "testdata", "zips", "zipslip.zip")); !esValidacion(err) {
		t.Fatalf("ValidarZip(zipslip) = %v", err)
	}
}
