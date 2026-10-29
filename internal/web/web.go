// Package web serves the management dashboard and its form actions. Handlers only
// parse and validate input, then delegate to the instances service; they never run
// commands themselves.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/dominio"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/instancias"
)

//go:embed templates/*.html
var archivos embed.FS

// Servicio is what the web layer needs from internal/instancias.
type Servicio interface {
	SolicitarCrear(nombre string) (dominio.Instancia, error)
	SolicitarPublicar(nombre, zip string) error
	SolicitarEliminar(nombre string) error
	Listar() []dominio.Instancia
}

type Opciones struct {
	Servicio Servicio
	Dominio  string // e.g. cloud.local
	Prefijo  int    // e.g. 24, shown as IP/24
	MaxZipMB int
	// ValidarZip checks an uploaded file before it is queued. Part C provides the real
	// one (internal/publicar); it must return *dominio.ErrorValidacion for bad input.
	ValidarZip func(ruta string) error
	Logf       func(format string, args ...any)
}

type servidor struct {
	o    Opciones
	tmpl *template.Template
}

// Nuevo returns the HTTP handler with every route.
func Nuevo(o Opciones) http.Handler {
	if o.Logf == nil {
		o.Logf = log.Printf
	}
	s := &servidor{o: o, tmpl: template.Must(template.ParseFS(archivos, "templates/*.html"))}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.dashboard)
	mux.HandleFunc("GET /salud", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") })
	mux.HandleFunc("GET /api/instancias", s.api)
	mux.HandleFunc("POST /instancias", s.crear)
	mux.HandleFunc("POST /contenido", s.publicar)
	mux.HandleFunc("POST /instancias/{nombre}/eliminar", s.eliminar)
	return seguridad(mux, int64(o.MaxZipMB)<<20)
}

// Fila is one instance as the dashboard shows it.
type Fila struct {
	Nombre, IP, URL, Creada, Error string
	Estado                         dominio.Estado
	Prefijo                        int
	Ocupada                        bool // a job is running: the page should refresh
	Eliminable                     bool
}

// Vista is the data given to templates/dashboard.html. Part B builds the final
// template on top of these fields (docs/parte-b/PLAN.md, B7).
type Vista struct {
	Instancias   []Fila
	Aviso        string            // success message after a redirect
	Errores      map[string]string // field name → message (HTTP 422)
	Nombre       string            // value to refill the name field
	MaxZipMB     int
	CampoNombre  string
	CampoArchivo string
	Refrescar    bool // some instance is busy
}

func (s *servidor) vista() Vista {
	v := Vista{MaxZipMB: s.o.MaxZipMB, CampoNombre: dominio.CampoNombre, CampoArchivo: dominio.CampoArchivo}
	for _, i := range s.o.Servicio.Listar() {
		f := s.fila(i)
		v.Refrescar = v.Refrescar || f.Ocupada
		v.Instancias = append(v.Instancias, f)
	}
	return v
}

func (s *servidor) fila(i dominio.Instancia) Fila {
	return Fila{
		Nombre: i.Nombre, IP: i.IP, Prefijo: s.o.Prefijo, Estado: i.Estado, Error: i.Error,
		URL:        "http://" + dominio.FQDN(i.Nombre, s.o.Dominio) + "/",
		Creada:     i.Creada.Local().Format("2006-01-02 15:04:05"),
		Ocupada:    i.Estado == dominio.Creando || i.Estado == dominio.Publicando || i.Estado == dominio.Eliminando,
		Eliminable: i.Estado.PuedeEliminar(),
	}
}

// avisos are the only messages a redirect can ask for; the URL never carries free text.
var avisos = map[string]string{
	"creada":     "Instancia %s en creación. Tarda alrededor de un minuto.",
	"publicando": "Publicando el contenido en %s.",
	"eliminando": "Eliminando la instancia %s.",
}

func (s *servidor) dashboard(w http.ResponseWriter, r *http.Request) {
	v := s.vista()
	q := r.URL.Query()
	if formato, ok := avisos[q.Get("aviso")]; ok && dominio.ValidarHostname(q.Get("nombre")) == nil {
		v.Aviso = fmt.Sprintf(formato, q.Get("nombre"))
	}
	s.render(w, http.StatusOK, v)
}

