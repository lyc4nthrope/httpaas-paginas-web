// Package config loads and validates config.json (docs/PLAN.md §4.5).
// Every problem is reported at startup, all at once, before anything touches a VM.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
)

type Config struct {
	Listen        string `json:"listen"`
	Dominio       string `json:"dominio"`
	Red           Red    `json:"red"`
	MaxInstancias int    `json:"max_instancias"`
	MaxZipMB      int    `json:"max_zip_mb"`
	DNS           DNS    `json:"dns"`
	SSH           SSH    `json:"ssh"`
	VBox          VBox   `json:"vbox"`
	Datos         string `json:"datos"`
	Logs          string `json:"logs"`
}

// Red is the host-only network and the range of addresses given to instances.
type Red struct {
	Base    string `json:"base"` // e.g. "192.168.10."
	Prefijo int    `json:"prefijo"`
	Primera int    `json:"primera"`
	Ultima  int    `json:"ultima"`
}

type DNS struct {
	IP      string `json:"ip"`
	Usuario string `json:"usuario"`
}

type SSH struct {
	Usuario    string `json:"usuario"`
	Llave      string `json:"llave"`
	KnownHosts string `json:"known_hosts"`
}

type VBox struct {
	VBoxManage  string `json:"vboxmanage"`
	DiscoBase   string `json:"disco_base"`
	RedHostOnly string `json:"red_hostonly"`
	OSType      string `json:"ostype"`
	Firmware    string `json:"firmware"`
	Controlador string `json:"controlador"`
	MemoriaMB   int    `json:"memoria_mb"`
	CPUs        int    `json:"cpus"`
}

var (
	dominioRE = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	usuarioRE = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
)

// Cargar reads and validates the file. Unknown fields are rejected so typos are not ignored.
func Cargar(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := c.Validar(); err != nil {
		return Config{}, fmt.Errorf("invalid %s:\n%w", path, err)
	}
	return c, nil
}

// Validar checks every field and returns all problems joined, one per line.
func (c Config) Validar() error {
	var errs []error
	add := func(campo, msg string) { errs = append(errs, fmt.Errorf("%s: %s", campo, msg)) }

	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		add("listen", "debe ser host:puerto, por ejemplo 127.0.0.1:8080")
	}
	if !dominioRE.MatchString(c.Dominio) {
		add("dominio", "debe ser un dominio en minúsculas, por ejemplo cloud.local")
	}

	baseOK := net.ParseIP(c.Red.Base+"1").To4() != nil && c.Red.Base[len(c.Red.Base)-1] == '.'
	if !baseOK {
		add("red.base", `debe ser los tres primeros octetos con punto final, por ejemplo "192.168.10."`)
	}
	if c.Red.Prefijo < 8 || c.Red.Prefijo > 30 {
		add("red.prefijo", "debe estar entre 8 y 30")
	}
	rangoOK := c.Red.Primera >= 2 && c.Red.Ultima <= 254 && c.Red.Primera <= c.Red.Ultima
	if !rangoOK {
		add("red.primera/ultima", "deben cumplir 2 ≤ primera ≤ ultima ≤ 254")
	}
	if c.MaxInstancias < 1 || (rangoOK && c.MaxInstancias > c.Red.Ultima-c.Red.Primera+1) {
		add("max_instancias", "debe ser al menos 1 y no mayor que las IPs del rango")
	}
	if c.MaxZipMB < 1 || c.MaxZipMB > 500 {
		add("max_zip_mb", "debe estar entre 1 y 500")
	}

	if ip := net.ParseIP(c.DNS.IP).To4(); ip == nil {
		add("dns.ip", "debe ser una IPv4")
	} else if baseOK && rangoOK && c.EnRango(c.DNS.IP) {
		add("dns.ip", "no puede estar dentro del rango de las instancias")
	}
	if !usuarioRE.MatchString(c.DNS.Usuario) {
		add("dns.usuario", "debe ser un usuario Linux válido")
	}
	if !usuarioRE.MatchString(c.SSH.Usuario) {
		add("ssh.usuario", "debe ser un usuario Linux válido")
	}
	if c.SSH.Llave == "" {
		add("ssh.llave", "falta la ruta de la llave privada")
	}
	if c.SSH.KnownHosts == "" {
		add("ssh.known_hosts", "falta la ruta del archivo known_hosts de la app")
	}

	v := c.VBox
	for campo, val := range map[string]string{
		"vbox.vboxmanage": v.VBoxManage, "vbox.disco_base": v.DiscoBase,
		"vbox.red_hostonly": v.RedHostOnly, "vbox.ostype": v.OSType,
	} {
		if val == "" {
			add(campo, "no puede estar vacío")
		}
	}
	if v.Firmware != "bios" && v.Firmware != "efi" {
		add("vbox.firmware", `debe ser "bios" o "efi" (el mismo de la plantilla)`)
	}
	if v.Controlador != "sata" {
		add("vbox.controlador", `debe ser "sata" (el mismo de la plantilla)`)
	}
	if v.MemoriaMB < 256 {
		add("vbox.memoria_mb", "debe ser al menos 256")
	}
	if v.CPUs < 1 {
		add("vbox.cpus", "debe ser al menos 1")
	}
	if c.Datos == "" {
		add("datos", "falta la ruta de instancias.json")
	}
	if c.Logs == "" {
		add("logs", "falta la carpeta del registro")
	}
	return errors.Join(errs...)
}

// VerificarArchivos checks the files the real adapters need. It is separate from Validar
// because simulated mode (-simular) runs without a key or a base disk.
func (c Config) VerificarArchivos() error {
	var errs []error
	for campo, ruta := range map[string]string{"ssh.llave": c.SSH.Llave, "vbox.disco_base": c.VBox.DiscoBase} {
		if _, err := os.Stat(ruta); err != nil {
			errs = append(errs, fmt.Errorf("%s: no se encuentra %s", campo, ruta))
		}
	}
	return errors.Join(errs...)
}

// IP returns the address of host number n in the network, e.g. IP(30) = "192.168.10.30".
func (c Config) IP(n int) string { return c.Red.Base + strconv.Itoa(n) }

// EnRango reports whether ip belongs to the range given to instances.
func (c Config) EnRango(ip string) bool {
	for n := c.Red.Primera; n <= c.Red.Ultima; n++ {
		if c.IP(n) == ip {
			return true
		}
	}
	return false
}
