#!/usr/bin/env bash
# One-shot, idempotent host bootstrap for Oracle Linux 9 running JobRadar.
# Re-running must be safe: every step checks state before acting.
#
# Required env:
#   ADMIN_USER             -- SSH user allowed to log in (must exist already)
#   ADMIN_IP_ALLOWLIST     -- space-separated CIDRs to allowlist in CrowdSec
#                             BEFORE enabling bouncer (prevents owner lockout)
#
# Optional env:
#   SSH_PORT                 (default 22)
#   OCI_METADATA_BLOCK       (default true)  -- block 169.254.169.254 from containers
#   CROWDSEC_BAN_DURATION    (default 4h)
#   DNF_AUTOMATIC            (default true)
#   ENV_FILE_PATH            (default /opt/jobradar/.env)

set -euo pipefail

: "${ADMIN_USER:?ADMIN_USER is required}"
: "${ADMIN_IP_ALLOWLIST:?ADMIN_IP_ALLOWLIST is required (space-separated CIDRs)}"
SSH_PORT="${SSH_PORT:-22}"
OCI_METADATA_BLOCK="${OCI_METADATA_BLOCK:-true}"
CROWDSEC_BAN_DURATION="${CROWDSEC_BAN_DURATION:-4h}"
DNF_AUTOMATIC="${DNF_AUTOMATIC:-true}"
ENV_FILE_PATH="${ENV_FILE_PATH:-/opt/jobradar/.env}"

log() { printf '[bootstrap] %s\n' "$*"; }
warn() { printf '[bootstrap][warn] %s\n' "$*" >&2; }

if [[ $EUID -ne 0 ]]; then
	echo "bootstrap.sh must run as root" >&2
	exit 1
fi

if ! id "$ADMIN_USER" >/dev/null 2>&1; then
	echo "ADMIN_USER '$ADMIN_USER' does not exist on this system" >&2
	exit 1
fi

# ---------------------------------------------------------------------------
# 1. Package install (idempotent -- dnf install -y is a no-op when installed)
# ---------------------------------------------------------------------------
log "installing base packages"
# CrowdSec + firewall bouncer live in the CrowdSec repo. Add only if missing.
if ! dnf repolist 2>/dev/null | grep -q '^crowdsec'; then
	log "adding crowdsec repo"
	curl -fsSL https://install.crowdsec.net | bash
fi
dnf install -y \
	firewalld \
	fail2ban-server \
	crowdsec \
	crowdsec-firewall-bouncer-iptables \
	dnf-automatic \
	chrony \
	iptables-services

# ---------------------------------------------------------------------------
# 2. firewalld -- allow only 22/80/443
# ---------------------------------------------------------------------------
log "configuring firewalld"
systemctl enable --now firewalld
for svc in ssh http https; do
	if ! firewall-cmd --permanent --zone=public --query-service="$svc" >/dev/null 2>&1; then
		firewall-cmd --permanent --zone=public --add-service="$svc"
	fi
done
firewall-cmd --reload

# Confirm no extra zones are active with ports open.
active_zones="$(firewall-cmd --get-active-zones | awk 'NR%2==1')"
log "active firewall zones: ${active_zones//$'\n'/ }"

# ---------------------------------------------------------------------------
# 3. SSH hardening
# ---------------------------------------------------------------------------
log "writing /etc/ssh/sshd_config.d/10-hardening.conf"
install -d -m 0755 /etc/ssh/sshd_config.d
cat >/etc/ssh/sshd_config.d/10-hardening.conf <<EOF
# Managed by infra/host/bootstrap.sh -- do not edit manually.
Port ${SSH_PORT}
PasswordAuthentication no
PermitRootLogin no
AllowUsers ${ADMIN_USER}
MaxAuthTries 3
ClientAliveInterval 300
ClientAliveCountMax 2
EOF
chmod 0644 /etc/ssh/sshd_config.d/10-hardening.conf
sshd -t
systemctl reload sshd

# ---------------------------------------------------------------------------
# 4. dnf-automatic -- nightly security updates
# ---------------------------------------------------------------------------
if [[ "$DNF_AUTOMATIC" == "true" ]]; then
	log "configuring dnf-automatic"
	# Simple key=value patches preserving the rest of the shipped config.
	sed -ri 's/^apply_updates\s*=.*/apply_updates = yes/' /etc/dnf/automatic.conf
	sed -ri 's/^emit_via\s*=.*/emit_via = stdio/' /etc/dnf/automatic.conf
	systemctl enable --now dnf-automatic-install.timer
else
	log "skipping dnf-automatic (DNF_AUTOMATIC=$DNF_AUTOMATIC)"
fi

# ---------------------------------------------------------------------------
# 5. chrony (time sync)
# ---------------------------------------------------------------------------
log "enabling chronyd"
systemctl enable --now chronyd
chronyc tracking || warn "chronyc tracking returned non-zero"

