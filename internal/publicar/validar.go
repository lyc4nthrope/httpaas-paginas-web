// Package publicar validates the user's zip on the host and deploys it to an instance
// through deploy/plantilla/httpaas-publicar (part C).
//
// The rules here are the same ones httpaas-publicar applies inside the VM (decision D11:
// two layers). They must stay equal: a zip accepted by the web and rejected by the VM, or
// the other way round, is a confusing error for the user.
package publicar

import (
	"archive/zip"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
)

// Limites bound what a zip may contain.
type Limites struct {
	MaxArchivos int
	MaxBytes    uint64 // total uncompressed size
}

// LimitesPorDefecto are the limits of httpaas-publicar: 2000 entries and 200 MB.
var LimitesPorDefecto = Limites{MaxArchivos: 2000, MaxBytes: 200 << 20}

// Resumen describes a valid zip.
type Resumen struct {
	Archivos int    // regular files
	Bytes    uint64 // total uncompressed size
	Raiz     string // "." or the single top-level folder that holds index.html
}

func rechazo(formato string, args ...any) error {
	return &dominio.ErrorValidacion{Campo: dominio.CampoArchivo, Mensaje: fmt.Sprintf(formato, args...)}
}

// ValidarZip checks a zip with the default limits. It is the function the web layer
// receives (web.Opciones.ValidarZip); every problem is a *dominio.ErrorValidacion.
func ValidarZip(ruta string) error {
	_, err := Validar(ruta, LimitesPorDefecto)
	return err
}

// Validar checks a zip and describes it. It only reads the zip's directory; nothing is
// extracted on the host.
func Validar(ruta string, lim Limites) (Resumen, error) {
	r, err := zip.OpenReader(ruta)
	if err != nil {
		return Resumen{}, rechazo("el archivo no es un .zip válido")
	}
	defer r.Close()

	if len(r.File) == 0 {
		return Resumen{}, rechazo("el zip está vacío")
	}
	if len(r.File) > lim.MaxArchivos {
		return Resumen{}, rechazo("el zip tiene más de %d archivos", lim.MaxArchivos)
	}

	var res Resumen
	vistos := map[string]bool{}   // every cleaned name, to refuse duplicates
	archivos := map[string]bool{} // regular files, by cleaned path
	carpetas := map[string]bool{} // folders: explicit ones and every parent of an entry
	visibles := map[string]bool{} // top-level names the VM script's "*" sees (not hidden)
	for _, f := range r.File {
		nombre := f.Name
		// Same test as httpaas-publicar: a leading "/" or any "..". Backslashes are
		// refused too, because some extractors treat them as separators (unzip fails on
		// them in the VM as well).
		if strings.HasPrefix(nombre, "/") || strings.Contains(nombre, "..") || strings.Contains(nombre, `\`) {
			return Resumen{}, rechazo("el zip contiene rutas peligrosas: %q", nombre)
		}
		// unzip cleans "./x", "a//b" and "a/./b"; so do we, so both sides see one name.
		esCarpeta := strings.HasSuffix(nombre, "/")
		limpio := path.Clean(nombre)
		if limpio == "." {
			continue // "./": the root itself
		}
		if vistos[limpio] {
			return Resumen{}, rechazo("el zip tiene un nombre repetido: %q", limpio)
		}
		vistos[limpio] = true

		// The type goes first: Go reports a name ending in "/" as a folder, so a symlink
		// called "link/" would otherwise pass as one.
		modo := f.Mode()
		switch {
		case modo&fs.ModeSymlink != 0:
			return Resumen{}, rechazo("el zip contiene enlaces simbólicos: %q", nombre)
		case modo.Type()&^fs.ModeDir != 0:
			// Stricter than the VM, which would extract a FIFO or device as a plain file.
			return Resumen{}, rechazo("el zip contiene un archivo de tipo no permitido: %q", nombre)
		case esCarpeta:
			// Only a name ending in "/" is a folder, as for unzip: a folder entry without
			// it is extracted as a file, and the file/folder check below catches a clash.
			carpetas[limpio] = true
		default:
			// res.Bytes never exceeds MaxBytes, so the subtraction cannot wrap around.
			if f.UncompressedSize64 > lim.MaxBytes-res.Bytes {
				return Resumen{}, rechazo("el zip descomprimido supera los %d MB", lim.MaxBytes>>20)
			}
			res.Bytes += f.UncompressedSize64
			res.Archivos++
			archivos[limpio] = true
		}
		// Stops at "." and at "/" too, so it always ends even for an unexpected name.
		for padre := path.Dir(limpio); padre != "." && padre != "/"; padre = path.Dir(padre) {
			carpetas[padre] = true
		}
		if primero, _, _ := strings.Cut(limpio, "/"); !strings.HasPrefix(primero, ".") {
			visibles[primero] = true
		}
	}
	// A name that is both a file and a folder cannot be extracted (unzip fails on it).
	for nombre := range archivos {
		if carpetas[nombre] {
			return Resumen{}, rechazo("el zip usa %q como archivo y carpeta a la vez", nombre)
		}
	}

	if archivos["index.html"] {
		res.Raiz = "."
		return res, nil
	}
	// Like the VM script: exactly one visible top-level item, it is a folder, and it holds
	// index.html directly.
	if len(visibles) == 1 {
		for unica := range visibles {
			if carpetas[unica] && archivos[path.Join(unica, "index.html")] {
				res.Raiz = unica
				return res, nil
			}
		}
	}
	return Resumen{}, rechazo("no se encontró index.html en la raíz del sitio ni dentro de una única carpeta")
}
