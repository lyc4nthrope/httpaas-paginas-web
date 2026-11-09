#!/usr/bin/env bash
# Converge a Debian 13 VM into the platform DNS server (docs/parte-b/PLAN.md, B3.3).
#
# Covers B1.5 to B3.2: packages, service user and keys, sshd hardening, the two
# zones, httpaas-dns and its sudoers rule. It assumes B1.1-B1.4 are already done
# (VM created, static 192.168.10.10, hostname ns).
#
# Idempotent: a second run leaves the same state. Zone files are only created when
# absent, because after B3 they hold dynamic records and overwriting them would
# drop every instance; use --forzar-zonas to overwrite deliberately.
#
# Usage: sudo ./instalar.sh [--llaves <dir>] [--forzar-zonas]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LLAVES_DIR="$SCRIPT_DIR/../llaves"
FORZAR_ZONAS=0
CHANGED=0

while [[ $# -gt 0 ]]; do
	case "$1" in
	--llaves)
		[[ $# -ge 2 ]] || {
			echo "--llaves needs a directory" >&2
			exit 2
		}
		LLAVES_DIR="$2"
		shift 2
		;;
	--forzar-zonas)
		FORZAR_ZONAS=1
		shift
		;;
	--help | -h)
		sed -n '2,14p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "unknown option: $1" >&2
		exit 2
		;;
	esac
done

[[ "$(id -u)" -eq 0 ]] || {
	echo "instalar.sh must run with root privileges (EUID 0)" >&2
	exit 1
}

say() { echo "[instalar] $*"; }

# sync_file <src> <dst> <owner> <group> <mode>
# Installs only when content, owner or mode differ. Returns 0 when it changed.
sync_file() {
	local src="$1" dst="$2" owner="$3" group="$4" mode="$5"
	if [[ -f "$dst" ]] &&
		cmp -s "$src" "$dst" &&
		[[ "$(stat -c '%U' "$dst")" == "$owner" ]] &&
		[[ "$(stat -c '%G' "$dst")" == "$group" ]] &&
		[[ "$(stat -c '%a' "$dst")" == "$mode" ]]; then
		return 1
	fi
	install -o "$owner" -g "$group" -m "$mode" "$src" "$dst"
	return 0
}

# --- B1.3 resolver for the VM itself -----------------------------------------
# The DHCP lease may hand out a resolver the VM cannot reach, which breaks apt
# and every `dig` without @server. Ask the VM's own named first (it forwards
# upstream) and the VirtualBox NAT resolver second, which is reachable even
# before bind9 is installed. `prepend` adds; it does not drop the lease servers.
dns_line='prepend domain-name-servers 127.0.0.1, 10.0.3.3;'
if ! grep -qxF "$dns_line" /etc/dhcp/dhclient.conf; then
	say "configuring dhclient resolver list"
	sed -i '/^[[:space:]]*prepend[[:space:]]\+domain-name-servers/d' /etc/dhcp/dhclient.conf
	printf '%s\n' "$dns_line" >>/etc/dhcp/dhclient.conf
	dhclient -r enp0s8 2>/dev/null || true
	dhclient enp0s8
fi

# --- B1.5 packages -----------------------------------------------------------
if ! dpkg -s bind9 bind9-dnsutils sudo >/dev/null 2>&1; then
	say "installing packages"
	apt-get update
	apt-get install -y bind9 bind9-dnsutils sudo
fi

# --- B1.6 service user and authorized keys -----------------------------------
if ! id -u httpaas >/dev/null 2>&1; then
	say "creating httpaas user"
	adduser --disabled-password --gecos "" httpaas
fi

if compgen -G "$LLAVES_DIR/*.pub" >/dev/null; then
	mkdir -p /home/httpaas/.ssh
	chown httpaas:httpaas /home/httpaas/.ssh
	chmod 700 /home/httpaas/.ssh

	tmp_keys="$(mktemp)"
	cat "$LLAVES_DIR"/*.pub >"$tmp_keys"
	if [[ ! -f /home/httpaas/.ssh/authorized_keys ]] ||
		! cmp -s "$tmp_keys" /home/httpaas/.ssh/authorized_keys; then
		say "installing authorized_keys"
		install -o httpaas -g httpaas -m 600 "$tmp_keys" /home/httpaas/.ssh/authorized_keys
	fi
	rm -f "$tmp_keys"
else
	say "warning: no public keys in $LLAVES_DIR; ssh access will not work"
fi

# --- B1.7 sshd hardening -----------------------------------------------------
sshd_tmp="$(mktemp)"
cat >"$sshd_tmp" <<'EOF'
PasswordAuthentication no
PermitRootLogin no
EOF
if sync_file "$sshd_tmp" /etc/ssh/sshd_config.d/10-httpaas.conf root root 644; then
	say "sshd config changed"
	sshd -t
	systemctl restart ssh
fi
rm -f "$sshd_tmp"

# --- B2 zones ----------------------------------------------------------------
if sync_file "$SCRIPT_DIR/named.conf.options" /etc/bind/named.conf.options root bind 644; then
	say "installed /etc/bind/named.conf.options"
	CHANGED=1
fi
if sync_file "$SCRIPT_DIR/named.conf.local" /etc/bind/named.conf.local root bind 644; then
	say "installed /etc/bind/named.conf.local"
	CHANGED=1
fi

for f in db.cloud.local db.192.168.10; do
	dst="/var/lib/bind/$f"
	if [[ -f "$dst" && "$FORZAR_ZONAS" -eq 0 ]]; then
		say "keeping $dst (dynamic zone; --forzar-zonas to overwrite)"
		continue
	fi
	if sync_file "$SCRIPT_DIR/$f" "$dst" bind bind 644; then
		say "installed $dst"
		CHANGED=1
	fi
done

say "validating bind configuration"
named-checkconf
named-checkzone cloud.local /var/lib/bind/db.cloud.local
named-checkzone 10.168.192.in-addr.arpa /var/lib/bind/db.192.168.10

if [[ "$CHANGED" -eq 1 ]]; then
	say "restarting named"
	systemctl restart named
fi
systemctl is-active --quiet named || {
	echo "named is not running" >&2
	exit 1
}

# --- B3.1 httpaas-dns --------------------------------------------------------
if sync_file "$SCRIPT_DIR/httpaas-dns" /usr/local/sbin/httpaas-dns root root 755; then
	say "installed /usr/local/sbin/httpaas-dns"
fi

# --- B3.2 sudoers ------------------------------------------------------------
# Validate the rule before it becomes active: a broken file in sudoers.d can
# leave the machine without a working sudo.
sudoers_tmp="$(mktemp)"
install -o root -g root -m 0440 "$SCRIPT_DIR/sudoers" "$sudoers_tmp"
visudo -cf "$sudoers_tmp"
if sync_file "$sudoers_tmp" /etc/sudoers.d/httpaas root root 440; then
	say "installed /etc/sudoers.d/httpaas"
fi
rm -f "$sudoers_tmp"

say "done"
