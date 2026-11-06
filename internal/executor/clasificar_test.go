package executor

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

// realExitError returns the error os/exec really produces for a command, so the tests
// classify what Go returns in production, not a guess of it.
func realExitError(t *testing.T, name string, args ...string) (*exec.ExitError, int) {
	t.Helper()
	err := exec.Command(name, args...).Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("%s did not produce an *exec.ExitError: %v", name, err)
	}
	return ee, ee.ExitCode()
}

func TestSSHRunClassifiesTransportFailures(t *testing.T) {
	killed, killedCode := realExitError(t, "sh", "-c", "kill -9 $$")
	tests := []struct {
		name string
		code int
		err  error
	}{
		{"ssh binary missing", 0, exec.ErrNotFound},
		{"process killed by timeout or signal", killedCode, killed},
		{"transport exit 255", 255, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestSSH(func(string, []string, string) (string, string, int, error) { return "", "", tt.code, tt.err })
			_, err := s.Run(context.Background(), Command{Name: "hostname"})
			var ce *ChannelError
			if !errors.As(err, &ce) {
				t.Fatalf("Run = %v, want a ChannelError (never a success)", err)
			}
		})
	}
}

func TestSSHRunRealRemoteExitIsAResult(t *testing.T) {
	ee, code := realExitError(t, "false")
	s := newTestSSH(func(string, []string, string) (string, string, int, error) { return "", "no such file", code, ee })
	res, err := s.Run(context.Background(), Command{Name: "cat", Args: []string{"/nope"}})
	if err != nil || res.ExitCode != code {
		t.Fatalf("a remote command failing with code %d is a result, got %+v, %v", code, res, err)
	}
}

func TestSCPRemoteFailureIsNotAChannelError(t *testing.T) {
	ee, code := realExitError(t, "false")
	s := newTestSSH(func(string, []string, string) (string, string, int, error) {
		return "", "scp: /home/httpaas/subidas: No such file or directory", code, ee
	})
	err := s.CopyTo(context.Background(), "/tmp/a.zip", "/home/httpaas/subidas/a.zip")
	var ce *ChannelError
	if err == nil || errors.As(err, &ce) {
		t.Fatalf("a missing remote folder must be a plain error, not 'no connection': %v", err)
	}
}

func TestSCPTransportFailuresAreChannelErrors(t *testing.T) {
	for _, tt := range []struct {
		name string
		code int
		err  error
	}{
		{"scp binary missing", 0, exec.ErrNotFound},
		{"transport exit 255", 255, errors.New("exit status 255")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestSSH(func(string, []string, string) (string, string, int, error) { return "", "", tt.code, tt.err })
			var ce *ChannelError
			if err := s.CopyTo(context.Background(), "/tmp/a", "/tmp/b"); !errors.As(err, &ce) {
				t.Fatalf("CopyTo = %v, want ChannelError", err)
			}
		})
	}
}
