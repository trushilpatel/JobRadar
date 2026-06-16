#!/usr/bin/env bash
# Fix recurring OCI-reboot failure modes that break k3s:
#
#  1. Stale KUBE-ROUTER-* iptables chains restored by netfilter-persistent
#     block pod→API connections (INPUT chain REJECT drops NEW packets from
#     the pod CIDR because ClusterIP DNAT routes pod→API through INPUT).
#
#  2. /etc/resolv.conf on OCI points to systemd-resolved's stub at 127.0.0.53,
#     which is unreachable from inside pods (each pod has its own loopback).
#     k3s reads /etc/rancher/k3s/resolv.conf to build the CoreDNS forward
#     directive; this file must list the real OCI DNS IPs.
#
#  3. Deprecated kube-apiserver flag --max-mutating-requests-in-flight was
#     removed in Kubernetes 1.30. k3s v1.30+ rejects it on startup, causing
#     k3s to exit immediately with "unknown flag". Removed before each start.
#
#  4. OCI A1 ARM nodes have slow cold-start I/O. k3s needs up to 10 minutes
#     to reach API-server Ready after a fresh start — poll for 600 s.
#
# Safe to re-run on any node at any time.
set -euo pipefail

echo "=== [1/5] Writing OCI-specific resolv.conf for k3s/CoreDNS ==="
mkdir -p /etc/rancher/k3s
cat > /etc/rancher/k3s/resolv.conf <<'EOF'
# OCI-specific upstream DNS for CoreDNS.
# /etc/resolv.conf on OCI points to systemd-resolved's stub at 127.0.0.53,
# which is unreachable from inside pods (each pod has its own loopback).
# k3s reads this file and generates "forward . <ip>" in the CoreDNS configmap.
nameserver 169.254.169.254
nameserver 8.8.8.8
nameserver 8.8.4.4
EOF
echo "  wrote /etc/rancher/k3s/resolv.conf"

echo "=== [2/5] Installing jobradar-iptables service ==="

cat > /usr/local/bin/jobradar-iptables-fix.sh <<'EOF'
#!/usr/bin/env bash
# Runs before k3s on every boot.
# Removes stale KUBE-ROUTER-* chains left by netfilter-persistent that
# block pod→API connections via the INPUT chain.
# Also ensures the pod CIDR (10.42.0.0/16) is accepted in INPUT so k3s
# ClusterIP DNAT (which routes pod→service traffic via INPUT) is not blocked.
set -euo pipefail

for chain in KUBE-ROUTER-FORWARD KUBE-ROUTER-INPUT KUBE-ROUTER-OUTPUT KUBE-ROUTER-FILTER; do
    if iptables -F "$chain" 2>/dev/null; then
        iptables -X "$chain" 2>/dev/null || true
        echo "cleared stale chain: $chain"
    fi
done

# Ensure pod CIDR is accepted in INPUT (before any REJECT rules)
if ! iptables -C INPUT -s 10.42.0.0/16 -j ACCEPT 2>/dev/null; then
    iptables -I INPUT 1 -s 10.42.0.0/16 -j ACCEPT
    echo "added pod CIDR ACCEPT to INPUT"
fi
EOF
chmod +x /usr/local/bin/jobradar-iptables-fix.sh

cat > /etc/systemd/system/jobradar-iptables.service <<'EOF'
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
EOF

systemctl daemon-reload
systemctl enable jobradar-iptables.service
systemctl start  jobradar-iptables.service
echo "  jobradar-iptables.service enabled and started"

echo "=== [3/5] Persisting kubeconfig mode in k3s config ==="
# k3s regenerates k3s.yaml with 0600 (root-only) on every restart by default.
# write-kubeconfig-mode: "0644" makes k3s set 644 after every start so the
# ubuntu deploy user (and CI via SSH) can read it without sudo.
if ! grep -q "write-kubeconfig-mode" /etc/rancher/k3s/config.yaml 2>/dev/null; then
    echo 'write-kubeconfig-mode: "0644"' >> /etc/rancher/k3s/config.yaml
    echo "  added write-kubeconfig-mode: 0644 to k3s config"
else
    echo "  write-kubeconfig-mode already set"
fi

echo "=== [4/5] Removing deprecated kube-apiserver flags from k3s config ==="
# --max-mutating-requests-in-flight was removed in Kubernetes 1.30.
# k3s v1.30+ bundles a kube-apiserver that rejects this flag on startup,
# causing k3s to exit immediately with "unknown flag". Remove it from any
# existing config.yaml before restarting.
if [ -f /etc/rancher/k3s/config.yaml ]; then
    if grep -q "max-mutating-requests-in-flight" /etc/rancher/k3s/config.yaml; then
        sed -i '/max-mutating-requests-in-flight/d' /etc/rancher/k3s/config.yaml
        echo "  removed deprecated max-mutating-requests-in-flight from k3s config"
    else
        echo "  no deprecated flags found"
    fi
else
    echo "  no k3s config.yaml found, skipping"
fi

echo "=== [5/5] Restarting k3s ==="
# Stop first so the unit is in a clean state before starting.
systemctl stop k3s 2>/dev/null || true
sleep 3
# Use --no-block so we don't wait for sd_notify: k3s sends READY after full
# API-server init which can exceed systemd's TimeoutStartSec on cold storage.
systemctl start --no-block k3s || true

echo -n "  waiting for k3s API (up to 600s)..."
deadline=$((SECONDS + 600))
while [ $SECONDS -lt $deadline ]; do
    # Ensure ubuntu user can read the kubeconfig once k3s regenerates it.
    chmod 644 /etc/rancher/k3s/k3s.yaml 2>/dev/null || true
    if KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl get nodes 2>/dev/null | grep -q Ready; then
        break
    fi
    sleep 5
    echo -n "."
done
echo ""

if ! KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl get nodes 2>/dev/null | grep -q Ready; then
    echo "  ERROR: k3s not ready after 600s — dumping diagnostics:"
    systemctl status k3s.service --no-pager -l || true
    journalctl -xeu k3s.service --no-pager -n 120 || true
    exit 1
fi

echo "  k3s is ready"
KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl get nodes

echo ""
echo "Done. Run this script on every new node during bootstrap."
