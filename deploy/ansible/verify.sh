#!/bin/bash
# agent-svc-plus 自动验证脚本
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$SCRIPT_DIR/common.sh"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 加载环境变量
source "$PROJECT_ROOT/.env" 2>/dev/null || true

INVENTORY_FILE="$SCRIPT_DIR/inventory.ini"
AGENT_VARS_FILE="$SCRIPT_DIR/vars/agent_svc_plus.yml"
TARGET_ENVIRONMENT="inventory.ini"

while [[ $# -gt 0 ]]; do
    case $1 in
        --prod)
            INVENTORY_FILE="$SCRIPT_DIR/inventory.prod.ini"
            AGENT_VARS_FILE="$SCRIPT_DIR/vars/agent_svc_plus.prod.yml"
            TARGET_ENVIRONMENT="production"
            shift
            ;;
        --inventory)
            if [ $# -lt 2 ]; then
                echo "错误: --inventory 需要一个文件路径"
                exit 1
            fi
            INVENTORY_FILE="$(resolve_inventory_path "$2")"
            TARGET_ENVIRONMENT="custom"
            shift 2
            ;;
        *)
            echo "未知参数: $1"
            echo "使用方法: $0 [--prod|--inventory FILE]"
            exit 1
            ;;
    esac
done

if [ ! -f "$INVENTORY_FILE" ]; then
    echo "错误: inventory 文件不存在: $INVENTORY_FILE"
    exit 1
fi

if [ ! -f "$AGENT_VARS_FILE" ]; then
    echo "错误: vars 文件不存在: $AGENT_VARS_FILE"
    exit 1
fi

HOST_ALIAS="$(inventory_first_host_alias "$INVENTORY_FILE" || true)"
HOST="$(inventory_target_host "$INVENTORY_FILE" || true)"
USER="$(inventory_target_user "$INVENTORY_FILE" || true)"
PORT="$(inventory_target_port "$INVENTORY_FILE" || true)"
EXPECTED_AGENT_ID="$(vars_agent_id "$AGENT_VARS_FILE" || true)"
EXPECTED_DNS_IP="$(vars_cloudflare_record_ip "$AGENT_VARS_FILE" || true)"

if [ -z "$HOST_ALIAS" ] || [ -z "$HOST" ]; then
    echo "错误: inventory 未配置 agent_svc_plus 主机: $INVENTORY_FILE"
    echo "请先填写 inventory 主机，或显式使用 --prod / --inventory。"
    exit 1
fi

echo "=========================================="
echo "agent-svc-plus 自动验证"
echo "=========================================="
echo ""
echo "目标环境: $TARGET_ENVIRONMENT"
echo "Inventory: $INVENTORY_FILE"
echo "Vars: $AGENT_VARS_FILE"
echo "目标主机: $HOST_ALIAS ($HOST:$PORT)"
echo "预期模型: 目标主机已通过 scripts/setup-proxy.sh 完成 xray/xray-tcp/caddy 预配置，Ansible 仅覆盖 agent 与模板。"
echo ""

# 检查函数
check_pass() {
    echo -e "${GREEN}✓ $1${NC}"
}

check_fail() {
    echo -e "${RED}✗ $1${NC}"
    FAILED=1
}

check_warn() {
    echo -e "${YELLOW}! $1${NC}"
}

FAILED=0

# ========== DNS 验证 ==========
echo "========== 1. DNS 验证 =========="
DNS_IP=$(dig +short "$HOST_ALIAS" @8.8.8.8 2>/dev/null || echo "")
if [ -n "$EXPECTED_DNS_IP" ] && [ "$DNS_IP" == "$EXPECTED_DNS_IP" ]; then
    check_pass "DNS 解析正确: $HOST_ALIAS -> $DNS_IP"
else
    if [ -z "$DNS_IP" ]; then
        check_warn "DNS 解析失败 (网络受限或记录未配置)"
        echo "  请手动执行: dig $HOST_ALIAS +short"
        if [ -n "$EXPECTED_DNS_IP" ]; then
            echo "  预期结果: $EXPECTED_DNS_IP"
        fi
    else
        if [ -n "$EXPECTED_DNS_IP" ]; then
            check_fail "DNS 解析错误: $HOST_ALIAS -> $DNS_IP (预期: $EXPECTED_DNS_IP)"
        else
            check_warn "DNS 解析返回: $HOST_ALIAS -> $DNS_IP"
        fi
    fi
fi
echo ""

