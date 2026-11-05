package vbox

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/config"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/puertos"
)

var vb = config.VBox{
	VBoxManage: "VBoxManage", DiscoBase: "/vms/httpaas-base/ApacheServer.vdi", RedHostOnly: "vboxnet0",
	OSType: "Debian_64", Firmware: "bios", Controlador: "sata", MemoriaMB: 512, CPUs: 1,
}

// guion is a fake VBoxManage: respuesta decides each answer, every call is recorded.
type guion struct {
	llamadas  []string
	respuesta func(args []string) (string, error)
}

func (g *guion) run(_ context.Context, args ...string) (string, error) {
	g.llamadas = append(g.llamadas, strings.Join(args, " "))
	if g.respuesta == nil {
		return "", nil
	}
	return g.respuesta(args)
}

func nuevo(g *guion) *VBox {
	return &VBox{cfg: vb, dominio: "cloud.local", dnsIP: "192.168.10.10", run: g.run, sondeo: time.Millisecond,
		puertoOK: func(context.Context, string) error { return nil }}
}

var noEncontrada = errors.New("VBoxManage: error: Could not find a registered machine named 'x' ... code VBOX_E_OBJECT_NOT_FOUND (0x80bb0001)")

func TestArgsCrear(t *testing.T) {
	got := argsCrear(puertos.SpecVM{Nombre: "paisajes", IP: "192.168.10.30", Prefijo: 24}, vb, "cloud.local", "192.168.10.10")
	want := []string{
		"createvm --name paisajes --ostype Debian_64 --register",
		"modifyvm paisajes --memory 512 --cpus 1 --firmware bios --nic1 hostonly --host-only-adapter1 vboxnet0 --nic2 nat --audio-enabled off",
		"storagectl paisajes --name SATA --add sata --controller IntelAhci",
		"storageattach paisajes --storagectl SATA --port 0 --device 0 --type hdd --medium /vms/httpaas-base/ApacheServer.vdi",
		"guestproperty set paisajes /httpaas/hostname paisajes",
		"guestproperty set paisajes /httpaas/ip 192.168.10.30/24",
		"guestproperty set paisajes /httpaas/dominio cloud.local",
		"guestproperty set paisajes /httpaas/dns 192.168.10.10",
		"startvm paisajes --type headless",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d commands, want %d", len(got), len(want))
	}
	for i := range want {
		if strings.Join(got[i], " ") != want[i] {
			t.Errorf("step %d:\n got  %s\n want %s", i, strings.Join(got[i], " "), want[i])
		}
	}
}

func TestArgsKeepPathsWithSpacesAsOneArgument(t *testing.T) {
	c := vb
	c.DiscoBase = "/home/u/VirtualBox VMs/httpaas-base/ApacheServer.vdi"
	attach := argsCrear(puertos.SpecVM{Nombre: "x"}, c, "cloud.local", "192.168.10.10")[3]
	if attach[len(attach)-1] != c.DiscoBase {
		t.Fatalf("the disk path must travel as one argument: %q", attach)
	}
}

func TestCrearRollsBackAfterAFailure(t *testing.T) {
	g := &guion{respuesta: func(a []string) (string, error) {
		if a[0] == "storageattach" {
			return "", errors.New("medium locked")
		}
		return "", nil
	}}
	err := nuevo(g).Crear(context.Background(), puertos.SpecVM{Nombre: "paisajes", IP: "192.168.10.30", Prefijo: 24})
	if err == nil || !strings.Contains(err.Error(), "medium locked") {
		t.Fatalf("err = %v", err)
	}
	if last := g.llamadas[len(g.llamadas)-1]; last != "unregistervm paisajes --delete" {
		t.Fatalf("last call = %q, want rollback", last)
	}
}

func TestCrearDoesNotRollBackWhenCreateVMFails(t *testing.T) {
	g := &guion{respuesta: func([]string) (string, error) { return "", errors.New("name already used") }}
	_ = nuevo(g).Crear(context.Background(), puertos.SpecVM{Nombre: "paisajes"})
	if len(g.llamadas) != 1 {
		t.Fatalf("a failed createvm registered nothing, so nothing must be deleted: %v", g.llamadas)
	}
}

