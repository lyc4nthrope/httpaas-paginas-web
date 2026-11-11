package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lyc4nthrope/httpaas-paginas-web/internal/oplog"
)

// Every ssh command of a job reaches the operation log: what ran, where, what it
// printed and how it ended. The stdin (the data sent) is not recorded.
func TestSSHRunIsRecorded(t *testing.T) {
	s := newTestSSH(func(string, []string, string) (string, string, int, error) {
		return "listo\n", "aviso\n", 3, nil
	})
	ctx, g := oplog.ConGrabadora(context.Background())
	if _, err := s.Run(ctx, Command{Name: "sudo", Args: []string{"httpaas-dns", "agregar", "web", "192.168.10.30"}, Stdin: "secreto"}); err != nil {
		t.Fatal(err)
	}
	p := g.Pasos()
	if len(p) != 1 {
		t.Fatalf("steps = %+v", p)
	}
	if p[0].Cmd != "ssh root@192.168.40.60 -- sudo httpaas-dns agregar web 192.168.10.30" ||
		p[0].Stdout != "listo\n" || p[0].Stderr != "aviso\n" || p[0].ExitCode != 3 || p[0].Error != "" {
		t.Fatalf("step = %+v", p[0])
	}
	if strings.Contains(p[0].Cmd, "secreto") {
		t.Fatal("stdin must not be recorded")
	}
}

// A transport failure is recorded too, with the reason, so the log explains why
// nothing ran on the VM.
func TestSSHChannelErrorIsRecorded(t *testing.T) {
	s := newTestSSH(func(string, []string, string) (string, string, int, error) {
		return "", "ssh: connect to host 192.168.40.60 port 22: No route to host\n", 255, nil
	})
	ctx, g := oplog.ConGrabadora(context.Background())
	if _, err := s.Run(ctx, Command{Name: "hostname"}); err == nil {
		t.Fatal("exit 255 must be a channel error")
	}
	p := g.Pasos()
	if len(p) != 1 || p[0].ExitCode != 255 || !strings.Contains(p[0].Error, "No route to host") {
		t.Fatalf("steps = %+v", p)
	}
}

func TestSCPCopyIsRecorded(t *testing.T) {
	s := newTestSSH(func(string, []string, string) (string, string, int, error) { return "", "", 0, nil })
	ctx, g := oplog.ConGrabadora(context.Background())
	if err := s.CopyTo(ctx, "/tmp/sitio.zip", "/home/httpaas/subidas/sitio.zip"); err != nil {
		t.Fatal(err)
	}
	p := g.Pasos()
	if len(p) != 1 || p[0].Cmd != "scp /tmp/sitio.zip root@192.168.40.60:/home/httpaas/subidas/sitio.zip" || p[0].ExitCode != 0 {
		t.Fatalf("steps = %+v", p)
	}
}

func TestOlvidarHostIsRecorded(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(kh, []byte("192.168.10.30 ssh-ed25519 AAAA\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, g := oplog.ConGrabadora(context.Background())
	if err := OlvidarHost(ctx, kh, "192.168.10.30"); err != nil {
		t.Fatal(err)
	}
	p := g.Pasos()
	if len(p) != 1 || p[0].Cmd != "ssh-keygen -R 192.168.10.30 -f "+kh || p[0].ExitCode != 0 {
		t.Fatalf("steps = %+v", p)
	}
}