# ========== 网络验证 ==========
echo "========== 2. 网络验证 =========="
if nc -zv -w5 "$HOST" "$PORT" 2>&1 | grep -q "succeeded\|open"; then
    check_pass "SSH 端口 ($PORT) 可达"
else
    check_warn "SSH 端口 ($PORT) 不可达 (网络受限)"
    echo "  请在可访问网络的环境中执行验证"
fi
echo ""

# ========== SSH 验证 ==========
echo "========== 3. SSH 验证 =========="
SSH_TEST=$(ssh -p "$PORT" -o ConnectTimeout=10 -o StrictHostKeyChecking=no -o BatchMode=yes "$USER@$HOST" "echo 'SSH_OK'" 2>&1 || echo "SSH_FAIL")
if [[ "$SSH_TEST" == *"SSH_OK"* ]]; then
    check_pass "SSH 连接成功"
    SSH_OK=1
else
    check_fail "SSH 连接失败: $(echo "$SSH_TEST" | head -1)"
    echo "  请检查网络或配置跳板机/VPN"
    SSH_OK=0
fi
echo ""

# ========== 服务验证 ==========
if [ "$SSH_OK" == "1" ]; then
    echo "========== 4. 服务验证 =========="
    
    # 检查服务状态
    SERVICE_STATUS=$(ssh -p "$PORT" "$USER@$HOST" "systemctl is-active agent-svc-plus" 2>/dev/null || echo "unknown")
    if [ "$SERVICE_STATUS" == "active" ]; then
        check_pass "服务状态: active (running)"
    else
        check_fail "服务状态: $SERVICE_STATUS"
    fi
    
    # 检查服务是否开机自启
    ENABLED=$(ssh -p "$PORT" "$USER@$HOST" "systemctl is-enabled agent-svc-plus" 2>/dev/null || echo "no")
    if [ "$ENABLED" == "enabled" ]; then
        check_pass "开机自启: enabled"
    else
        check_warn "开机自启: $ENABLED"
    fi
    
    # 检查二进制文件
    BINARY_EXISTS=$(ssh -p "$PORT" "$USER@$HOST" "test -f /usr/local/bin/agent-svc-plus && echo 'yes' || echo 'no'" 2>/dev/null || echo "no")
    if [ "$BINARY_EXISTS" == "yes" ]; then
        check_pass "二进制文件: /usr/local/bin/agent-svc-plus 存在"
    else
        check_fail "二进制文件: 不存在"
    fi
    
    # 检查配置文件
    CONFIG_EXISTS=$(ssh -p "$PORT" "$USER@$HOST" "test -f /etc/agent/account-agent.yaml && echo 'yes' || echo 'no'" 2>/dev/null || echo "no")
    if [ "$CONFIG_EXISTS" == "yes" ]; then
        check_pass "配置文件: /etc/agent/account-agent.yaml 存在"
    else
        check_fail "配置文件: 不存在"
    fi

    # 检查 systemd ExecStart 是否匹配 setup-proxy.sh 模型
    EXECSTART_OK=$(ssh -p "$PORT" "$USER@$HOST" "systemctl cat agent-svc-plus | grep -F -- '-config /etc/agent/account-agent.yaml' >/dev/null && echo 'yes' || echo 'no'" 2>/dev/null || echo "no")
    if [ "$EXECSTART_OK" == "yes" ]; then
        check_pass "systemd ExecStart 使用 /etc/agent/account-agent.yaml"
    else
        check_fail "systemd ExecStart 未使用 /etc/agent/account-agent.yaml"
    fi

    # 检查 Agent ID
    REMOTE_AGENT_ID=$(ssh -p "$PORT" "$USER@$HOST" "awk -F'\"' '/^[[:space:]]*id:/{print \$2; exit}' /etc/agent/account-agent.yaml" 2>/dev/null || echo "")
    if [ -n "$EXPECTED_AGENT_ID" ] && [ "$REMOTE_AGENT_ID" == "$EXPECTED_AGENT_ID" ]; then
        check_pass "Agent ID 匹配: $REMOTE_AGENT_ID"
    elif [ -n "$REMOTE_AGENT_ID" ]; then
        check_fail "Agent ID 不匹配: $REMOTE_AGENT_ID (预期: $EXPECTED_AGENT_ID)"
    else
        check_fail "无法读取 Agent ID"
    fi
    
    # 检查进程
    PROCESS_RUNNING=$(ssh -p "$PORT" "$USER@$HOST" "pgrep -f 'agent-svc-plus' || echo 'no'" 2>/dev/null || echo "no")
    if [ "$PROCESS_RUNNING" != "no" ]; then
        check_pass "进程运行中: PID $PROCESS_RUNNING"
    else
        check_fail "进程未运行"
    fi

    # 检查 setup-proxy.sh 预配置目录
    CADDYFILE_EXISTS=$(ssh -p "$PORT" "$USER@$HOST" "test -f /etc/caddy/Caddyfile && echo 'yes' || echo 'no'" 2>/dev/null || echo "no")
    if [ "$CADDYFILE_EXISTS" == "yes" ]; then
        check_pass "Caddy 配置: /etc/caddy/Caddyfile 存在"
    else
        check_fail "Caddy 配置: /etc/caddy/Caddyfile 不存在"
    fi

    TEMPLATE_DIR_EXISTS=$(ssh -p "$PORT" "$USER@$HOST" "test -d /usr/local/etc/xray/templates && echo 'yes' || echo 'no'" 2>/dev/null || echo "no")
    if [ "$TEMPLATE_DIR_EXISTS" == "yes" ]; then
        check_pass "Xray 模板目录: /usr/local/etc/xray/templates 存在"
    else
        check_fail "Xray 模板目录: /usr/local/etc/xray/templates 不存在"
    fi

    # 检查 Xray / Caddy 服务
    XHTTP_STATUS=$(ssh -p "$PORT" "$USER@$HOST" "systemctl is-active xray.service" 2>/dev/null || echo "unknown")
    if [ "$XHTTP_STATUS" == "active" ]; then
        check_pass "xray.service 状态: active"
    else
        check_fail "xray.service 状态: $XHTTP_STATUS"
    fi

    TCP_STATUS=$(ssh -p "$PORT" "$USER@$HOST" "systemctl is-active xray-tcp.service" 2>/dev/null || echo "unknown")
    if [ "$TCP_STATUS" == "active" ]; then
        check_pass "xray-tcp.service 状态: active"
    else
        check_fail "xray-tcp.service 状态: $TCP_STATUS"
    fi

    CADDY_STATUS=$(ssh -p "$PORT" "$USER@$HOST" "systemctl is-active caddy.service" 2>/dev/null || echo "unknown")
    if [ "$CADDY_STATUS" == "active" ]; then
        check_pass "caddy.service 状态: active"
    else
        check_fail "caddy.service 状态: $CADDY_STATUS"
    fi

    CADDY_ENABLED=$(ssh -p "$PORT" "$USER@$HOST" "systemctl is-enabled caddy.service" 2>/dev/null || echo "unknown")
    if [ "$CADDY_ENABLED" == "enabled" ]; then
        check_pass "caddy.service 开机自启: enabled"
    else
        check_warn "caddy.service 开机自启: $CADDY_ENABLED"
    fi
    
    echo ""
    echo "========== 5. 日志验证 =========="
    echo "最近 10 行日志:"
    ssh -p "$PORT" "$USER@$HOST" "journalctl -u agent-svc-plus -n 10 --no-pager" 2>/dev/null || echo "无法获取日志"
    echo ""
    
    echo "========== 6. 连接验证 =========="
    # 检查与控制器的连接
    CONTROLLER_URL=$(awk -F'"' '/^[[:space:]]*agent_controller_url:/{print $2; exit}' "$AGENT_VARS_FILE")
    echo "控制器 URL: $CONTROLLER_URL"
    
    # 尝试从目标主机测试控制器连接
    CONTROLLER_TEST=$(ssh -p "$PORT" "$USER@$HOST" "curl -s -o /dev/null -w '%{http_code}' --connect-timeout 5 $CONTROLLER_URL/healthz 2>/dev/null || echo '000'" 2>/dev/null || echo "000")
    if [ "$CONTROLLER_TEST" == "200" ] || [ "$CONTROLLER_TEST" == "404" ]; then
        check_pass "控制器连接正常 (HTTP $CONTROLLER_TEST)"
    else
        check_warn "控制器连接: HTTP $CONTROLLER_TEST"
    fi
else
    echo "========== 跳过远程验证 =========="
    echo "SSH 连接不可用，跳过远程验证"
    echo ""
    echo "请在可访问目标主机的环境中执行验证:"
    echo "  ssh -p $PORT $USER@$HOST 'systemctl status agent-svc-plus'"
fi

echo ""
echo "=========================================="
if [ "$FAILED" == "1" ]; then
    echo -e "${RED}验证失败${NC}"
    exit 1
else
    echo -e "${GREEN}验证通过${NC}"
    exit 0
fi
