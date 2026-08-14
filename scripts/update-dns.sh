#!/bin/bash
# DNS 更新脚本 - 创建 us-xhttp.svc.plus DNS 记录

set -e

# 加载环境变量
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../.env" 2>/dev/null || true

ZONE_NAME="svc.plus"
RECORD_NAME="us-xhttp"
RECORD_TYPE="A"
RECORD_CONTENT="5.78.45.49"
RECORD_TTL=300
RECORD_PROXIED=false

echo "=== 创建/更新 DNS 记录 ==="
echo "Zone: $ZONE_NAME"
echo "Record: $RECORD_NAME.$ZONE_NAME"
echo "Type: $RECORD_TYPE"
echo "Content: $RECORD_CONTENT"
echo ""

# 获取 Zone ID
echo "获取 Zone ID..."
ZONE_ID=$(curl -s -X GET "https://api.cloudflare.com/client/v4/zones?name=$ZONE_NAME" \
  -H "Authorization: Bearer $CLOUDFLARE_DNS_API_TOKEN" \
  -H "Content-Type: application/json" | jq -r '.result[0].id')

if [ -z "$ZONE_ID" ] || [ "$ZONE_ID" == "null" ]; then
    echo "错误: 无法获取 Zone ID"
    exit 1
fi

echo "Zone ID: $ZONE_ID"

# 检查记录是否存在
echo "检查现有记录..."
EXISTING_RECORD=$(curl -s -X GET "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/dns_records?name=$RECORD_NAME.$ZONE_NAME&type=$RECORD_TYPE" \
  -H "Authorization: Bearer $CLOUDFLARE_DNS_API_TOKEN" \
  -H "Content-Type: application/json")

RECORD_ID=$(echo "$EXISTING_RECORD" | jq -r '.result[0].id // empty')

if [ -n "$RECORD_ID" ]; then
    echo "记录已存在，更新中..."
    RESPONSE=$(curl -s -X PUT "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/dns_records/$RECORD_ID" \
      -H "Authorization: Bearer $CLOUDFLARE_DNS_API_TOKEN" \
      -H "Content-Type: application/json" \
      --data "{\"type\":\"$RECORD_TYPE\",\"name\":\"$RECORD_NAME\",\"content\":\"$RECORD_CONTENT\",\"ttl\":$RECORD_TTL,\"proxied\":$RECORD_PROXIED}")
else
    echo "创建新记录..."
    RESPONSE=$(curl -s -X POST "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/dns_records" \
      -H "Authorization: Bearer $CLOUDFLARE_DNS_API_TOKEN" \
      -H "Content-Type: application/json" \
      --data "{\"type\":\"$RECORD_TYPE\",\"name\":\"$RECORD_NAME\",\"content\":\"$RECORD_CONTENT\",\"ttl\":$RECORD_TTL,\"proxied\":$RECORD_PROXIED}")
fi

echo "$RESPONSE" | jq '.'

# 验证
if echo "$RESPONSE" | jq -e '.success' > /dev/null; then
    echo ""
    echo "✅ DNS 记录创建/更新成功!"
    echo "验证: dig $RECORD_NAME.$ZONE_NAME +short"
else
    echo ""
    echo "❌ DNS 记录创建/更新失败"
    exit 1
fi
