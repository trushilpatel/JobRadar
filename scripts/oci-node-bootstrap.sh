#!/usr/bin/env bash
# oci-node-bootstrap.sh — one-time setup for a fresh OCI ARM A1 instance.
# Identical software stack on every node; which services run is controlled by
# the Helm values files (values.prod-infra.yaml / values.prod-app.yaml).
#
# Usage:
#   bash oci-node-bootstrap.sh
#
# Optional env overrides (skip interactive prompts):
#   GH_USER=<github-username> GH_PAT=<read:packages PAT> bash oci-node-bootstrap.sh
#
# Optional zram (compressed RAM swap): off by default since only the APP
# (pipeline) node has shown measured memory/iowait pressure — see
# infra/CLAUDE.md "Why swap is required". Opt in per node at bootstrap time:
#   ENABLE_ZRAM=true bash oci-node-bootstrap.sh
#
# Idempotent — safe to re-run on the same node at any time.
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; NC='\033[0m'

ok()   { echo -e "${GREEN}  ✓${NC} $*"; }
warn() { echo -e "${YELLOW}  !${NC} $*"; }
die()  { echo -e "${RED}  ✗${NC} $*"; exit 1; }
step() { echo -e "\n${BOLD}$*${NC}"; }

NS="jobradar-local"
KUBECONFIG_PATH="/etc/rancher/k3s/k3s.yaml"

# ── 0. Swap (2 GB) ────────────────────────────────────────────────────────────
step "[0/6] Swap"
if swapon --show | grep -q '/swapfile'; then
  ok "swapfile already active"
elif [[ -f /swapfile ]]; then
  sudo swapon /swapfile
  ok "swapfile activated"
else
  echo "  Creating 2 GB swapfile..."
  sudo fallocate -l 2G /swapfile
  sudo chmod 600 /swapfile
  sudo mkswap /swapfile
  sudo swapon /swapfile
  grep -q swapfile /etc/fstab || echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab >/dev/null
  ok "2 GB swap created and persisted"
fi

# ── 1. k3s ────────────────────────────────────────────────────────────────────
step "[1/6] k3s"

# Write config before installing — disables components we don't need (saves ~200MB RAM)
sudo mkdir -p /etc/rancher/k3s
if [[ ! -f /etc/rancher/k3s/config.yaml ]]; then
  sudo tee /etc/rancher/k3s/config.yaml >/dev/null <<'EOF'
disable:
  - traefik
  - servicelb
  - metrics-server
disable-network-policy: true
# OCI /etc/resolv.conf points to systemd-resolved stub (127.0.0.53) which is
# unreachable from inside pods. Override here with real OCI DNS IPs so CoreDNS
# can forward external queries correctly.
resolv-conf: /etc/rancher/k3s/resolv.conf
# k3s regenerates k3s.yaml with 0600 (root-only) on every restart by default.
# 0644 lets the ubuntu deploy user and CI read the kubeconfig without sudo.
write-kubeconfig-mode: "0644"
EOF
  ok "k3s config written"
else
  # Ensure write-kubeconfig-mode is set even if config already exists
  if ! grep -q "write-kubeconfig-mode" /etc/rancher/k3s/config.yaml; then
    echo 'write-kubeconfig-mode: "0644"' | sudo tee -a /etc/rancher/k3s/config.yaml >/dev/null
    ok "added write-kubeconfig-mode to existing k3s config"
  else
    ok "k3s config already exists"
  fi
fi

# OCI-specific resolv.conf for k3s/CoreDNS (must exist before k3s starts if
# config.yaml references it; idempotent if already written by oci-k3s-netfix.sh)
if [[ ! -f /etc/rancher/k3s/resolv.conf ]]; then
  sudo tee /etc/rancher/k3s/resolv.conf >/dev/null <<'EOF'
nameserver 169.254.169.254
nameserver 8.8.8.8
nameserver 8.8.4.4
EOF
  ok "k3s resolv.conf written (OCI DNS)"
else
  ok "k3s resolv.conf already exists"
fi

if command -v k3s >/dev/null 2>&1; then
  ok "k3s already installed ($(k3s --version | head -1))"
else
  echo "  Installing k3s..."
  curl -sfL https://get.k3s.io | sh -
  ok "k3s installed"
fi

echo "  Waiting for node to become Ready..."
until sudo KUBECONFIG="$KUBECONFIG_PATH" kubectl get nodes 2>/dev/null | grep -q " Ready"; do sleep 2; done
ok "Node is Ready"

