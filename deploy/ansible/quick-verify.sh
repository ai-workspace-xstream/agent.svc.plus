#!/bin/bash
# 快速验证脚本 - 用于目标主机上执行
set -e

echo "=== agent-svc-plus 快速验证 ==="
echo ""

# 1. 检查服务状态
echo "1. 服务状态:"
systemctl is-active agent-svc-plus && echo "   ✓ 服务运行中" || echo "   ✗ 服务未运行"

# 2. 检查配置文件
echo ""
echo "2. 配置文件:"
test -f /etc/agent/account-agent.yaml && echo "   ✓ /etc/agent/account-agent.yaml 存在" || echo "   ✗ /etc/agent/account-agent.yaml 不存在"
test -f /usr/local/bin/agent-svc-plus && echo "   ✓ agent-svc-plus 存在" || echo "   ✗ agent-svc-plus 不存在"
systemctl cat agent-svc-plus | grep -F -- "-config /etc/agent/account-agent.yaml" >/dev/null && echo "   ✓ systemd ExecStart 正确" || echo "   ✗ systemd ExecStart 不匹配"

# 3. 检查依赖服务
echo ""
echo "3. 依赖服务:"
systemctl is-active xray.service >/dev/null 2>&1 && echo "   ✓ xray.service 运行中" || echo "   ✗ xray.service 未运行"
systemctl is-active xray-tcp.service >/dev/null 2>&1 && echo "   ✓ xray-tcp.service 运行中" || echo "   ✗ xray-tcp.service 未运行"
systemctl is-active caddy.service >/dev/null 2>&1 && echo "   ✓ caddy.service 运行中" || echo "   ✗ caddy.service 未运行"
test -f /etc/caddy/Caddyfile && echo "   ✓ /etc/caddy/Caddyfile 存在" || echo "   ✗ /etc/caddy/Caddyfile 不存在"
test -d /usr/local/etc/xray/templates && echo "   ✓ /usr/local/etc/xray/templates 存在" || echo "   ✗ /usr/local/etc/xray/templates 不存在"

# 4. 显示最近日志
echo ""
echo "4. 最近日志:"
journalctl -u agent-svc-plus -n 5 --no-pager 2>/dev/null || echo "   无法获取日志"

# 5. 检查 Agent ID
echo ""
echo "5. Agent 配置:"
grep "id:" /etc/agent/account-agent.yaml 2>/dev/null | head -1 || echo "   无法读取配置"

echo ""
echo "=== 验证完成 ==="
