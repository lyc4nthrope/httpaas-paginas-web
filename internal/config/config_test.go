package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The example shipped in the repo must always load: it is what teammates copy.
func TestExampleLoads(t *testing.T) {
	c, err := Cargar(filepath.Join("..", "..", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.IP(30) != "192.168.10.30" || !c.EnRango("192.168.10.99") || c.EnRango("192.168.10.10") {
		t.Fatalf("unexpected addressing: %+v", c.Red)
	}
}

func valida(t *testing.T) Config {
	t.Helper()
	c, err := Cargar(filepath.Join("..", "..", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestValidarRejects(t *testing.T) {
	tests := []struct {
		name  string
		mod   func(*Config)
		campo string
	}{
		{"listen without port", func(c *Config) { c.Listen = "127.0.0.1" }, "listen"},
		{"uppercase domain", func(c *Config) { c.Dominio = "Cloud.Local" }, "dominio"},
		{"base without trailing dot", func(c *Config) { c.Red.Base = "192.168.10" }, "red.base"},
		{"base not ipv4", func(c *Config) { c.Red.Base = "300.1.1." }, "red.base"},
		{"prefix too small", func(c *Config) { c.Red.Prefijo = 4 }, "red.prefijo"},
		{"range reversed", func(c *Config) { c.Red.Primera, c.Red.Ultima = 50, 40 }, "red.primera/ultima"},
		{"range hits broadcast", func(c *Config) { c.Red.Ultima = 255 }, "red.primera/ultima"},
		{"zero instances", func(c *Config) { c.MaxInstancias = 0 }, "max_instancias"},
		{"more instances than ips", func(c *Config) { c.Red.Primera, c.Red.Ultima, c.MaxInstancias = 30, 31, 3 }, "max_instancias"},
		{"zip limit too big", func(c *Config) { c.MaxZipMB = 1000 }, "max_zip_mb"},
		{"dns inside instance range", func(c *Config) { c.DNS.IP = "192.168.10.40" }, "dns.ip"},
		{"dns not an ip", func(c *Config) { c.DNS.IP = "ns" }, "dns.ip"},
		{"ssh user with shell syntax", func(c *Config) { c.SSH.Usuario = "root;id" }, "ssh.usuario"},
		{"no key path", func(c *Config) { c.SSH.Llave = "" }, "ssh.llave"},
		{"efi typo", func(c *Config) { c.VBox.Firmware = "uefi" }, "vbox.firmware"},
		{"nvme controller", func(c *Config) { c.VBox.Controlador = "nvme" }, "vbox.controlador"},
		{"too little memory", func(c *Config) { c.VBox.MemoriaMB = 128 }, "vbox.memoria_mb"},
		{"empty base disk", func(c *Config) { c.VBox.DiscoBase = "" }, "vbox.disco_base"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valida(t)
			tt.mod(&c)
			err := c.Validar()
			if err == nil || !strings.Contains(err.Error(), tt.campo+":") {
				t.Fatalf("Validar() = %v, want an error on %q", err, tt.campo)
			}
		})
	}
}

func TestValidarReportsEveryProblem(t *testing.T) {
	c := valida(t)
	c.Listen, c.Dominio, c.VBox.CPUs = "", "", 0
	err := c.Validar()
	for _, campo := range []string{"listen:", "dominio:", "vbox.cpus:"} {
		if err == nil || !strings.Contains(err.Error(), campo) {
			t.Errorf("missing %q in %v", campo, err)
		}
	}
}

func TestCargarRejectsUnknownFields(t *testing.T) {
	data, _ := os.ReadFile(filepath.Join("..", "..", "config.example.json"))
	typo := strings.Replace(string(data), `"dominio"`, `"dominoi"`, 1)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(typo), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Cargar(path); err == nil || !strings.Contains(err.Error(), "dominoi") {
		t.Fatalf("Cargar = %v, want unknown field error", err)
	}
}

func TestVerificarArchivos(t *testing.T) {
	c := valida(t)
	dir := t.TempDir()
	c.SSH.Llave = filepath.Join(dir, "llave")
	c.VBox.DiscoBase = filepath.Join(dir, "ApacheServer.vdi")
	if err := c.VerificarArchivos(); err == nil || !strings.Contains(err.Error(), "ssh.llave") || !strings.Contains(err.Error(), "vbox.disco_base") {
		t.Fatalf("missing files: %v", err)
	}
	for _, p := range []string{c.SSH.Llave, c.VBox.DiscoBase} {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.VerificarArchivos(); err != nil {
		t.Fatalf("existing files: %v", err)
	}
}