# write-kubeconfig-mode in config.yaml means k3s already set 644; chmod 644
# here is belt-and-suspenders for the initial install before config was present.
sudo chmod 644 "$KUBECONFIG_PATH"
ok "kubeconfig permissions set (644)"
export KUBECONFIG="$KUBECONFIG_PATH"

# Persist for future sessions
if ! grep -q "KUBECONFIG=$KUBECONFIG_PATH" ~/.bashrc 2>/dev/null; then
  echo "export KUBECONFIG=$KUBECONFIG_PATH" >> ~/.bashrc
  ok "KUBECONFIG added to ~/.bashrc"
else
  ok "KUBECONFIG already in ~/.bashrc"
fi

# Install the iptables cleanup service so stale KUBE-ROUTER-* chains (restored by
# netfilter-persistent after OCI reboots) don't block pod→API connections.
sudo tee /usr/local/bin/jobradar-iptables-fix.sh >/dev/null <<'SCRIPT'
#!/usr/bin/env bash
for chain in KUBE-ROUTER-FORWARD KUBE-ROUTER-INPUT KUBE-ROUTER-OUTPUT KUBE-ROUTER-FILTER; do
    if iptables -F "$chain" 2>/dev/null; then iptables -X "$chain" 2>/dev/null || true; fi
done
iptables -C INPUT -s 10.42.0.0/16 -j ACCEPT 2>/dev/null || \
    iptables -I INPUT 1 -s 10.42.0.0/16 -j ACCEPT
SCRIPT
sudo chmod +x /usr/local/bin/jobradar-iptables-fix.sh

sudo tee /etc/systemd/system/jobradar-iptables.service >/dev/null <<'SVC'
[Unit]
Description=Clear stale KUBE-ROUTER iptables chains before k3s starts
DefaultDependencies=no
Before=k3s.service
After=network-pre.target
[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/local/bin/jobradar-iptables-fix.sh
[Install]
WantedBy=multi-user.target
SVC
sudo systemctl daemon-reload
sudo systemctl enable --now jobradar-iptables.service
ok "jobradar-iptables.service installed and enabled"

# ── 2. Helm ───────────────────────────────────────────────────────────────────
step "[2/6] Helm"
if command -v helm >/dev/null 2>&1; then
  ok "helm already installed ($(helm version --short))"
else
  echo "  Installing Helm..."
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
  ok "helm installed"
fi

# ── 3. jq ─────────────────────────────────────────────────────────────────────
step "[3/6] jq"
if command -v jq >/dev/null 2>&1; then
  ok "jq already installed ($(jq --version))"
else
  echo "  Installing jq..."
  if command -v apt-get >/dev/null 2>&1; then
    sudo DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a apt-get install -y jq
  elif command -v yum >/dev/null 2>&1; then
    sudo yum install -y jq
  else
    die "Unknown package manager — install jq manually then re-run"
  fi
  ok "jq installed"
fi

# ── 4. Doppler CLI ────────────────────────────────────────────────────────────
step "[4/6] Doppler CLI"
if command -v doppler >/dev/null 2>&1; then
  ok "doppler already installed ($(doppler --version))"
else
  echo "  Installing Doppler CLI..."
  curl -Ls https://cli.doppler.com/install.sh | sudo sh
  ok "doppler installed"
fi

if doppler secrets get SUPABASE_URL >/dev/null 2>&1; then
  ok "Doppler already authenticated and configured"
else
  warn "Doppler not yet authenticated. Run these commands manually after bootstrap:"
  echo "    doppler login"
  echo "    doppler setup   # select project: job-radar, config: prod"
fi

# ── 5. Kubernetes namespace ───────────────────────────────────────────────────
step "[5/7] Kubernetes namespace"
# k3s API server may still be starting after a reboot — wait until it accepts connections
echo "  Waiting for k3s API server..."
until kubectl get nodes >/dev/null 2>&1; do sleep 3; done
ok "k3s API server is ready"
if kubectl get namespace "$NS" >/dev/null 2>&1; then
  ok "Namespace '$NS' already exists"
else
  kubectl create namespace "$NS"
  ok "Namespace '$NS' created"
fi

# ── 6. Harden CoreDNS against CPU-steal spikes (self-healing) ───────────────
# The 1-OCPU OCI free-tier nodes run at high CPU steal + swap pressure. With
# the stock probes (liveness timeout 1s/period 10s, readiness timeout
# 1s/period 2s) CoreDNS misses probes during a starvation spike, gets
# kubelet-restarted or pulled from Service endpoints, and cluster DNS drops —
# breaking external lookups (Supabase, NATS, the Gemini API) for every pod
# still on the ClusterFirst dnsPolicy, which used to cascade into
# filter/enricher failures. Widen both probes.
#
# This used to be a one-off `kubectl patch` run only at bootstrap time, but
# k3s re-applies its bundled CoreDNS addon on every k3s restart/upgrade,
# silently reverting it — that's exactly what happened in production (the
# patch was applied once, reverted on a later k3s restart, and nobody
# re-ran bootstrap). Installed as a systemd timer instead so it self-heals
# without depending on a human noticing.
step "[6/7] Harden CoreDNS liveness/readiness probes (self-healing via timer)"

sudo tee /usr/local/bin/jobradar-coredns-harden.sh >/dev/null <<'SCRIPT'
#!/usr/bin/env bash
# Idempotent — re-applying the same values is a no-op (no rollout, no
# restart). Safe to run every few minutes indefinitely.
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl -n kube-system get deploy coredns >/dev/null 2>&1 || exit 0
kubectl -n kube-system patch deploy coredns --type=json -p '[
  {"op":"replace","path":"/spec/template/spec/containers/0/livenessProbe/failureThreshold","value":10},
  {"op":"replace","path":"/spec/template/spec/containers/0/livenessProbe/periodSeconds","value":20},
  {"op":"replace","path":"/spec/template/spec/containers/0/livenessProbe/timeoutSeconds","value":10},
  {"op":"replace","path":"/spec/template/spec/containers/0/readinessProbe/failureThreshold","value":10},
  {"op":"replace","path":"/spec/template/spec/containers/0/readinessProbe/periodSeconds","value":10},
  {"op":"replace","path":"/spec/template/spec/containers/0/readinessProbe/timeoutSeconds","value":5}
]' >/dev/null 2>&1 || true
SCRIPT
sudo chmod +x /usr/local/bin/jobradar-coredns-harden.sh

