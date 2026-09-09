#!/bin/bash
# xconnect-edge-agent 完整部署脚本
# 在有网络的环境中执行此脚本

set -e
: "${INTERNAL_SERVICE_TOKEN:?export INTERNAL_SERVICE_TOKEN before running this script}"

echo "=========================================="
echo "xconnect-edge-agent 完整部署"
echo "=========================================="
echo ""
echo "目标主机: 5.78.45.49 (us-xhttp.svc.plus)"
echo "当前目录: $(pwd)"
echo ""

# 检查文件
echo "=== 检查本地文件 ==="
for f in xconnect-edge-agent config/xray.xhttp.template.json config/xray.tcp.template.json; do
    if [ -f "$f" ]; then
        echo "✓ $f 存在"
    else
        echo "✗ $f 不存在"
        echo "请在项目根目录执行此脚本"
        exit 1
    fi
done
echo ""

# 传输文件
echo "=== 步骤 1/3: 传输文件 ==="
scp xconnect-edge-agent root@5.78.45.49:/tmp/ && echo "✓ 二进制传输成功"
scp config/xray.xhttp.template.json root@5.78.45.49:/tmp/ && echo "✓ XHTTP 模板传输成功"
scp config/xray.tcp.template.json root@5.78.45.49:/tmp/ && echo "✓ TCP 模板传输成功"
echo ""

# 执行远程安装
echo "=== 步骤 2/3: 远程安装 ==="
ssh root@5.78.45.49 'bash -s' << 'REMOTE_SCRIPT'
set -e

echo "创建目录..."
mkdir -p /usr/local/etc/xconnect-edge-agent
mkdir -p /usr/local/etc/xray/templates
mkdir -p /var/log/xconnect-edge-agent
mkdir -p /var/lib/xconnect-edge-agent

echo "安装二进制..."
mv /tmp/xconnect-edge-agent /usr/local/bin/
chmod 755 /usr/local/bin/xconnect-edge-agent

echo "复制配置模板..."
mv /tmp/xray.xhttp.template.json /usr/local/etc/xray/templates/
mv /tmp/xray.tcp.template.json /usr/local/etc/xray/templates/

echo "创建配置文件..."
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

echo "创建 systemd 服务..."
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

echo "启动服务..."
systemctl daemon-reload
systemctl enable xconnect-edge-agent
systemctl start xconnect-edge-agent

echo ""
echo "=== 服务状态 ==="
systemctl status xconnect-edge-agent --no-pager
REMOTE_SCRIPT
echo ""

# 验证
echo "=== 步骤 3/3: 验证 ==="
ssh root@5.78.45.49 'systemctl is-active xconnect-edge-agent && echo "✓ 服务运行中" || echo "✗ 服务未运行"'
ssh root@5.78.45.49 'journalctl -u xconnect-edge-agent -n 10 --no-pager'
echo ""

echo "=========================================="
echo "部署完成!"
echo "=========================================="
echo ""
echo "后续命令:"
echo "  查看状态: ssh root@5.78.45.49 'systemctl status xconnect-edge-agent'"
echo "  查看日志: ssh root@5.78.45.49 'journalctl -u xconnect-edge-agent -f'"
