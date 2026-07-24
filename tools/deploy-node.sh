#!/bin/bash
# ==============================================================
# OneProxy Node — one-click shadowsocks-libev deploy
# Target: Ubuntu 20.04+ / Debian 11+, 1 vCPU, 1GB RAM, 25GB disk
# Usage: curl -sSL <this-script> | sudo bash   OR   sudo bash deploy-node.sh
# ==============================================================
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
err()   { echo -e "${RED}[ERROR]${NC} $*"; }

# ── Config ─────────────────────────────────────────────────────
SS_PORT="${SS_PORT:-8388}"
SS_METHOD="${SS_METHOD:-aes-256-gcm}"
SS_TIMEOUT="${SS_TIMEOUT:-300}"
CONF="/etc/shadowsocks-libev/config.json"
LOG_DIR="/var/log/shadowsocks-libev"

# ── Pre-flight ──────────────────────────────────────────────────
if [[ $EUID -ne 0 ]]; then
    err "This script must be run as root (use sudo)."
    exit 1
fi
info "OneProxy Node deploy starting — port=${SS_PORT} method=${SS_METHOD}"

# ── Dependencies (idempotent) ───────────────────────────────────
info "Checking dependencies..."
PKGS="shadowsocks-libev curl jq"
NEED_INSTALL=false
for pkg in $PKGS; do
    if dpkg -s "$pkg" &>/dev/null; then
        info "  $pkg: OK"
    else
        warn "  $pkg: will install"
        NEED_INSTALL=true
    fi
done
if $NEED_INSTALL; then
    apt-get update -qq
    apt-get install -y -qq $PKGS
    info "Dependencies installed."
fi

# ── Firewall ────────────────────────────────────────────────────
if command -v ufw &>/dev/null && ufw status 2>/dev/null | grep -q "active"; then
    ufw allow "$SS_PORT"/tcp 2>/dev/null || true
    info "Firewall: port $SS_PORT/tcp allowed (ufw)"
else
    info "Firewall: ufw not active, skipping (Vultr has external firewall)"
fi

# ── Password (keep existing if re-running) ──────────────────────
if [[ -f "$CONF" ]] && [[ -s "$CONF" ]]; then
    SS_PASS=$(jq -r '.password' "$CONF" 2>/dev/null || true)
fi
if [[ -z "${SS_PASS:-}" ]]; then
    # $(...) strips trailing newline, so password is clean.
    SS_PASS=$(openssl rand -base64 16)
    info "Generated new random password."
else
    info "Existing config found, keeping current password."
fi

# ── Write config ────────────────────────────────────────────────
mkdir -p "$(dirname "$CONF")"
cat > "$CONF" << JSONEOF
{
    "server":       "0.0.0.0",
    "server_port":  $SS_PORT,
    "password":     "$SS_PASS",
    "timeout":      $SS_TIMEOUT,
    "method":       "$SS_METHOD",
    "fast_open":    true,
    "mode":         "tcp_and_udp",
    "nameserver":   "8.8.8.8",
    "reuse_port":   true
}
JSONEOF
chmod 600 "$CONF"
info "Config written: $CONF"

# ── Log directory + logrotate ──────────────────────────────────
mkdir -p "$LOG_DIR"
cat > /etc/logrotate.d/shadowsocks-libev << 'LOGEOF'
/var/log/shadowsocks-libev/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    copytruncate
    maxsize 10M
}
LOGEOF
info "Logrotate: 7-day retention, 10MB max per file"

# ── systemd service ─────────────────────────────────────────────
# Stop any default Ubuntu package service first.
systemctl stop shadowsocks-libev 2>/dev/null || true

cat > /etc/systemd/system/shadowsocks-libev.service << SVC_EOF
[Unit]
Description=Shadowsocks-libev Server (OneProxy Node)
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/ss-server -c $CONF -v
Restart=always
RestartSec=5
StandardOutput=append:$LOG_DIR/ss.log
StandardError=append:$LOG_DIR/ss-error.log
LimitNOFILE=65535
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
SVC_EOF

# Ensure log files exist with correct permissions.
touch "$LOG_DIR/ss.log" "$LOG_DIR/ss-error.log"
chmod 644 "$LOG_DIR/ss.log" "$LOG_DIR/ss-error.log"

systemctl daemon-reload
systemctl enable shadowsocks-libev
systemctl restart shadowsocks-libev
sleep 1

if systemctl is-active --quiet shadowsocks-libev; then
    info "Service: running as systemd unit"
else
    err "Service failed to start. Check: journalctl -u shadowsocks-libev -n 20"
    exit 1
fi

# ── Kernel tuning (idempotent) ─────────────────────────────────
if ! grep -q "OneProxy Node" /etc/sysctl.conf 2>/dev/null; then
    cat >> /etc/sysctl.conf << 'KER_EOF'

# OneProxy Node optimization
net.core.default_qdisc = fq
net.ipv4.tcp_congestion_control = bbr
net.ipv4.tcp_fastopen = 3
KER_EOF
    sysctl -p &>/dev/null || true
    info "Kernel: TCP BBR + fast_open enabled (reboot may be needed)"
else
    info "Kernel: optimizations already applied"
fi

# ── Output subscription link ───────────────────────────────────
IP=$(curl -s -4 ifconfig.me 2>/dev/null || \
     curl -s -4 icanhazip.com 2>/dev/null || \
     hostname -I | awk '{print $1}')
SS_BASE64=$(echo -n "$SS_METHOD:$SS_PASS" | base64 -w0)
SS_URL="ss://${SS_BASE64}@${IP}:${SS_PORT}"
echo "$SS_URL" > /root/oneproxy-node-url.txt

echo ""
echo "=============================================="
echo -e "  ${GREEN}OneProxy Node — Deploy Complete${NC}"
echo "=============================================="
echo ""
echo -e "  ${YELLOW}${SS_URL}${NC}"
echo ""
echo "  Server  : $IP"
echo "  Port    : $SS_PORT"
echo "  Method  : $SS_METHOD"
echo "  Password: $SS_PASS"
echo ""
echo "  systemctl status  shadowsocks-libev"
echo "  tail -f $LOG_DIR/ss.log"
echo ""
echo "=============================================="
info "Subscription URL also saved to /root/oneproxy-node-url.txt"