sudo tee /etc/systemd/system/jobradar-coredns-harden.service >/dev/null <<'SVC'
[Unit]
Description=Widen CoreDNS liveness/readiness probe timeouts (idempotent)
After=k3s.service
[Service]
Type=oneshot
ExecStart=/usr/local/bin/jobradar-coredns-harden.sh
SVC

sudo tee /etc/systemd/system/jobradar-coredns-harden.timer >/dev/null <<'TMR'
[Unit]
Description=Periodically re-apply CoreDNS probe hardening (k3s reverts it on every restart/upgrade)
[Timer]
OnBootSec=1min
OnUnitActiveSec=3min
[Install]
WantedBy=timers.target
TMR

sudo systemctl daemon-reload
sudo systemctl enable --now jobradar-coredns-harden.timer
sudo systemctl start jobradar-coredns-harden.service
ok "CoreDNS hardening installed as a self-healing systemd timer (re-applies every 3 min)"

# ── 7. GHCR imagePullSecret ───────────────────────────────────────────────────
step "[7/7] GHCR imagePullSecret"
if kubectl get secret ghcr-pull-secret -n "$NS" >/dev/null 2>&1; then
  ok "ghcr-pull-secret already exists in $NS"
else
  # Use env vars if provided, otherwise prompt interactively
  if [[ -z "${GH_USER:-}" || -z "${GH_PAT:-}" ]]; then
    echo ""
    echo "  A GitHub PAT with 'read:packages' scope is required."
    echo "  Create one at: https://github.com/settings/tokens/new"
    echo "  Or re-run with:  GH_USER=<user> GH_PAT=<token> bash oci-node-bootstrap.sh"
    echo ""
    read -rp "  GitHub username: " GH_USER
    read -rsp "  GitHub PAT (hidden): " GH_PAT
    echo ""
  fi

  kubectl create secret docker-registry ghcr-pull-secret \
    --docker-server=ghcr.io \
    --docker-username="$GH_USER" \
    --docker-password="$GH_PAT" \
    --namespace "$NS"
  ok "ghcr-pull-secret created in $NS"
fi

# ── Done ──────────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Bootstrap complete.${NC}"
echo ""
echo "Remaining manual steps:"
echo "  1. If Doppler auth was skipped:  doppler login && doppler setup"
echo "  2. Open OCI Security List ports in the OCI Console:"
echo "       INFRA node: TCP 22, 30080, 30300"
echo "       APP node:   TCP 22 only"
echo "  3. Add GitHub repo secrets (Settings → Secrets → Actions):"
echo "       OCI_INFRA_HOST, OCI_APP_HOST, OCI_SSH_USER, OCI_SSH_KEY"
echo "       VITE_CLERK_PUBLISHABLE_KEY, CLERK_JWKS_URL, CLERK_ISSUER_URL"
echo "       CODECOV_TOKEN"
echo ""
