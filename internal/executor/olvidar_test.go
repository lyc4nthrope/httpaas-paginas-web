package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILrGDQDw8Lvx036wtRzG4cjy9GkoBhfV4Rb/qdzjSFX5"

func TestOlvidarHostRemovesOnlyThatIP(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	content := "192.168.10.30 " + testKey + "\n192.168.10.31 " + testKey + "\n"
	if err := os.WriteFile(kh, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := OlvidarHost(context.Background(), kh, "192.168.10.30"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(kh)
	if strings.Contains(string(got), "192.168.10.30 ") || !strings.Contains(string(got), "192.168.10.31 ") {
		t.Fatalf("known_hosts after forgetting .30:\n%s", got)
	}
}

func TestOlvidarHostUnknownIPIsNotAnError(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(kh, []byte("192.168.10.31 "+testKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := OlvidarHost(context.Background(), kh, "192.168.10.99"); err != nil {
		t.Fatalf("unknown ip: %v", err)
	}
}

func TestOlvidarHostMissingFileIsNotAnError(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "no-existe")
	if err := OlvidarHost(context.Background(), kh, "192.168.10.30"); err != nil {
		t.Fatalf("missing file: %v", err)
	}
}

func TestOlvidarHostRejectsNonIP(t *testing.T) {
	for _, bad := range []string{"", "-f", "paisajes", "192.168.10.30; rm"} {
		if err := OlvidarHost(context.Background(), "unused", bad); err == nil {
			t.Errorf("OlvidarHost(%q) = nil, want error", bad)
		}
	}
}
