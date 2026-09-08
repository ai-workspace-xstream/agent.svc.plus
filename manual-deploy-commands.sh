#!/bin/bash
# xconnect-edge-agent 手动部署命令
# 复制以下命令到目标主机执行

# ========== 配置 ==========
HOST="5.78.45.49"
USER="root"
AGENT_ID="us-xhttp.svc.plus"
CONTROLLER_URL="https://accounts-svc-plus-266500572462.asia-northeast1.run.app"
API_TOKEN="${INTERNAL_SERVICE_TOKEN:?export INTERNAL_SERVICE_TOKEN before generating the remote script}"

echo "=========================================="
echo "xconnect-edge-agent 手动部署命令"
echo "=========================================="
echo ""

# ===== 步骤 1: 传输文件 =====
echo "# ===== 步骤 1: 传输文件到目标主机 ====="
echo "# 从本地执行:"
echo "scp xconnect-edge-agent ${USER}@${HOST}:/tmp/"
echo "scp config/xray.xhttp.template.json ${USER}@${HOST}:/tmp/"
echo "scp config/xray.tcp.template.json ${USER}@${HOST}:/tmp/"
echo ""

# ===== 步骤 2: SSH 登录并安装 =====
echo "# ===== 步骤 2: SSH 登录并安装 ====="
echo "# 执行以下命令:"
echo "ssh ${USER}@${HOST} << 'REMOTE_SCRIPT'"
cat << 'EOF'
# 创建目录
mkdir -p /usr/local/etc/xconnect-edge-agent
mkdir -p /usr/local/etc/xray/templates
mkdir -p /var/log/xconnect-edge-agent
mkdir -p /var/lib/xconnect-edge-agent

# 安装二进制
mv /tmp/xconnect-edge-agent /usr/local/bin/
chmod 755 /usr/local/bin/xconnect-edge-agent

# 复制配置模板
mv /tmp/xray.xhttp.template.json /usr/local/etc/xray/templates/
mv /tmp/xray.tcp.template.json /usr/local/etc/xray/templates/

# 创建配置文件
cat > /usr/local/etc/xconnect-edge-agent/config.yaml << 'CONFIG'
mode: "agent"

log:
  level: info

agent:
  id: "us-xhttp.svc.plus"
  controllerUrl: "https://accounts-svc-plus-266500572462.asia-northeast1.run.app"
  apiToken: "${INTERNAL_SERVICE_TOKEN}"
  httpTimeout: 15s
  statusInterval: 1m
  syncInterval: 5m
  tls:
    insecureSkipVerify: false

xray:
  sync:
    enabled: true
    interval: 5m
    targets:
      - name: "xhttp"
        outputPath: "/usr/local/etc/xray/config.json"
        templatePath: "/usr/local/etc/xray/templates/xray.xhttp.template.json"
        validateCommand: []
        restartCommand: []
      - name: "tcp"
        outputPath: "/usr/local/etc/xray/tcp-config.json"
        templatePath: "/usr/local/etc/xray/templates/xray.tcp.template.json"
        validateCommand: []
        restartCommand: []
CONFIG

chmod 640 /usr/local/etc/xconnect-edge-agent/config.yaml

# 创建 systemd 服务
cat > /etc/systemd/system/xconnect-edge-agent.service << 'SERVICE'
[Unit]
Description=XConnect Edge Agent - us-xhttp.svc.plus
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/xconnect-edge-agent --config /usr/local/etc/xconnect-edge-agent/config.yaml
WorkingDirectory=/var/lib/xconnect-edge-agent
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
Environment=HOME=/root
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/usr/local/etc/xconnect-edge-agent /var/log/xconnect-edge-agent /usr/local/etc/xray

[Install]
WantedBy=multi-user.target
SERVICE

# 启动服务
systemctl daemon-reload
systemctl enable xconnect-edge-agent
systemctl start xconnect-edge-agent

# 验证
echo ""
echo "=== 服务状态 ==="
systemctl status xconnect-edge-agent --no-pager

echo ""
echo "=== 最近日志 ==="
journalctl -u xconnect-edge-agent -n 20 --no-pager
EOF
echo "REMOTE_SCRIPT"
echo ""

# ===== 步骤 3: 验证 =====
echo "# ===== 步骤 3: 验证 ====="
echo "ssh ${USER}@${HOST} 'systemctl status xconnect-edge-agent'"
echo "ssh ${USER}@${HOST} 'journalctl -u xconnect-edge-agent -n 50'"