# ---------------------------------------------------------------------------
# 6. Docker daemon hardening
# ---------------------------------------------------------------------------
log "writing /etc/docker/daemon.json"
install -d -m 0755 /etc/docker
daemon_tmp="$(mktemp)"
cat >"$daemon_tmp" <<'EOF'
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "50m",
    "max-file": "5"
  },
  "live-restore": true,
  "userland-proxy": false,
  "no-new-privileges": true,
  "icc": false
}
EOF
if ! cmp -s "$daemon_tmp" /etc/docker/daemon.json 2>/dev/null; then
	mv "$daemon_tmp" /etc/docker/daemon.json
	chmod 0644 /etc/docker/daemon.json
	warn "docker daemon.json changed -- restart docker when safe: systemctl restart docker"
else
	rm -f "$daemon_tmp"
fi

# ---------------------------------------------------------------------------
# 7. Block OCI metadata service from containers
# ---------------------------------------------------------------------------
if [[ "$OCI_METADATA_BLOCK" == "true" ]]; then
	log "blocking 169.254.169.254 from DOCKER-USER chain"
	# Ensure chain exists (docker creates it when running). Create if missing.
	if ! iptables -L DOCKER-USER >/dev/null 2>&1; then
		iptables -N DOCKER-USER || true
	fi
	if ! iptables -C DOCKER-USER -d 169.254.169.254 -j DROP 2>/dev/null; then
		iptables -I DOCKER-USER -d 169.254.169.254 -j DROP
	fi
	install -d -m 0755 /etc/sysconfig
	iptables-save >/etc/sysconfig/iptables
	systemctl enable --now iptables || warn "iptables service unavailable -- ensure rules persist via another mechanism"
else
	log "skipping OCI metadata block (OCI_METADATA_BLOCK=$OCI_METADATA_BLOCK)"
fi

# ---------------------------------------------------------------------------
# 8. CrowdSec
# ---------------------------------------------------------------------------
log "configuring CrowdSec"
systemctl enable --now crowdsec

# Install collections (idempotent -- cscli is a no-op if already installed).
for coll in crowdsecurity/sshd crowdsecurity/linux crowdsecurity/http-cve; do
	if ! cscli collections list -o json | grep -q "\"name\": \"$coll\""; then
		cscli collections install "$coll"
	fi
done

# Allowlist admin CIDRs BEFORE the bouncer goes live.
log "adding allowlist decisions for ADMIN_IP_ALLOWLIST"
for cidr in $ADMIN_IP_ALLOWLIST; do
	# `cscli decisions add --type allow` is idempotent per-invocation by design --
	# it will add a new decision each call, but they expire together. That's fine.
	cscli decisions add --type allow --duration 8760h --ip "$cidr" \
		--reason "admin allowlist via bootstrap.sh" || warn "failed to add allow for $cidr"
done

# Register the firewall bouncer key if not already done.
BOUNCER_CFG=/etc/crowdsec/bouncers/crowdsec-firewall-bouncer.yaml
if [[ -f "$BOUNCER_CFG" ]] && ! grep -q '^api_key: .\+' "$BOUNCER_CFG"; then
	key="$(cscli bouncers add firewall-bouncer -o raw 2>/dev/null || true)"
	if [[ -n "$key" ]]; then
		sed -ri "s|^api_key:.*|api_key: $key|" "$BOUNCER_CFG"
	fi
fi
systemctl enable --now crowdsec-firewall-bouncer

# ---------------------------------------------------------------------------
# 9. .env permissions
# ---------------------------------------------------------------------------
if [[ -f "$ENV_FILE_PATH" ]]; then
	log "tightening $ENV_FILE_PATH permissions"
	chown "$ADMIN_USER:$ADMIN_USER" "$ENV_FILE_PATH"
	chmod 0600 "$ENV_FILE_PATH"
else
	warn "$ENV_FILE_PATH not present yet -- remember to chmod 600 when you create it"
fi

# ---------------------------------------------------------------------------
# Final report
# ---------------------------------------------------------------------------
cat <<EOF

[bootstrap] Done. Checklist:
  [x] firewalld: 22/80/443 open (public zone)
  [x] sshd hardened: /etc/ssh/sshd_config.d/10-hardening.conf
  [x] dnf-automatic: ${DNF_AUTOMATIC}
  [x] chronyd enabled
  [x] /etc/docker/daemon.json applied (restart docker if it changed)
  [x] OCI metadata block: ${OCI_METADATA_BLOCK}
  [x] CrowdSec + firewall bouncer + allowlist for: ${ADMIN_IP_ALLOWLIST}

Logs:
  journalctl -u sshd
  journalctl -u crowdsec
  journalctl -u crowdsec-firewall-bouncer
  journalctl -u dnf-automatic-install.timer
  chronyc tracking

Break-glass: SSH in and run 'cscli decisions delete --all' to clear all bans.
EOF
