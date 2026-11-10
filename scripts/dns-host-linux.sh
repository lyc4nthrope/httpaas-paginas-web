#!/usr/bin/env bash
# Make a Linux host resolve *.cloud.local through the platform DNS server (B5).
#
# Usage:
#   sudo ./scripts/dns-host-linux.sh
#   sudo ./scripts/dns-host-linux.sh --undo
#
# Two independent pieces:
#   1. nsswitch.conf: `dns` must come before `mdns4_minimal`. Debian sends every
#      `.local` name to mDNS first, and `[NOTFOUND=return]` stops the lookup
#      there, so cloud.local never reaches DNS.
#   2. The resolver: split DNS through systemd-resolved when it is running,
#      otherwise a first `nameserver` line in /etc/resolv.conf.
#
# Idempotent: a second run changes nothing.
set -euo pipefail

DNS_IP="${DNS_IP:-192.168.10.10}"
IFACE="${VBOX_IFACE:-vboxnet0}"
NSSWITCH="${NSSWITCH:-/etc/nsswitch.conf}"
RESOLV="${RESOLV:-/etc/resolv.conf}"

# Debian's line, used by --undo when there is no backup to restore.
HOSTS_LINE_DEFAULT='hosts:          files mdns4_minimal [NOTFOUND=return] dns'

say() { echo "[dns-host] $*"; }
die() { echo "[dns-host] $*" >&2; exit 1; }

# Prints the nsswitch content with `dns` moved before mDNS. Every other module
# keeps its original position: a distro that lists `mymachines`, `resolve` or
# `myhostname` does not lose them. Pure: no writes, so it can be tested.
nsswitch_con_dns() {
	local file="$1"
	awk '
		function join(prefix, n, tok,   i, out) {
			out = prefix
			for (i = 1; i <= n; i++) out = out tok[i] (i < n ? " " : "")
			return out
		}
		/^[[:space:]]*hosts:/ {
			if (done) next
			done = 1
			match($0, /^[[:space:]]*hosts:[[:space:]]*/)
			prefix = substr($0, 1, RLENGTH)
			if (prefix !~ /[[:space:]]$/) prefix = prefix " "
			n = 0
			for (i = 2; i <= NF; i++) tok[++n] = $i
			mdns = 0; dns = 0
			for (i = 1; i <= n; i++) {
				if (tok[i] ~ /^mdns[46]_minimal$/) { if (!mdns) mdns = i }
				if (tok[i] == "dns") { dns = i }
			}
			if (dns == 0) {
				pos = mdns ? mdns : 2
				if (pos > n + 1) pos = n + 1
				for (i = n; i >= pos; i--) tok[i + 1] = tok[i]
				tok[pos] = "dns"
				n++
			} else if (mdns && dns > mdns) {
				for (i = dns; i < n; i++) tok[i] = tok[i + 1]
				n--
				for (i = n; i >= mdns; i--) tok[i + 1] = tok[i]
				tok[mdns] = "dns"
				n++
			}
			print join(prefix, n, tok)
			next
		}
		{ print }
		END { if (!done) print "hosts: files dns" }
	' "$file"
}

# Prints the resolv.conf content with `nameserver DNS_IP` as the first entry and
# without duplicates of it further down. Pure, same reason.
resolv_con_dns() {
	local file="$1" escaped
	escaped="${DNS_IP//./\\.}"
	printf 'nameserver %s\n' "$DNS_IP"
	grep -v -E "^[[:space:]]*nameserver[[:space:]]+${escaped}[[:space:]]*$" "$file" || true
}

# True when the hosts line already has dns before mDNS, which is the only
# invariant this script enforces.
hosts_reordenado() {
	awk '
		/^[[:space:]]*hosts:/ {
			d = 0; m = 0
			for (i = 2; i <= NF; i++) {
				if ($i == "dns") d = i
				if ($i ~ /^mdns[46]_minimal$/) { if (!m) m = i }
			}
			ok = (d && m && d < m) ? 1 : 0
			exit
		}
		END { exit (ok ? 0 : 1) }
	' "$1"
}

# Keeps the original content once, so --undo can restore it exactly.
backup_once() {
	local file="$1" bak="$1.httpaas.bak"
	[[ -f "$bak" ]] || cp "$file" "$bak"
}

# Writes through the path instead of replacing it, so a symlink such as
# /etc/resolv.conf keeps pointing at its real target.
write_through() {
	local file="$1" content="$2"
	printf '%s\n' "$content" >"$file"
}

configurar() {
	local nuevo

	if [[ "$(cat "$NSSWITCH")" == "$(nsswitch_con_dns "$NSSWITCH")" ]]; then
		say "nsswitch: dns already comes before mdns4_minimal"
	else
		backup_once "$NSSWITCH"
		nuevo="$(nsswitch_con_dns "$NSSWITCH")"
		write_through "$NSSWITCH" "$nuevo"
		say "nsswitch: dns moved before mdns4_minimal"
	fi

	if systemctl is-active --quiet systemd-resolved 2>/dev/null; then
		ip link show "$IFACE" >/dev/null 2>&1 ||
			die "interface '$IFACE' not found: create the host-only adapter first (B0), or set VBOX_IFACE"
		resolvectl dns "$IFACE" "$DNS_IP"
		resolvectl domain "$IFACE" '~cloud.local'
		say "systemd-resolved: $IFACE -> $DNS_IP for ~cloud.local"
	else
		if [[ "$(head -n1 "$RESOLV" 2>/dev/null || true)" == "nameserver $DNS_IP" ]]; then
			say "resolv.conf: already starts with $DNS_IP"
		else
			backup_once "$RESOLV"
			nuevo="$(resolv_con_dns "$RESOLV")"
			write_through "$RESOLV" "$nuevo"
			say "resolv.conf: $DNS_IP added as the first nameserver"
		fi
	fi

	if getent hosts ns.cloud.local >/dev/null 2>&1; then
		say "ok: $(getent hosts ns.cloud.local | awk 'NR==1 { print $1, $2 }')"
	else
		die "ns.cloud.local still does not resolve: is the VM running and $DNS_IP reachable?"
	fi
}

deshacer() {
	local file bak

	for file in "$NSSWITCH" "$RESOLV"; do
		bak="$file.httpaas.bak"
		if [[ -f "$bak" ]]; then
			write_through "$file" "$(cat "$bak")"
			rm -f "$bak"
			say "restored $file"
		elif [[ "$file" == "$NSSWITCH" ]] && hosts_reordenado "$file"; then
			write_through "$file" "$(sed "s|^hosts:.*|$HOSTS_LINE_DEFAULT|" "$file")"
			say "nsswitch: restored the Debian default hosts line"
		fi
	done

	if systemctl is-active --quiet systemd-resolved 2>/dev/null; then
		resolvectl revert "$IFACE" 2>/dev/null || true
		say "systemd-resolved: reverted $IFACE"
	fi
}

main() {
	case "${1:-}" in
	"") configurar ;;
	--undo) deshacer ;;
	-h | --help)
		echo "usage: $0 [--undo]"
		echo "  VBOX_IFACE=<name>  host-only interface (default: vboxnet0)"
		echo "  DNS_IP=<ip>        DNS server (default: 192.168.10.10)"
		;;
	*) die "unknown option: $1" ;;
	esac
}

# Sourced by the tests: expose the pure functions and do nothing else.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	[[ "$(id -u)" -eq 0 ]] || exec sudo -- "$0" "$@"
	main "$@"
fi
