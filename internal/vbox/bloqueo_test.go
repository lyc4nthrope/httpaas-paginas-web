package vbox

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The text VirtualBox really printed on 2026-10-29 when a VM was used right after
// changing state.
var bloqueada = errors.New("VBoxManage: error: The machine 'paisajes' is already locked by a session (or being locked or unlocked); code VBOX_E_INVALID_OBJECT_STATE (0x80bb0007)")

func TestEliminarRetriesWhileTheVMIsStillLocked(t *testing.T) {
	intentos := 0
	g := &guion{respuesta: func(a []string) (string, error) {
		switch a[0] {
		case "showvminfo":
			return `VMState="poweroff"` + "\n", nil
		case "guestproperty":
			return "Value: paisajes\n", nil
		case "unregistervm":
			if intentos++; intentos < 3 {
				return "", bloqueada
			}
		}
		return "", nil
	}}
	if err := nuevo(g).Eliminar(context.Background(), "paisajes"); err != nil {
		t.Fatalf("Eliminar = %v, want success after the lock is released", err)
	}
	if intentos != 3 {
		t.Fatalf("unregistervm attempts = %d, want 3", intentos)
	}
}

func TestEliminarDoesNotRetryOtherErrors(t *testing.T) {
	intentos := 0
	g := &guion{respuesta: func(a []string) (string, error) {
		switch a[0] {
		case "showvminfo":
			return `VMState="poweroff"` + "\n", nil
		case "guestproperty":
			return "Value: paisajes\n", nil
		case "unregistervm":
			intentos++
			return "", errors.New("VBoxManage: error: Cannot delete the medium: permission denied")
		}
		return "", nil
	}}
	err := nuevo(g).Eliminar(context.Background(), "paisajes")
	if err == nil || !strings.Contains(err.Error(), "permission denied") || intentos != 1 {
		t.Fatalf("err=%v intentos=%d: only a lock may be retried", err, intentos)
	}
}

func TestEliminarGivesUpWhenTheLockNeverEnds(t *testing.T) {
	g := &guion{respuesta: func(a []string) (string, error) {
		switch a[0] {
		case "showvminfo":
			return `VMState="poweroff"` + "\n", nil
		case "guestproperty":
			return "Value: paisajes\n", nil
		}
		return "", bloqueada
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := nuevo(g).Eliminar(ctx, "paisajes"); err == nil {
		t.Fatal("a lock that never ends must become an error, not a hang")
	}
}
