package vbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/config"
	"github.com/lyc4nthrope/httpaas-paginas-web/internal/oplog"
)

// The real runner notes every VBoxManage call in the job's operation log, with its
// output and exit code, also when it fails. A fake VBoxManage stands in for VirtualBox.
func TestRunnerRecordsEveryCall(t *testing.T) {
	falso := filepath.Join(t.TempDir(), "VBoxManage")
	guion := "#!/bin/sh\nif [ \"$1\" = list ]; then echo '\"web\" {1}'; exit 0; fi\necho 'VBOX_E_OBJECT_NOT_FOUND' >&2\nexit 1\n"
	if err := os.WriteFile(falso, []byte(guion), 0o755); err != nil {
		t.Fatal(err)
	}
	v := Nuevo(config.Config{VBox: config.VBox{VBoxManage: falso}})
	ctx, g := oplog.ConGrabadora(context.Background())
	if _, err := v.run(ctx, "list", "vms"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.run(ctx, "showvminfo", "otra"); err == nil {
		t.Fatal("exit 1 must be an error")
	}
	p := g.Pasos()
	if len(p) != 2 {
		t.Fatalf("steps = %+v", p)
	}
	if p[0].Cmd != "VBoxManage list vms" || p[0].Stdout != "\"web\" {1}\n" || p[0].ExitCode != 0 {
		t.Fatalf("first step = %+v", p[0])
	}
	if p[1].Cmd != "VBoxManage showvminfo otra" || p[1].ExitCode != 1 || p[1].Stderr != "VBOX_E_OBJECT_NOT_FOUND\n" {
		t.Fatalf("second step = %+v", p[1])
	}
}
