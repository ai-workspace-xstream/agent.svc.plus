#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}Starting Agent Service Plus Installation...${NC}"

XRAY_TCP_USER="caddy"
OPEN_STUNNEL_5443="${OPEN_STUNNEL_5443:-false}"
STANDALONE_MODE=false
STANDALONE_UUID_FILE="/usr/local/etc/xray/standalone.uuid"

is_truthy() {
    case "${1:-}" in
        1|true|TRUE|yes|YES|y|Y|on|ON)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

apply_low_latency_tuning() {
    echo -e "${GREEN}[post] Applying low-latency kernel tuning (BBR/fq)...${NC}"

    cat > /etc/sysctl.d/99-agent-lowlatency.conf <<'EOF'
# Agent low-latency tuning (safe baseline)
net.ipv4.tcp_congestion_control = bbr
net.core.default_qdisc = fq
net.ipv4.tcp_fastopen = 3

# Queue/backlog
net.core.somaxconn = 8192
net.ipv4.tcp_max_syn_backlog = 8192
net.core.netdev_max_backlog = 16384

# Socket buffers
net.core.rmem_max = 67108864
net.core.wmem_max = 67108864
net.ipv4.tcp_rmem = 4096 87380 33554432
net.ipv4.tcp_wmem = 4096 65536 33554432

# Better handling for path MTU edge-cases
net.ipv4.tcp_mtu_probing = 1
EOF

    if ! sysctl --system >/tmp/agent-lowlatency-sysctl.log 2>&1; then
        echo -e "${YELLOW}sysctl --system returned non-zero; checking applied values...${NC}"
        tail -n 80 /tmp/agent-lowlatency-sysctl.log || true
    fi

    sysctl net.ipv4.tcp_congestion_control \
        net.core.default_qdisc \
        net.ipv4.tcp_fastopen \
        net.core.somaxconn \
        net.core.netdev_max_backlog \
        net.ipv4.tcp_max_syn_backlog \
        net.core.rmem_max \
        net.core.wmem_max \
        net.ipv4.tcp_mtu_probing || true
}

setup_fq_qdisc_service() {
    echo -e "${GREEN}[post] Ensuring fq qdisc persistence service...${NC}"

    cat > /usr/local/sbin/apply-fq-qdisc.sh <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

while IFS= read -r dev; do
    [ "$dev" = "lo" ] && continue
    tc qdisc replace dev "$dev" root fq >/dev/null 2>&1 || true
done < <(ip -o link show | awk -F': ' '{print $2}' | cut -d'@' -f1)
EOF
    chmod +x /usr/local/sbin/apply-fq-qdisc.sh

    cat > /etc/systemd/system/apply-fq-qdisc.service <<'EOF'
[Unit]
Description=Apply fq qdisc for low-latency pacing
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/apply-fq-qdisc.sh

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable --now apply-fq-qdisc.service || true
}

ensure_ufw_ports() {
    local ufw_ports=(80 443 1443)

    if is_truthy "$OPEN_STUNNEL_5443"; then
        ufw_ports+=(5443)
    fi

    echo -e "${GREEN}[post] UFW check (${ufw_ports[*]})...${NC}"

    if ! command -v ufw >/dev/null 2>&1; then
        echo -e "${YELLOW}ufw not installed; skipping UFW checks.${NC}"
        return 0
    fi

    UFW_STATE="$(ufw status | head -n 1 | awk '{print $2}')"
    if [ "$UFW_STATE" != "active" ]; then
        echo -e "${YELLOW}ufw is installed but not active; skipping automatic rule changes.${NC}"
        return 0
    fi

    for port in "${ufw_ports[@]}"; do
        ufw allow "${port}/tcp" >/dev/null 2>&1 || true
    done
    ufw reload >/dev/null 2>&1 || true

    echo "UFW active rules:"
    ufw status | sed -n '1,40p' || true
}

post_install_network_optimization() {
    ensure_ufw_ports
    apply_low_latency_tuning
    if command -v tc >/dev/null 2>&1; then
        setup_fq_qdisc_service
    else
        echo -e "${YELLOW}tc command not found; skipping fq qdisc persistence setup.${NC}"
    fi
}

