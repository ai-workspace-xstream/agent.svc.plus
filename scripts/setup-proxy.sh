#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}Starting Agent Service Plus Installation...${NC}"

usage() {
    cat <<EOF
Usage:
  $0 [--upgrade-only|--upgrade] [--node <domain>] [--auth-url <url>] [--internal-service-token <token>]

Env (optional):
  AUTH_URL
  INTERNAL_SERVICE_TOKEN

Examples:
  curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | \\
    bash -s -- --node hk-xhttp.svc.plus

  AUTH_URL=https://accounts-svc-plus-266500572462.asia-northeast1.run.app \\
  INTERNAL_SERVICE_TOKEN=xxxx \\
  curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | \\
    bash -s -- --node hk-xhttp.svc.plus

  # Upgrade binaries only (no config overwrite)
  curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | \\
    bash -s -- --upgrade-only
EOF
}

DOMAIN=""
AUTH_URL="${AUTH_URL:-}"
INTERNAL_SERVICE_TOKEN="${INTERNAL_SERVICE_TOKEN:-}"
UPGRADE_ONLY=false

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
if [ "$UPGRADE_ONLY" = true ]; then
    echo -e "${YELLOW}Running in upgrade-only mode: configuration files will not be overwritten.${NC}"
fi
if [ -n "$AUTH_URL" ]; then
    echo -e "${GREEN}Using AUTH_URL: ${AUTH_URL}${NC}"
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

# Install Go
if ! command -v go &> /dev/null; then
    GO_VER="1.23.4"
    GO_FILE="go${GO_VER}.linux-amd64.tar.gz"
    GO_URL="https://go.dev/dl/${GO_FILE}"
    LOCAL_TAR="/tmp/${GO_FILE}"
    
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
    wget https://github.com/caddyserver/xcaddy/releases/download/v0.4.5/xcaddy_0.4.5_linux_amd64.deb -O /tmp/xcaddy.deb
    dpkg -i /tmp/xcaddy.deb
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

# 4. Configuration Directories
echo -e "${GREEN}[4/7] Setting up configuration directories...${NC}"
mkdir -p /usr/local/etc/xray
mkdir -p /etc/caddy
mkdir -p /etc/agent

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

if [ "$UPGRADE_ONLY" = true ]; then
    echo -e "${GREEN}[upgrade-only] Restarting services to apply new binaries...${NC}"
    systemctl restart xray || true
    systemctl restart xray-tcp || true
    systemctl restart caddy || true
    systemctl restart agent-svc-plus || true

    echo -e "${GREEN}Upgrade Complete!${NC}"
    echo -e "Service states:"
    echo -e "  - xray: $(systemctl is-active xray || echo unknown)"
    echo -e "  - xray-tcp: $(systemctl is-active xray-tcp || echo unknown)"
    echo -e "  - caddy: $(systemctl is-active caddy || echo unknown)"
    echo -e "  - agent-svc-plus: $(systemctl is-active agent-svc-plus || echo unknown)"
    exit 0
fi

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

# Always update templates
mkdir -p /usr/local/etc/xray/templates
cp config/*.template.json /usr/local/etc/xray/templates/
echo "Templates updated at /usr/local/etc/xray/templates/"

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
cp /usr/local/etc/xray/templates/xray.tcp.template.json /usr/local/etc/xray/tcp-config.json
chown caddy:caddy /usr/local/etc/xray/tcp-config.json
chmod 0644 /usr/local/etc/xray/tcp-config.json
echo "Updated Xray TCP template/config to use: ${XRAY_CERT}"

cat > /etc/caddy/Caddyfile <<EOF
${DOMAIN}:443 {
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
    respond "Agent Service Plus Node"
}
EOF

# 7. Systemd Services
echo -e "${GREEN}[7/7] Installing Systemd Services...${NC}"

# Kill conflicting processes on 80/443
echo "Checking for port conflicts..."
fuser -k 80/tcp || true
fuser -k 443/tcp || true
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

# Xray TCP Service
cat > /etc/systemd/system/xray-tcp.service <<EOF
[Unit]
Description=Xray Service (TCP)
Documentation=https://github.com/xtls
After=network.target nss-lookup.target

[Service]
User=caddy
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

systemctl daemon-reload
systemctl enable xray
systemctl enable xray-tcp
systemctl enable caddy
systemctl enable agent-svc-plus
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

if [ -n "$AUTH_URL" ] && [ -n "$INTERNAL_SERVICE_TOKEN" ]; then
    systemctl restart agent-svc-plus
else
    echo -e "${YELLOW}Skipping agent-svc-plus start: AUTH_URL or INTERNAL_SERVICE_TOKEN is missing.${NC}"
fi

echo -e "${GREEN}Installation Complete!${NC}"
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
