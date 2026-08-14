#!/bin/bash
# 一键部署并验证
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$SCRIPT_DIR/common.sh"

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

if [ -z "$HOST_ALIAS" ] || [ -z "$HOST" ]; then
    echo "错误: inventory 未配置 agent_svc_plus 主机: $INVENTORY_FILE"
    echo "请先填写 inventory 主机，或显式使用 --prod / --inventory。"
    exit 1
fi

echo "=========================================="
echo "agent-svc-plus 一键部署和验证"
echo "=========================================="
echo ""
echo "目标环境: $TARGET_ENVIRONMENT"
echo "Inventory: $INVENTORY_FILE"
echo "Vars: $AGENT_VARS_FILE"
echo "目标主机: $HOST_ALIAS ($HOST)"
echo "前提: 目标主机必须已通过 scripts/setup-proxy.sh 完成 xray/xray-tcp/caddy 预配置"
echo ""

# 步骤 1: 更新 DNS
echo "=== 步骤 1/3: 更新 DNS ==="
cd "$SCRIPT_DIR"
export ANSIBLE_CONFIG="$SCRIPT_DIR/ansible.cfg"
export AGENT_VARS_FILE

ansible-playbook -i "$INVENTORY_FILE" playbooks/update_cloudflare_dns.yml -v || {
    echo "DNS 更新失败，继续部署..."
}
echo ""

# 步骤 2: 部署 Agent
echo "=== 步骤 2/3: 部署 Agent ==="
ansible-playbook -i "$INVENTORY_FILE" playbooks/deploy_agent_svc_plus.yml -v || {
    echo "部署失败!"
    exit 1
}
echo ""

# 步骤 3: 验证
echo "=== 步骤 3/3: 验证部署 ==="
./verify.sh --inventory "$INVENTORY_FILE"