func TestEsperarLista(t *testing.T) {
	estados := []string{"No value set!\n", "Value: configurando\n", "Value: listo\n"}
	g := &guion{respuesta: func(a []string) (string, error) {
		if a[3] == propIP {
			return "Value: 192.168.10.30/24\n", nil
		}
		e := estados[0]
		if len(estados) > 1 {
			estados = estados[1:]
		}
		return e, nil
	}}
	v := nuevo(g)
	var direccion string
	v.puertoOK = func(_ context.Context, d string) error { direccion = d; return nil }
	if err := v.EsperarLista(context.Background(), "paisajes"); err != nil {
		t.Fatal(err)
	}
	if direccion != "192.168.10.30:22" {
		t.Fatalf("ssh check went to %q", direccion)
	}
}

func TestEsperarListaWaitsForSSH(t *testing.T) {
	g := &guion{respuesta: func(a []string) (string, error) {
		if a[3] == propIP {
			return "Value: 192.168.10.30/24\n", nil
		}
		return "Value: listo\n", nil
	}}
	v := nuevo(g)
	intentos := 0
	v.puertoOK = func(context.Context, string) error {
		if intentos++; intentos < 3 {
			return errors.New("connection refused")
		}
		return nil
	}
	if err := v.EsperarLista(context.Background(), "paisajes"); err != nil || intentos != 3 {
		t.Fatalf("err=%v intentos=%d", err, intentos)
	}
}

func TestEsperarListaReportsGuestError(t *testing.T) {
	g := &guion{respuesta: func([]string) (string, error) { return "Value: error:datos-invalidos\n", nil }}
	err := nuevo(g).EsperarLista(context.Background(), "paisajes")
	if err == nil || !strings.Contains(err.Error(), "error:datos-invalidos") {
		t.Fatalf("err = %v", err)
	}
}

func TestEsperarListaTimesOut(t *testing.T) {
	g := &guion{respuesta: func([]string) (string, error) { return "No value set!\n", nil }}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := nuevo(g).EsperarLista(ctx, "paisajes"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestEliminar(t *testing.T) {
	tests := []struct {
		name   string
		estado []string // successive VMState answers
		want   []string
	}{
		{"powered off", []string{"poweroff"}, []string{"showvminfo paisajes --machinereadable", "unregistervm paisajes --delete"}},
		{"saved", []string{"saved"}, []string{"showvminfo paisajes --machinereadable", "discardstate paisajes", "unregistervm paisajes --delete"}},
		{"running", []string{"running", "stopping", "poweroff"}, []string{
			"showvminfo paisajes --machinereadable", "controlvm paisajes poweroff",
			"showvminfo paisajes --machinereadable", "showvminfo paisajes --machinereadable",
			"unregistervm paisajes --delete"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			estados := tt.estado
			g := &guion{respuesta: func(a []string) (string, error) {
				if a[0] != "showvminfo" {
					return "", nil
				}
				e := estados[0]
				if len(estados) > 1 {
					estados = estados[1:]
				}
				return "name=\"paisajes\"\nVMState=\"" + e + "\"\nVMStateChangeTime=\"x\"\n", nil
			}}
			if err := nuevo(g).Eliminar(context.Background(), "paisajes"); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(g.llamadas, tt.want) {
				t.Fatalf("calls:\n got  %q\n want %q", g.llamadas, tt.want)
			}
		})
	}
}

func TestEliminarMissingVMIsNotAnError(t *testing.T) {
	g := &guion{respuesta: func([]string) (string, error) { return "", noEncontrada }}
	if err := nuevo(g).Eliminar(context.Background(), "paisajes"); err != nil {
		t.Fatal(err)
	}
	if len(g.llamadas) != 1 {
		t.Fatalf("nothing else must run for a missing VM: %v", g.llamadas)
	}
}

func TestListar(t *testing.T) {
	out := "\"Debian-Bookzilla\" {201014f6-cc79-4926-ab58-a26f4c4446be}\n\"mi vm {rara}\" {02a07c1f-0887-4a29-b33e-862322966a8e}\n\"paisajes\" {6432ef58-4603-4d01-ac02-af5522d76544}\n"
	g := &guion{respuesta: func([]string) (string, error) { return out, nil }}
	got, err := nuevo(g).Listar(context.Background())
	want := []string{"Debian-Bookzilla", "mi vm {rara}", "paisajes"}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("Listar = %q, %v", got, err)
	}
}

func TestParsearValor(t *testing.T) {
	tests := map[string]string{
		"Value: listo\n":          "listo",
		"Value: error:linea-12\n": "error:linea-12",
		"No value set!\n":         "",
		"":                        "",
	}
	for in, want := range tests {
		if got := parsearValor(in); got != want {
			t.Errorf("parsearValor(%q) = %q, want %q", in, got, want)
		}
	}
}
