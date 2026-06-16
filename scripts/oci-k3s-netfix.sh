#!/usr/bin/env bash
# Fix two recurring OCI-reboot failure modes that break k3s:
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
# Safe to re-run on any node at any time.
set -euo pipefail

echo "=== [1/3] Writing OCI-specific resolv.conf for k3s/CoreDNS ==="
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

echo "=== [2/3] Installing jobradar-iptables service ==="

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

echo "=== [3/3] Persisting kubeconfig mode in k3s config ==="
# k3s regenerates k3s.yaml with 0600 (root-only) on every restart by default.
# write-kubeconfig-mode: "0644" makes k3s set 644 after every start so the
# ubuntu deploy user (and CI via SSH) can read it without sudo.
if ! grep -q "write-kubeconfig-mode" /etc/rancher/k3s/config.yaml 2>/dev/null; then
    echo 'write-kubeconfig-mode: "0644"' >> /etc/rancher/k3s/config.yaml
    echo "  added write-kubeconfig-mode: 0644 to k3s config"
else
    echo "  write-kubeconfig-mode already set"
fi

echo "=== [4/4] Restarting k3s ==="
# Stop first so the unit is in a clean state before starting.
systemctl stop k3s 2>/dev/null || true
sleep 3
systemctl start k3s || true

echo -n "  waiting for k3s API (up to 120s)..."
deadline=$((SECONDS + 120))
while [ $SECONDS -lt $deadline ]; do
    if KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl get nodes 2>/dev/null | grep -q Ready; then
        break
    fi
    sleep 5
    echo -n "."
done
echo ""

if ! KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl get nodes 2>/dev/null | grep -q Ready; then
    echo "  ERROR: k3s not ready after 120s — dumping diagnostics:"
    systemctl status k3s.service --no-pager -l || true
    journalctl -xeu k3s.service --no-pager -n 80 || true
    exit 1
fi

echo "  k3s is ready"
KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl get nodes

echo ""
echo "Done. Run this script on every new node during bootstrap."
