package executor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

// OlvidarHost removes every key recorded for ip from the service's known_hosts file.
// It runs when an instance is deleted: each clone generates new host keys, so without
// this the next VM that reuses the IP would be refused as a changed host (decision D8).
// A missing known_hosts file means there is nothing to forget, so it is not an error.
func OlvidarHost(ctx context.Context, knownHosts, ip string) error {
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("forget host: %q is not an IP address", ip)
	}
	if _, err := os.Stat(knownHosts); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	out, err := exec.CommandContext(ctx, "ssh-keygen", "-R", ip, "-f", knownHosts).CombinedOutput()
	if err != nil {
		return fmt.Errorf("forget host %s: %w: %s", ip, err, strings.TrimSpace(string(out)))
	}
	return nil
}
