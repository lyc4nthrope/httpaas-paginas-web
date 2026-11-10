package publicar

import (
	"archive/zip"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

type Limites struct {
	MaxArchivos int
	MaxBytes    int64
}

type Resumen struct {
	Archivos int
	Bytes    int64
	Raiz     string
}

var LimitesPorDefecto = Limites{
	MaxArchivos: 2000,
	MaxBytes:    200 * 1024 * 1024,
}

func ValidarZip(ruta string, lim Limites) (Resumen, error) {
	if lim.MaxArchivos <= 0 {
		lim.MaxArchivos = LimitesPorDefecto.MaxArchivos
	}
	if lim.MaxBytes <= 0 {
		lim.MaxBytes = LimitesPorDefecto.MaxBytes
	}

	r, err := zip.OpenReader(ruta)
	if err != nil {
		return Resumen{}, fmt.Errorf("no es un archivo zip valido: %w", err)
	}
	defer r.Close()

	if len(r.File) == 0 {
		return Resumen{}, errors.New("el zip esta vacio")
	}

	var resumen Resumen
	tieneIndex := false

	for _, f := range r.File {
		cleanPath := filepath.Clean(f.Name)
		if !filepath.IsLocal(cleanPath) || strings.HasPrefix(cleanPath, "..") || strings.HasPrefix(f.Name, "/") {
			return Resumen{}, fmt.Errorf("el zip contiene rutas peligrosas: %s", f.Name)
		}

		if f.Mode()&0400000 != 0 || f.Mode()&0120000 == 0120000 {
			return Resumen{}, fmt.Errorf("el zip contiene enlaces simbolicos: %s", f.Name)
		}

		resumen.Archivos++
		if resumen.Archivos > lim.MaxArchivos {
			return Resumen{}, fmt.Errorf("el zip excede el limite maximo de %d archivos", lim.MaxArchivos)
		}

		resumen.Bytes += int64(f.UncompressedSize64)
		if resumen.Bytes > lim.MaxBytes {
			return Resumen{}, fmt.Errorf("el zip excede el limite maximo de %d bytes", lim.MaxBytes)
		}

		if cleanPath == "index.html" {
			tieneIndex = true
		}
	}

	if !tieneIndex {
		return Resumen{}, errors.New("no se encontro index.html en la raiz del sitio")
	}

	resumen.Raiz = "."
	return resumen, nil
}
