package publicar

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type FakeExecutor struct {
	LastSrcLocal  string
	LastDstRemote string
	ExitCode      int
	Stdout        string
	Stderr        string
	RunErr        error
	CopyErr       error
}

func (f *FakeExecutor) CopyTo(srcLocal, dstRemote string) error {
	f.LastSrcLocal = srcLocal
	f.LastDstRemote = dstRemote
	return f.CopyErr
}

func (f *FakeExecutor) Run(cmd string) (int, string, string, error) {
	return f.ExitCode, f.Stdout, f.Stderr, f.RunErr
}

func crearZipEnMemoria(t *testing.T, archivos map[string][]byte) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "testzip_*.zip")
	if err != nil {
		t.Fatalf("error creando archivo temporal: %v", err)
	}
	defer tmpFile.Close()

	zw := zip.NewWriter(tmpFile)
	for name, body := range archivos {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("error creando entrada zip %s: %v", name, err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatalf("error escribiendo entrada zip %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("error cerrando zip writer: %v", err)
	}
	return tmpFile.Name()
}

func TestValidarZip_CasosEnMemoria(t *testing.T) {
	zipValido := crearZipEnMemoria(t, map[string][]byte{
		"index.html": []byte("<h1>Sitio Valido</h1>"),
	})
	defer os.Remove(zipValido)

	zipSinIndex := crearZipEnMemoria(t, map[string][]byte{
		"estilo.css": []byte("body { color: red; }"),
	})
	defer os.Remove(zipSinIndex)

	zipSlip := crearZipEnMemoria(t, map[string][]byte{
		"../../etc/passwd": []byte("root:x:0:0"),
	})
	defer os.Remove(zipSlip)

	zipAbsoluto := crearZipEnMemoria(t, map[string][]byte{
		"/etc/shadow": []byte("secret"),
	})
	defer os.Remove(zipAbsoluto)

	tmpNotZip, _ := os.CreateTemp("", "notzip_*.zip")
	tmpNotZip.WriteString("este texto no es un zip")
	tmpNotZip.Close()
	defer os.Remove(tmpNotZip.Name())

	tests := []struct {
		nombre      string
		ruta        string
		esperaError bool
	}{
		{"Zip Valido", zipValido, false},
		{"Sin Index", zipSinIndex, true},
		{"Zip Slip ..", zipSlip, true},
		{"Ruta Absoluta", zipAbsoluto, true},
		{"No es un Zip", tmpNotZip.Name(), true},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			_, err := ValidarZip(tt.ruta, LimitesPorDefecto)
			if tt.esperaError && err == nil {
				t.Errorf("Se esperaba error para %s pero se obtuvo nil", tt.nombre)
			}
			if !tt.esperaError && err != nil {
				t.Errorf("No se esperaba error para %s pero se obtuvo: %v", tt.nombre, err)
			}
		})
	}
}

func TestPublicar_FakeExecutor(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "paisajes.cloud.local" {
			t.Errorf("Se esperaba Host paisajes.cloud.local pero se obtuvo %s", r.Host)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer ts.Close()

	hostPort := strings.TrimPrefix(ts.URL, "http://")

	fakeExec := &FakeExecutor{
		ExitCode: 0,
		Stdout:   "OK",
	}

	pub := NewPublicador(fakeExec)
	err := pub.Publicar("paisajes.cloud.local", hostPort, "dummy.zip")
	if err != nil {
		t.Fatalf("Publicar fallo: %v", err)
	}

	if !strings.HasPrefix(fakeExec.LastDstRemote, "/home/httpaas/subidas/") || !strings.HasSuffix(fakeExec.LastDstRemote, ".zip") {
		t.Errorf("Ruta destino invalida: %s", fakeExec.LastDstRemote)
	}
}