func (s *servidor) api(w http.ResponseWriter, _ *http.Request) {
	filas := []Fila{}
	for _, i := range s.o.Servicio.Listar() {
		filas = append(filas, s.fila(i))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(filas)
}

func (s *servidor) crear(w http.ResponseWriter, r *http.Request) {
	nombre := r.FormValue(dominio.CampoNombre)
	if _, err := s.o.Servicio.SolicitarCrear(nombre); err != nil {
		s.fallo(w, r, nombre, err)
		return
	}
	redirigir(w, r, "creada", nombre)
}

func (s *servidor) publicar(w http.ResponseWriter, r *http.Request) {
	// Parse explicitly: FormValue would swallow a "body too large" error and the user
	// would get "choose a file" instead of "file too large".
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if !errors.As(err, &tooBig) {
			err = &dominio.ErrorValidacion{Campo: dominio.CampoArchivo, Mensaje: "no se pudo leer el formulario; vuelve a elegir el archivo"}
		}
		s.fallo(w, r, "", err)
		return
	}
	defer r.MultipartForm.RemoveAll()
	nombre := r.FormValue(dominio.CampoNombre)
	archivo, _, err := r.FormFile(dominio.CampoArchivo)
	if err != nil {
		s.fallo(w, r, nombre, &dominio.ErrorValidacion{Campo: dominio.CampoArchivo, Mensaje: "elige un archivo .zip"})
		return
	}
	defer archivo.Close()

	ruta, err := guardarTemporal(archivo)
	if err != nil {
		s.fallo(w, r, nombre, err)
		return
	}
	if err := s.o.ValidarZip(ruta); err != nil {
		os.Remove(ruta)
		s.fallo(w, r, nombre, err)
		return
	}
	// From here on the service owns the file and deletes it.
	if err := s.o.Servicio.SolicitarPublicar(nombre, ruta); err != nil {
		s.fallo(w, r, nombre, err)
		return
	}
	redirigir(w, r, "publicando", nombre)
}

func (s *servidor) eliminar(w http.ResponseWriter, r *http.Request) {
	nombre := r.PathValue("nombre")
	if err := s.o.Servicio.SolicitarEliminar(nombre); err != nil {
		s.fallo(w, r, "", err)
		return
	}
	redirigir(w, r, "eliminando", nombre)
}

// fallo answers an error: 422 for bad input, 413 for a too large upload,
// 503 when the queue is full and 500 otherwise. The detail goes to the log.
func (s *servidor) fallo(w http.ResponseWriter, r *http.Request, nombre string, err error) {
	v := s.vista()
	v.Nombre = nombre
	v.Errores = map[string]string{}
	var ev *dominio.ErrorValidacion
	var tooBig *http.MaxBytesError
	code := http.StatusInternalServerError
	switch {
	case errors.As(err, &ev):
		code, v.Errores[ev.Campo] = http.StatusUnprocessableEntity, ev.Mensaje
	case errors.As(err, &tooBig):
		code = http.StatusRequestEntityTooLarge
		v.Errores[dominio.CampoArchivo] = "el archivo supera el máximo de " + strconv.Itoa(s.o.MaxZipMB) + " MB"
	case errors.Is(err, instancias.ErrOcupado):
		code, v.Errores[dominio.CampoNombre] = http.StatusServiceUnavailable, err.Error()
	default:
		s.o.Logf("%s %s: %v", r.Method, r.URL.Path, err)
		v.Errores[dominio.CampoNombre] = "ocurrió un error interno; revisa el registro de la aplicación"
	}
	s.render(w, code, v)
}

func (s *servidor) render(w http.ResponseWriter, code int, v Vista) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	if err := s.tmpl.ExecuteTemplate(w, "dashboard.html", v); err != nil {
		s.o.Logf("render dashboard: %v", err)
	}
}

func redirigir(w http.ResponseWriter, r *http.Request, aviso, nombre string) {
	q := url.Values{"aviso": {aviso}, "nombre": {nombre}}
	http.Redirect(w, r, "/?"+q.Encode(), http.StatusSeeOther)
}

func guardarTemporal(src io.Reader) (string, error) {
	f, err := os.CreateTemp("", "httpaas-subida-*.zip")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
