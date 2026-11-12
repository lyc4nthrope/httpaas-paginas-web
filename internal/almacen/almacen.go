// Package almacen persists the instances in a JSON file and hands out their IPs.
// Every change is written atomically (temp file + rename), so a crash never leaves
// a half-written file (decision D12).
package almacen

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
)

// ErrNoExiste is returned when an operation names an instance that is not stored.
var ErrNoExiste = errors.New("la instancia no existe")

// Almacen is safe for concurrent use.
type Almacen struct {
	mu    sync.Mutex
	ruta  string
	rango []string // candidate IPs in allocation order
	inst  map[string]dominio.Instancia
}

// Abrir loads the file at ruta. A missing file is an empty store; a corrupt one is an
// error, and it is never overwritten, so no data is silently lost.
func Abrir(ruta string, rango []string) (*Almacen, error) {
	a := &Almacen{ruta: ruta, rango: rango, inst: map[string]dominio.Instancia{}}
	data, err := os.ReadFile(ruta)
	if errors.Is(err, os.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read store: %w", err)
	}
	var lista []dominio.Instancia
	if err := json.Unmarshal(data, &lista); err != nil {
		return nil, fmt.Errorf("store %s is corrupt (fix or move it; it was not modified): %w", ruta, err)
	}
	for _, i := range lista {
		a.inst[i.Nombre] = i
	}
	return a, nil
}

// Reservar validates and stores a new instance in state Creando with the lowest free IP,
// in one step under the lock: two concurrent requests can never get the same name or IP.
func (a *Almacen) Reservar(nombre string, max int, ahora time.Time) (dominio.Instancia, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.inst[nombre]; ok {
		return dominio.Instancia{}, &dominio.ErrorValidacion{Campo: dominio.CampoNombre, Mensaje: fmt.Sprintf("ya existe una instancia llamada %q", nombre)}
	}
	if len(a.inst) >= max {
		return dominio.Instancia{}, &dominio.ErrorValidacion{Campo: dominio.CampoNombre, Mensaje: fmt.Sprintf("se alcanzó el máximo de %d instancias; elimina una primero", max)}
	}
	ip, ok := a.ipLibre()
	if !ok {
		return dominio.Instancia{}, &dominio.ErrorValidacion{Campo: dominio.CampoNombre, Mensaje: "no quedan direcciones IP libres en la red"}
	}
	i := dominio.Instancia{Nombre: nombre, IP: ip, Estado: dominio.Creando, Creada: ahora}
	a.inst[nombre] = i
	if err := a.guardar(); err != nil {
		delete(a.inst, nombre)
		return dominio.Instancia{}, err
	}
	return i, nil
}

// Actualizar applies cambio to a stored instance and persists it. Nombre and IP cannot change.
func (a *Almacen) Actualizar(nombre string, cambio func(*dominio.Instancia)) (dominio.Instancia, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	antes, ok := a.inst[nombre]
	if !ok {
		return dominio.Instancia{}, ErrNoExiste
	}
	despues := antes
	cambio(&despues)
	despues.Nombre, despues.IP = antes.Nombre, antes.IP
	a.inst[nombre] = despues
	if err := a.guardar(); err != nil {
		a.inst[nombre] = antes
		return dominio.Instancia{}, err
	}
	return despues, nil
}

// Quitar deletes an instance; its IP becomes free again.
func (a *Almacen) Quitar(nombre string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	antes, ok := a.inst[nombre]
	if !ok {
		return ErrNoExiste
	}
	delete(a.inst, nombre)
	if err := a.guardar(); err != nil {
		a.inst[nombre] = antes
		return err
	}
	return nil
}

// Obtener returns one instance.
func (a *Almacen) Obtener(nombre string) (dominio.Instancia, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	i, ok := a.inst[nombre]
	return i, ok
}

// Listar returns every instance, oldest first (the dashboard order).
func (a *Almacen) Listar() []dominio.Instancia {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ordenadas()
}

func (a *Almacen) ordenadas() []dominio.Instancia {
	lista := make([]dominio.Instancia, 0, len(a.inst))
	for _, i := range a.inst {
		lista = append(lista, i)
	}
	slices.SortFunc(lista, func(x, y dominio.Instancia) int {
		if c := x.Creada.Compare(y.Creada); c != 0 {
			return c
		}
		return compareStrings(x.Nombre, y.Nombre)
	})
	return lista
}

func compareStrings(x, y string) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

// ipLibre must be called with a.mu held.
func (a *Almacen) ipLibre() (string, bool) {
	usadas := map[string]bool{}
	for _, i := range a.inst {
		usadas[i.IP] = true
	}
	for _, ip := range a.rango {
		if !usadas[ip] {
			return ip, true
		}
	}
	return "", false
}

// guardar writes the whole store atomically. Must be called with a.mu held.
func (a *Almacen) guardar() error {
	data, err := json.MarshalIndent(a.ordenadas(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode store: %w", err)
	}
	dir := filepath.Dir(a.ruta)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".instancias-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp store: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp store: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp store: %w", err)
	}
	if err := os.Rename(tmp.Name(), a.ruta); err != nil {
		return fmt.Errorf("replace store: %w", err)
	}
	return nil
}