usage() {
    cat <<EOF
Usage:
  $0 [--upgrade-only|--upgrade] [--node <domain>] [--auth-url <url>] [--internal-service-token <token>] [--open-stunnel-5443] [--standalone]
  $0 --print-arch

Env (optional):
  AUTH_URL
  INTERNAL_SERVICE_TOKEN
  OPEN_STUNNEL_5443=true   # when co-locating postgresql.svc.plus on same node

Examples:
  # Supports AMD64 and ARM64 (aarch64)
  curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | \\
    bash -s -- --node hk-xhttp.svc.plus

  AUTH_URL=https://accounts-svc-plus-266500572462.asia-northeast1.run.app \\
  INTERNAL_SERVICE_TOKEN=xxxx \\
  curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | \\
    bash -s -- --node hk-xhttp.svc.plus

  # Upgrade binaries only (no config overwrite)
  curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | \\
    bash -s -- --upgrade-only

  # Open 5443/tcp together with 80/443/1443 for stunnel(PostgreSQL) co-location
  OPEN_STUNNEL_5443=true \\
  curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | \\
    bash -s -- --node jp-xhttp.svc.plus

  # Standalone self-host mode: installs caddy + xray only, generates UUID and prints import links
  curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | \\
    bash -s -- --node jp-xhttp.svc.plus --standalone

  # Print detected architecture and download artifacts (no install)
  curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | \\
    bash -s -- --print-arch
EOF
}

DOMAIN=""
AUTH_URL="${AUTH_URL:-}"
INTERNAL_SERVICE_TOKEN="${INTERNAL_SERVICE_TOKEN:-}"
UPGRADE_ONLY=false
PRINT_ARCH=false

while [ "$#" -gt 0 ]; do
    case "$1" in
        --node)
            DOMAIN="${2:-}"
            shift 2
            ;;
        --node=*)
            DOMAIN="${1#*=}"
            shift
            ;;
        --auth-url)
            AUTH_URL="${2:-}"
            shift 2
            ;;
        --auth-url=*)
            AUTH_URL="${1#*=}"
            shift
            ;;
        --internal-service-token)
            INTERNAL_SERVICE_TOKEN="${2:-}"
            shift 2
            ;;
        --internal-service-token=*)
            INTERNAL_SERVICE_TOKEN="${1#*=}"
            shift
            ;;
        --upgrade-only|--upgrade)
            UPGRADE_ONLY=true
            shift
            ;;
        --print-arch)
            PRINT_ARCH=true
            shift
            ;;
        --open-stunnel-5443)
            OPEN_STUNNEL_5443=true
            shift
            ;;
        --open-stunnel-5443=*)
            OPEN_STUNNEL_5443="${1#*=}"
            shift
            ;;
        --standalone)
            STANDALONE_MODE=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        --)
            shift
            break
            ;;
        *)
            if [ -z "$DOMAIN" ]; then
                DOMAIN="$1"
                shift
            else
                echo -e "${RED}Unknown argument: $1${NC}"
                usage
                exit 1
            fi
            ;;
    esac
done

if [ "$PRINT_ARCH" = false ]; then
    if [ -z "$DOMAIN" ]; then
        HOSTNAME=$(hostname)
        echo -e "${YELLOW}No node provided. Using system hostname: ${HOSTNAME}${NC}"
        DOMAIN="$HOSTNAME"
    fi

    if [ -z "$DOMAIN" ]; then
        echo -e "${RED}Node domain is required.${NC}"
        exit 1
    fi

    echo -e "${GREEN}Using node domain: ${DOMAIN}${NC}"
fi

if [ "$UPGRADE_ONLY" = true ]; then
    echo -e "${YELLOW}Running in upgrade-only mode: configuration files will not be overwritten.${NC}"
fi
if [ "$STANDALONE_MODE" = true ]; then
    echo -e "${GREEN}Running in standalone self-host mode (caddy + xray only).${NC}"
