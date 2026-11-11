package oplog

import (
	"context"
	"strings"
	"sync"
)

// Grabadora collects the commands one operation runs. The service puts one in the
// context of each job, and the code that really runs commands (VBoxManage, ssh, scp)
// notes each step with Anotar. Calls without a recorder, such as the periodic ns
// check, record nothing.
type Grabadora struct {
	mu    sync.Mutex
	pasos []Step
}

type claveGrabadora struct{}

// ConGrabadora returns a context that carries a new recorder, and the recorder.
func ConGrabadora(ctx context.Context) (context.Context, *Grabadora) {
	g := &Grabadora{}
	return context.WithValue(ctx, claveGrabadora{}, g), g
}

// Anotar records s in the context's recorder, if there is one. A step equal to the
// previous one (same command, output and exit code), as the polls of EsperarLista are,
// is merged into it: Repeats counts the runs and the durations add up.
func Anotar(ctx context.Context, s Step) {
	g, ok := ctx.Value(claveGrabadora{}).(*Grabadora)
	if !ok {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if n := len(g.pasos); n > 0 && igual(g.pasos[n-1], s) {
		ultimo := &g.pasos[n-1]
		if ultimo.Repeats == 0 {
			ultimo.Repeats = 1
		}
		ultimo.Repeats++
		ultimo.DurationMS += s.DurationMS
		return
	}
	g.pasos = append(g.pasos, s)
}

func igual(a, b Step) bool {
	return a.Cmd == b.Cmd && a.Stdout == b.Stdout && a.Stderr == b.Stderr &&
		a.ExitCode == b.ExitCode && a.Error == b.Error
}

// Pasos returns a copy of the recorded steps, in order.
func (g *Grabadora) Pasos() []Step {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]Step(nil), g.pasos...)
}

// Canales names the channels the steps used, in order of first use: "VBoxManage",
// "SSH" (ssh and scp) or the program name. Empty when nothing ran.
func (g *Grabadora) Canales() string {
	var canales []string
	for _, p := range g.Pasos() {
		c, _, _ := strings.Cut(p.Cmd, " ")
		if c == "ssh" || c == "scp" {
			c = "SSH"
		}
		if !contiene(canales, c) {
			canales = append(canales, c)
		}
	}
	return strings.Join(canales, ", ")
}

func contiene(lista []string, s string) bool {
	for _, x := range lista {
		if x == s {
			return true
		}
	}
	return false
}
