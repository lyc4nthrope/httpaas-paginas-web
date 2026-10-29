// Package vbox implements puertos.Hipervisor with VBoxManage. Every command is built by
// a pure function from validated values and run with exec (never through a shell).
// The command sequence follows docs/parte-c/PLAN.md (C6, clon-manual.md).
package vbox

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/config"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/puertos"
)

const (
	propHostname = "/httpaas/hostname"
	propIP       = "/httpaas/ip"
	propDominio  = "/httpaas/dominio"
	propDNS      = "/httpaas/dns"
	propEstado   = "/httpaas/estado"
	controlador  = "SATA"
)

// runner runs VBoxManage with args and returns its stdout; stderr is part of the error.
type runner func(ctx context.Context, args ...string) (string, error)

// VBox is safe for concurrent use, though the orchestrator calls it one job at a time.
type VBox struct {
	cfg      config.VBox
	dominio  string
	dnsIP    string
	run      runner
	sondeo   time.Duration                                     // pause between state checks
	puertoOK func(ctx context.Context, direccion string) error // TCP check of port 22
}

var _ puertos.Hipervisor = (*VBox)(nil)

func Nuevo(c config.Config) *VBox {
	v := &VBox{cfg: c.VBox, dominio: c.Dominio, dnsIP: c.DNS.IP, sondeo: 2 * time.Second}
	v.run = func(ctx context.Context, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, c.VBox.VBoxManage, args...)
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err != nil {
			return out.String(), fmt.Errorf("VBoxManage %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
		}
		return out.String(), nil
	}
	v.puertoOK = func(ctx context.Context, direccion string) error {
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", direccion)
		if err != nil {
			return err
		}
		return conn.Close()
	}
	return v
}

// argsCrear is the exact command sequence that creates and starts an instance.
func argsCrear(spec puertos.SpecVM, vb config.VBox, dominio, dnsIP string) [][]string {
	n := spec.Nombre
	return [][]string{
		{"createvm", "--name", n, "--ostype", vb.OSType, "--register"},
		{"modifyvm", n, "--memory", strconv.Itoa(vb.MemoriaMB), "--cpus", strconv.Itoa(vb.CPUs),
			"--firmware", vb.Firmware, "--nic1", "hostonly", "--host-only-adapter1", vb.RedHostOnly,
			"--nic2", "nat", "--audio-enabled", "off"},
		{"storagectl", n, "--name", controlador, "--add", "sata", "--controller", "IntelAhci"},
		{"storageattach", n, "--storagectl", controlador, "--port", "0", "--device", "0",
			"--type", "hdd", "--medium", vb.DiscoBase},
		{"guestproperty", "set", n, propHostname, n},
		{"guestproperty", "set", n, propIP, spec.IP + "/" + strconv.Itoa(spec.Prefijo)},
		{"guestproperty", "set", n, propDominio, dominio},
		{"guestproperty", "set", n, propDNS, dnsIP},
		{"startvm", n, "--type", "headless"},
	}
}

// Crear runs argsCrear. If a step fails after the VM is registered, the VM is deleted
// again, so no half-created VM keeps the name or a differencing disk.
func (v *VBox) Crear(ctx context.Context, spec puertos.SpecVM) error {
	for i, args := range argsCrear(spec, v.cfg, v.dominio, v.dnsIP) {
		if _, err := v.run(ctx, args...); err != nil {
			if i > 0 {
				if _, rerr := v.run(context.WithoutCancel(ctx), "unregistervm", spec.Nombre, "--delete"); rerr != nil {
					err = fmt.Errorf("%w (rollback also failed: %v)", err, rerr)
				}
			}
			return fmt.Errorf("create vm %s: %w", spec.Nombre, err)
		}
	}
	return nil
}

// EsperarLista waits until the VM reports "listo" and its SSH port answers.
func (v *VBox) EsperarLista(ctx context.Context, nombre string) error {
	for {
		estado, err := v.propiedad(ctx, nombre, propEstado)
		if err != nil {
			return err
		}
		switch {
		case estado == "listo":
			ip, err := v.propiedad(ctx, nombre, propIP)
			if err != nil {
				return err
			}
			ip, _, _ = strings.Cut(ip, "/")
			if v.puertoOK(ctx, net.JoinHostPort(ip, "22")) == nil {
				return nil
			}
		case strings.HasPrefix(estado, "error:"):
			return fmt.Errorf("vm %s reported %s", nombre, estado)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("vm %s not ready (last state %q): %w", nombre, estado, ctx.Err())
		case <-time.After(v.sondeo):
		}
	}
}

// Eliminar powers the VM off if needed and deletes it with its differencing disk.
// The multiattach base disk is shared, so VirtualBox keeps it (checked in A6, step 6).
func (v *VBox) Eliminar(ctx context.Context, nombre string) error {
	estado, existe, err := v.estadoVM(ctx, nombre)
	if err != nil || !existe {
		return err
	}
	switch estado {
	case "poweroff", "aborted":
	case "saved":
		if _, err := v.run(ctx, "discardstate", nombre); err != nil {
			return err
		}
	default:
		if _, err := v.run(ctx, "controlvm", nombre, "poweroff"); err != nil {
			return err
		}
		if err := v.esperarApagada(ctx, nombre); err != nil {
			return err
		}
	}
	_, err = v.run(ctx, "unregistervm", nombre, "--delete")
	return err
}

// Listar returns the names of every registered VM.
func (v *VBox) Listar(ctx context.Context) ([]string, error) {
	out, err := v.run(ctx, "list", "vms")
	if err != nil {
		return nil, err
	}
	return parsearLista(out), nil
}

func (v *VBox) esperarApagada(ctx context.Context, nombre string) error {
	for {
		estado, _, err := v.estadoVM(ctx, nombre)
		if err != nil {
			return err
		}
		if estado == "poweroff" || estado == "aborted" {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("vm %s did not power off (state %q): %w", nombre, estado, ctx.Err())
		case <-time.After(v.sondeo):
		}
	}
}

// estadoVM returns VMState, or existe=false when VirtualBox has no such VM.
func (v *VBox) estadoVM(ctx context.Context, nombre string) (estado string, existe bool, err error) {
	out, err := v.run(ctx, "showvminfo", nombre, "--machinereadable")
	if err != nil {
		if strings.Contains(err.Error(), "VBOX_E_OBJECT_NOT_FOUND") {
			return "", false, nil
		}
		return "", false, err
	}
	return parsearEstado(out), true, nil
}

func (v *VBox) propiedad(ctx context.Context, nombre, clave string) (string, error) {
	out, err := v.run(ctx, "guestproperty", "get", nombre, clave)
	if err != nil {
		return "", err
	}
	return parsearValor(out), nil
}

// parsearValor reads "Value: x" from guestproperty get; "No value set!" becomes "".
// VBoxManage exits 0 in both cases, so the text is what tells them apart.
func parsearValor(out string) string {
	v, ok := strings.CutPrefix(strings.TrimSpace(out), "Value: ")
	if !ok {
		return ""
	}
	return v
}

// parsearEstado reads VMState="running" from showvminfo --machinereadable.
func parsearEstado(out string) string {
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "VMState="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

// parsearLista reads `"name" {uuid}` lines from list vms. Names may contain spaces.
func parsearLista(out string) []string {
	var nombres []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		linea := sc.Text()
		if !strings.HasPrefix(linea, `"`) {
			continue
		}
		if fin := strings.LastIndex(linea, `" {`); fin > 0 {
			nombres = append(nombres, linea[1:fin])
		}
	}
	return nombres
}