fi
if [ -n "$AUTH_URL" ]; then
    echo -e "${GREEN}Using AUTH_URL: ${AUTH_URL}${NC}"
fi
if is_truthy "$OPEN_STUNNEL_5443"; then
    OPEN_STUNNEL_5443=true
    echo -e "${GREEN}UFW will also allow 5443/tcp for stunnel co-location.${NC}"
else
    OPEN_STUNNEL_5443=false
fi

if [ "$PRINT_ARCH" = true ]; then
    ARCH_RAW="$(uname -m)"
    GOARCH=""
    case "$ARCH_RAW" in
        x86_64|amd64)
            GOARCH="amd64"
            ;;
        aarch64|arm64)
            GOARCH="arm64"
            ;;
        *)
            echo -e "${RED}Unsupported architecture: ${ARCH_RAW}${NC}"
            exit 1
            ;;
    esac

    GO_VER="1.23.4"
    GO_FILE="go${GO_VER}.linux-${GOARCH}.tar.gz"
    XCADDY_VER="0.4.5"
    XCADDY_DEB="xcaddy_${XCADDY_VER}_linux_${GOARCH}.deb"
    echo -e "${GREEN}Detected architecture: ${ARCH_RAW} (GOARCH=${GOARCH})${NC}"
    echo -e "${GREEN}Go artifact: ${GO_FILE}${NC}"
    echo -e "${GREEN}xcaddy artifact: ${XCADDY_DEB}${NC}"
    exit 0
fi

# 1. System Update & Dependencies
if [ "$UPGRADE_ONLY" = true ]; then
    echo -e "${YELLOW}[1/7] Upgrade mode: skipping apt dependency install.${NC}"
else
    echo -e "${GREEN}[1/7] Updating system and installing dependencies...${NC}"
    apt-get update && apt-get install -y curl wget git socat build-essential debian-keyring debian-archive-keyring apt-transport-https dnsutils
fi

# 2. Xray Installation
echo -e "${GREEN}[2/7] Installing Xray...${NC}"
bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install

# 3. Go & Caddy Installation
echo -e "${GREEN}[3/7] Installing Go and building Caddy with DNS + L4 plugins...${NC}"

# Detect architecture for Go/xcaddy downloads.
ARCH_RAW="$(uname -m)"
GOARCH=""
case "$ARCH_RAW" in
    x86_64|amd64)
        GOARCH="amd64"
        ;;
    aarch64|arm64)
        GOARCH="arm64"
        ;;
    *)
        echo -e "${RED}Unsupported architecture: ${ARCH_RAW}${NC}"
        exit 1
        ;;
esac
echo -e "${GREEN}Detected architecture: ${ARCH_RAW} (GOARCH=${GOARCH})${NC}"

# Install Go
if ! command -v go &> /dev/null; then
    GO_VER="1.23.4"
    GO_FILE="go${GO_VER}.linux-${GOARCH}.tar.gz"
    GO_URL="https://go.dev/dl/${GO_FILE}"
    LOCAL_TAR="/tmp/${GO_FILE}"
    echo -e "${GREEN}Go artifact: ${GO_FILE}${NC}"
    
    NEED_DOWNLOAD=true
    
    if [ -f "$LOCAL_TAR" ]; then
        echo "Found cached Go tarball at $LOCAL_TAR"
        echo "Verifying checksum..."
        # Fetch expected SHA256
        REMOTE_SHA256=$(curl -sL "${GO_URL}.sha256")
        if [ -n "$REMOTE_SHA256" ]; then
            LOCAL_SHA256=$(sha256sum "$LOCAL_TAR" | awk '{print $1}')
            if [ "$LOCAL_SHA256" == "$REMOTE_SHA256" ]; then
                echo -e "${GREEN}Checksum verified. Skipping download.${NC}"
                NEED_DOWNLOAD=false
            else
                echo -e "${YELLOW}Checksum mismatch (Local: $LOCAL_SHA256, Remote: $REMOTE_SHA256). Re-downloading...${NC}"
            fi
        else
            echo "Could not fetch remote checksum. Proceeding with existing file."
            NEED_DOWNLOAD=false
        fi
    fi

    if [ "$NEED_DOWNLOAD" = true ]; then
        wget "$GO_URL" -O "$LOCAL_TAR"
    fi
    
    rm -rf /usr/local/go && tar -C /usr/local -xzf "$LOCAL_TAR"
    export PATH=$PATH:/usr/local/go/bin
