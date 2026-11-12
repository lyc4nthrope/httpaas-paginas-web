package oplog

import (
	"context"
	"testing"
)

func TestAnotarWithoutRecorderDoesNothing(t *testing.T) {
	Anotar(context.Background(), Step{Cmd: "VBoxManage list vms"}) // must not panic
}

func TestRecorderKeepsStepsInOrder(t *testing.T) {
	ctx, g := ConGrabadora(context.Background())
	Anotar(ctx, Step{Cmd: "VBoxManage createvm", ExitCode: 0, DurationMS: 5})
	Anotar(ctx, Step{Cmd: "ssh httpaas@192.168.10.10 -- sudo httpaas-dns agregar", ExitCode: 1, Stderr: "boom"})
	pasos := g.Pasos()
	if len(pasos) != 2 || pasos[0].Cmd != "VBoxManage createvm" || pasos[1].ExitCode != 1 || pasos[1].Stderr != "boom" {
		t.Fatalf("steps = %+v", pasos)
	}
}

// EsperarLista polls the same property every 2 s: identical consecutive steps become
// one step with a counter and the summed time, instead of one line per poll.
func TestRecorderMergesIdenticalConsecutiveSteps(t *testing.T) {
	ctx, g := ConGrabadora(context.Background())
	poll := Step{Cmd: "VBoxManage guestproperty get web /httpaas/estado", Stdout: "No value set!\n", DurationMS: 10}
	for i := 0; i < 3; i++ {
		Anotar(ctx, poll)
	}
	listo := poll
	listo.Stdout = "Value: listo\n"
	Anotar(ctx, listo)
	Anotar(ctx, poll) // not consecutive with the first run: a new step

	pasos := g.Pasos()
	if len(pasos) != 3 {
		t.Fatalf("want 3 steps, got %d: %+v", len(pasos), pasos)
	}
	if pasos[0].Repeats != 3 || pasos[0].DurationMS != 30 {
		t.Fatalf("merged step = %+v, want Repeats 3 and 30 ms", pasos[0])
	}
	if pasos[1].Repeats != 0 || pasos[2].Repeats != 0 {
		t.Fatalf("single steps must have no counter: %+v", pasos)
	}
}

func TestRecorderNamesTheChannels(t *testing.T) {
	ctx, g := ConGrabadora(context.Background())
	if got := g.Canales(); got != "" {
		t.Fatalf("no steps: channels = %q", got)
	}
	Anotar(ctx, Step{Cmd: "VBoxManage startvm web"})
	Anotar(ctx, Step{Cmd: "scp -- a httpaas@1.2.3.4:b"})
	Anotar(ctx, Step{Cmd: "ssh httpaas@1.2.3.4 -- x"})
	Anotar(ctx, Step{Cmd: "VBoxManage unregistervm web"})
	if got := g.Canales(); got != "VBoxManage, SSH" {
		t.Fatalf("channels = %q", got)
	}
}

// Pasos returns a copy: the caller cannot change what the recorder holds.
func TestRecorderReturnsACopy(t *testing.T) {
	ctx, g := ConGrabadora(context.Background())
	Anotar(ctx, Step{Cmd: "a"})
	g.Pasos()[0].Cmd = "changed"
	if g.Pasos()[0].Cmd != "a" {
		t.Fatal("Pasos must return a copy")
	}
}