fi
go version

# Install xcaddy
if ! command -v xcaddy &> /dev/null; then
    XCADDY_VER="0.4.5"
    XCADDY_DEB="xcaddy_${XCADDY_VER}_linux_${GOARCH}.deb"
    XCADDY_URL="https://github.com/caddyserver/xcaddy/releases/download/v${XCADDY_VER}/${XCADDY_DEB}"
    echo -e "${GREEN}xcaddy artifact: ${XCADDY_DEB}${NC}"
    if wget "$XCADDY_URL" -O /tmp/xcaddy.deb; then
        dpkg -i /tmp/xcaddy.deb
    else
        echo -e "${YELLOW}xcaddy deb not available for ${GOARCH}. Falling back to 'go install'.${NC}"
        GOBIN="/usr/local/bin" go install "github.com/caddyserver/xcaddy/cmd/xcaddy@v${XCADDY_VER}"
    fi
fi

# Build Caddy
# Function to compare versions
version_lt() { test "$(echo "$@" | tr " " "\n" | sort -rV | head -n 1)" != "$1"; }

INSTALLED_CADDY_VER="0.0.0"
CADDY_HAS_L4=false
if command -v caddy &> /dev/null; then
    INSTALLED_CADDY_VER=$(caddy version | awk '{print $1}' | sed 's/v//')
    if caddy list-modules 2>/dev/null | grep -q '^layer4'; then
        CADDY_HAS_L4=true
    fi
fi

REQUIRED_VER="2.8.0"
echo "Installed Caddy Version: $INSTALLED_CADDY_VER"
echo "Caddy L4 Module Present: $CADDY_HAS_L4"

if [ "$UPGRADE_ONLY" = true ] || version_lt "$INSTALLED_CADDY_VER" "$REQUIRED_VER" || [ "$CADDY_HAS_L4" != true ]; then
    echo -e "${YELLOW}Building/Upgrading Caddy with required plugins...${NC}"
    xcaddy build \
        --with github.com/caddy-dns/cloudflare \
        --with github.com/caddy-dns/alidns \
        --with github.com/mholt/caddy-l4 \
        --output /usr/bin/caddy
else
    echo -e "${GREEN}Caddy is up to date (v$INSTALLED_CADDY_VER). Skipping build.${NC}"
fi

caddy version

# Ensure caddy runtime user exists (required by xray-tcp.service + file ownership)
ensure_caddy_user() {
    if id -u caddy >/dev/null 2>&1; then
        XRAY_TCP_USER="caddy"
        return 0
    fi

    echo -e "${YELLOW}caddy user not found, creating system user/group...${NC}"
    if ! getent group caddy >/dev/null 2>&1; then
        groupadd --system caddy || true
    fi

    USER_SHELL="/usr/sbin/nologin"
    if [ ! -x "$USER_SHELL" ]; then
        USER_SHELL="/usr/bin/false"
    fi

    useradd --system --gid caddy --create-home --home-dir /var/lib/caddy --shell "$USER_SHELL" caddy || true
    mkdir -p /var/lib/caddy

    if id -u caddy >/dev/null 2>&1; then
        chown -R caddy:caddy /var/lib/caddy || true
        XRAY_TCP_USER="caddy"
    else
        XRAY_TCP_USER="root"
        echo -e "${YELLOW}Failed to create caddy user. Falling back to root for xray-tcp service.${NC}"
    fi
}

ensure_caddy_user

generate_uuid() {
    if command -v uuidgen >/dev/null 2>&1; then
        uuidgen | tr '[:upper:]' '[:lower:]'
    else
        cat /proc/sys/kernel/random/uuid
    fi
}

ensure_standalone_uuid() {
    mkdir -p "$(dirname "$STANDALONE_UUID_FILE")"

    if [ -f "$STANDALONE_UUID_FILE" ]; then
        STANDALONE_UUID="$(tr -d '[:space:]' < "$STANDALONE_UUID_FILE")"
    fi

    if [ -z "${STANDALONE_UUID:-}" ]; then
        STANDALONE_UUID="$(generate_uuid)"
        printf '%s\n' "$STANDALONE_UUID" > "$STANDALONE_UUID_FILE"
        chmod 0644 "$STANDALONE_UUID_FILE"
    fi
}

render_xray_config_from_template() {
    local template_path="$1"
    local output_path="$2"
    local uuid_value="$3"

    sed "s|{{ UUID }}|${uuid_value}|g" "$template_path" > "$output_path"
}

print_standalone_links() {
    local node_name="${DOMAIN}"
    local xhttp_name="${node_name}-xhttp"
    local tcp_name="${node_name}-tcp"
    local xhttp_link
    local tcp_link

    xhttp_link="vless://${STANDALONE_UUID}@${DOMAIN}:443?encryption=none&security=tls&sni=${DOMAIN}&fp=chrome&type=xhttp&path=%2Fsplit#${xhttp_name}"
    tcp_link="vless://${STANDALONE_UUID}@${DOMAIN}:1443?encryption=none&flow=xtls-rprx-vision&security=tls&sni=${DOMAIN}&fp=chrome&type=tcp#${tcp_name}"

    echo ""
    echo -e "${GREEN}Standalone node import links:${NC}"
    echo "  XHTTP (recommended for OneXray/Xstream):"
    echo "  ${xhttp_link}"
    echo ""
    echo "  TCP Vision:"
    echo "  ${tcp_link}"
    echo ""
    echo "UUID:"
    echo "  ${STANDALONE_UUID}"
}

disable_agent_service_if_present() {
    if systemctl list-unit-files agent-svc-plus.service >/dev/null 2>&1; then
        systemctl disable --now agent-svc-plus >/dev/null 2>&1 || true
    fi
}

# 4. Configuration Directories
echo -e "${GREEN}[4/7] Setting up configuration directories...${NC}"
mkdir -p /usr/local/etc/xray
mkdir -p /etc/caddy
mkdir -p /etc/caddy/conf.d
if [ "$STANDALONE_MODE" != true ]; then
    mkdir -p /etc/agent
fi

# Co-location hygiene: remove known duplicated PostgreSQL caddy fragments
# generated by older scripts (e.g. postgresql-postgresql-*.caddy).
for stale in /etc/caddy/conf.d/postgresql-postgresql-*.caddy; do
    [ -e "$stale" ] || continue
    rm -f "$stale" || true
done

if [ "$STANDALONE_MODE" = true ]; then
    echo -e "${GREEN}[5/7] Preparing standalone Xray configuration...${NC}"
    mkdir -p /usr/local/etc/xray/templates
    cp config/*.template.json /usr/local/etc/xray/templates/
    ensure_standalone_uuid
    render_xray_config_from_template /usr/local/etc/xray/templates/xray.xhttp.template.json /usr/local/etc/xray/config.json "$STANDALONE_UUID"
    echo "Standalone UUID: ${STANDALONE_UUID}"
else
    # 5. Agent Installation
    echo -e "${GREEN}[5/7] Installing/Updating Agent Service...${NC}"

    AGENT_DIR="/opt/agent.svc.plus"

    if [ -d "$AGENT_DIR" ]; then
        echo "Updating existing agent repository..."
        cd "$AGENT_DIR"
        git fetch origin
        git reset --hard origin/main
    else
        git clone https://github.com/cloud-neutral-toolkit/agent.svc.plus.git "$AGENT_DIR"
        cd "$AGENT_DIR"
    fi

    # Build
    echo "Building Agent binary..."
    go mod tidy
    go build -o /usr/local/bin/agent-svc-plus ./cmd/agent
fi

if [ "$UPGRADE_ONLY" = true ]; then
    post_install_network_optimization

    echo -e "${GREEN}[upgrade-only] Restarting services to apply new binaries...${NC}"
    systemctl restart xray || true
    systemctl restart xray-tcp || true
    systemctl restart caddy || true
    if [ "$STANDALONE_MODE" != true ]; then
        systemctl restart agent-svc-plus || true
    fi

    echo -e "${GREEN}Upgrade Complete!${NC}"
    echo -e "Service states:"
    echo -e "  - xray: $(systemctl is-active xray || echo unknown)"
    echo -e "  - xray-tcp: $(systemctl is-active xray-tcp || echo unknown)"
    echo -e "  - caddy: $(systemctl is-active caddy || echo unknown)"
    if [ "$STANDALONE_MODE" != true ]; then
        echo -e "  - agent-svc-plus: $(systemctl is-active agent-svc-plus || echo unknown)"
    fi
    if [ "$STANDALONE_MODE" = true ]; then
        print_standalone_links
    fi
    exit 0
fi

# Always update templates
mkdir -p /usr/local/etc/xray/templates
cp config/*.template.json /usr/local/etc/xray/templates/
echo "Templates updated at /usr/local/etc/xray/templates/"

if [ "$STANDALONE_MODE" != true ]; then
    # Copy default config if not exists, but don't overwrite user config
    mkdir -p /etc/agent
    if [ ! -f /etc/agent/account-agent.yaml ]; then
        echo "Initializing new configuration file..."
        cp account-agent.yaml /etc/agent/account-agent.yaml
        # Initial path setup for templates in the new config
        sed -i 's|config/xray.xhttp.template.json|/usr/local/etc/xray/templates/xray.xhttp.template.json|g' /etc/agent/account-agent.yaml
        sed -i 's|config/xray.tcp.template.json|/usr/local/etc/xray/templates/xray.tcp.template.json|g' /etc/agent/account-agent.yaml
    else
        echo "Configuration file exists at /etc/agent/account-agent.yaml, skipping overwrite."
    fi

    # Apply runtime config from args/env (idempotent)
    sed -i -E "s|^([[:space:]]*id:[[:space:]]*).*$|\\1\"${DOMAIN}\"|g" /etc/agent/account-agent.yaml
    if [ -n "$AUTH_URL" ]; then
        sed -i -E "s|^([[:space:]]*controllerUrl:[[:space:]]*).*$|\\1\"${AUTH_URL}\"|g" /etc/agent/account-agent.yaml
    fi
    if [ -n "$INTERNAL_SERVICE_TOKEN" ]; then
        sed -i -E "s|^([[:space:]]*apiToken:[[:space:]]*).*$|\\1\"${INTERNAL_SERVICE_TOKEN}\"|g" /etc/agent/account-agent.yaml
    fi
fi

# 6. Caddy Configuration
echo -e "${GREEN}[6/7] Configuration Caddyfile...${NC}"

# Check for existing Certbot certificates to reuse
LE_CERT="/etc/letsencrypt/live/${DOMAIN}/fullchain.pem"
LE_KEY="/etc/letsencrypt/live/${DOMAIN}/privkey.pem"
CADDY_CERT_DIR="/var/lib/caddy/.local/share/caddy/certificates/acme-v02.api.letsencrypt.org-directory/${DOMAIN}"
TLS_CONFIG=""
XRAY_CERT="${CADDY_CERT_DIR}/${DOMAIN}.crt"
XRAY_KEY="${CADDY_CERT_DIR}/${DOMAIN}.key"

if [ -f "$LE_CERT" ] && [ -f "$LE_KEY" ]; then
    echo "Found existing Certbot certificates. usage: Direct."
    TLS_CONFIG="tls $LE_CERT $LE_KEY"
    XRAY_CERT="$LE_CERT"
    XRAY_KEY="$LE_KEY"
    
    # Fix Permissions
    echo "Adjusting permissions for /etc/letsencrypt to allow Xray access..."
    chmod 755 /etc/letsencrypt
    chmod 755 /etc/letsencrypt/live
    chmod 755 /etc/letsencrypt/archive
    chmod -R +r /etc/letsencrypt/archive/${DOMAIN}
    chmod -R +r /etc/letsencrypt/live/${DOMAIN}
else
    echo "No existing Certbot certificates found at $LE_CERT. Xray TCP will use Caddy-managed cert path: $XRAY_CERT"
fi

# Ensure Xray TCP template + config always match the chosen cert paths
sed -i -E "s|(\"certificateFile\"[[:space:]]*:[[:space:]]*\")[^\"]*(\")|\\1${XRAY_CERT}\\2|g" /usr/local/etc/xray/templates/xray.tcp.template.json
sed -i -E "s|(\"keyFile\"[[:space:]]*:[[:space:]]*\")[^\"]*(\")|\\1${XRAY_KEY}\\2|g" /usr/local/etc/xray/templates/xray.tcp.template.json
if [ "$STANDALONE_MODE" = true ]; then
    render_xray_config_from_template /usr/local/etc/xray/templates/xray.xhttp.template.json /usr/local/etc/xray/config.json "$STANDALONE_UUID"
    render_xray_config_from_template /usr/local/etc/xray/templates/xray.tcp.template.json /usr/local/etc/xray/tcp-config.json "$STANDALONE_UUID"
else
    cp /usr/local/etc/xray/templates/xray.tcp.template.json /usr/local/etc/xray/tcp-config.json
fi
chown "${XRAY_TCP_USER}:${XRAY_TCP_USER}" /usr/local/etc/xray/tcp-config.json || true
chmod 0644 /usr/local/etc/xray/tcp-config.json
echo "Updated Xray TCP template/config to use: ${XRAY_CERT}"

cat > /etc/caddy/Caddyfile <<EOF
${DOMAIN} {
    ${TLS_CONFIG}
    
    @grpc {
        path /split/*
    }

    handle @grpc {
        reverse_proxy unix//dev/shm/xray.sock {
             transport http {
                 versions h2c 2
             }
        }
    }

    # Fallback/Default site content
    respond "$( [ "$STANDALONE_MODE" = true ] && printf '%s' 'Standalone Xray Node' || printf '%s' 'Agent Service Plus Node' )"
}

import /etc/caddy/conf.d/*.caddy
EOF

# 7. Systemd Services
echo -e "${GREEN}[7/7] Installing Systemd Services...${NC}"

# Kill conflicting processes on 80/443
echo "Checking for port conflicts..."
if command -v fuser >/dev/null 2>&1; then
    fuser -k 80/tcp || true
    fuser -k 443/tcp || true
else
    echo -e "${YELLOW}fuser not found, skipping port pre-kill check.${NC}"
fi
# Stop legacy services if known
systemctl stop nginx || true
systemctl stop apache2 || true
systemctl stop caddy || true

# Permissions for config dir
mkdir -p /usr/local/etc/xray
chown -R root:root /usr/local/etc/xray
chmod -R a+rX /usr/local/etc/xray
# Legacy helper is no longer needed after switching to direct cert paths.
rm -f /usr/local/bin/sync-agent-certs

# Xray Service (XHTTP/Default)
cat > /etc/systemd/system/xray.service <<EOF
[Unit]
Description=Xray Service (XHTTP)
Documentation=https://github.com/xtls
After=network.target nss-lookup.target

[Service]
User=nobody
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ExecStart=/usr/local/bin/xray run -config /usr/local/etc/xray/config.json
Restart=on-failure
RestartPreventExitStatus=23
LimitNPROC=10000
LimitNOFILE=1000000
Environment=XRAY_LOCATION_ASSET=/usr/local/share/xray/

[Install]
WantedBy=multi-user.target
EOF

# Caddy Service (fallback when distro package service is absent)
if [ ! -f /etc/systemd/system/caddy.service ] && [ ! -f /lib/systemd/system/caddy.service ] && [ ! -f /usr/lib/systemd/system/caddy.service ]; then
cat > /etc/systemd/system/caddy.service <<EOF
[Unit]
Description=Caddy
Documentation=https://caddyserver.com/docs/
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${XRAY_TCP_USER}
Group=${XRAY_TCP_USER}
ExecStart=/usr/bin/caddy run --environ --config /etc/caddy/Caddyfile
ExecReload=/usr/bin/caddy reload --config /etc/caddy/Caddyfile
TimeoutStopSec=5s
LimitNOFILE=1048576
PrivateTmp=true
ProtectSystem=full
AmbientCapabilities=CAP_NET_BIND_SERVICE
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF
fi

# Xray TCP Service
cat > /etc/systemd/system/xray-tcp.service <<EOF
[Unit]
Description=Xray Service (TCP)
Documentation=https://github.com/xtls
After=network.target nss-lookup.target

[Service]
User=${XRAY_TCP_USER}
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ExecStart=/usr/local/bin/xray run -config /usr/local/etc/xray/tcp-config.json
Restart=on-failure
RestartPreventExitStatus=23
LimitNPROC=10000
LimitNOFILE=1000000
Environment=XRAY_LOCATION_ASSET=/usr/local/share/xray/

[Install]
WantedBy=multi-user.target
EOF

if [ "$STANDALONE_MODE" != true ]; then
# Agent Service
cat > /etc/systemd/system/agent-svc-plus.service <<EOF
[Unit]
Description=Agent Service Plus
After=network.target

[Service]
ExecStart=/usr/local/bin/agent-svc-plus -config /etc/agent/account-agent.yaml
Restart=always
User=root
WorkingDirectory=/opt/agent.svc.plus

[Install]
WantedBy=multi-user.target
EOF
fi

systemctl daemon-reload
systemctl enable xray
systemctl enable xray-tcp
systemctl enable caddy || true
if [ "$STANDALONE_MODE" != true ]; then
    systemctl enable agent-svc-plus
fi
systemctl restart xray || true
systemctl restart caddy || true

if [ ! -f "$LE_CERT" ] || [ ! -f "$LE_KEY" ]; then
    for i in $(seq 1 30); do
        if [ -f "$XRAY_CERT" ] && [ -f "$XRAY_KEY" ]; then
            break
        fi
        sleep 2
    done
fi
systemctl restart xray-tcp || true

if [ "$STANDALONE_MODE" = true ]; then
    disable_agent_service_if_present
    echo -e "${GREEN}Standalone mode: skipping agent-svc-plus service installation.${NC}"
elif [ -n "$AUTH_URL" ] && [ -n "$INTERNAL_SERVICE_TOKEN" ]; then
    systemctl restart agent-svc-plus
else
    echo -e "${YELLOW}Skipping agent-svc-plus start: AUTH_URL or INTERNAL_SERVICE_TOKEN is missing.${NC}"
fi

post_install_network_optimization

echo -e "${GREEN}Installation Complete!${NC}"
if [ "$STANDALONE_MODE" = true ]; then
    echo -e "Standalone config:"
    echo -e "  - xray xhttp: /usr/local/etc/xray/config.json"
    echo -e "  - xray tcp: /usr/local/etc/xray/tcp-config.json"
    echo -e "  - uuid: ${STANDALONE_UUID}"
    print_standalone_links
else
    echo -e "Config file: /etc/agent/account-agent.yaml"
    echo -e "  - agent.id: ${DOMAIN}"
    echo -e "  - controllerUrl: ${AUTH_URL:-<not set>}"
    if [ -n "$INTERNAL_SERVICE_TOKEN" ]; then
        echo -e "  - apiToken: <provided>"
    else
        echo -e "  - apiToken: <not set>"
    fi
    if [ -z "$AUTH_URL" ] || [ -z "$INTERNAL_SERVICE_TOKEN" ]; then
        echo -e "Set AUTH_URL and INTERNAL_SERVICE_TOKEN then run: systemctl restart agent-svc-plus"
    fi
fi
