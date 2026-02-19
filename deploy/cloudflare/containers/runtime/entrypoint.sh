#!/usr/bin/env bash
set -euo pipefail

AGENT_CONFIG_TEMPLATE="${AGENT_CONFIG_TEMPLATE:-/etc/agent/account-agent.container.yaml}"
AGENT_CONFIG_PATH="${AGENT_CONFIG_PATH:-/etc/agent/account-agent.yaml}"
XRAY_CONFIG_PATH="${XRAY_CONFIG_PATH:-/usr/local/etc/xray/config.json}"
XRAY_BOOTSTRAP_PATH="${XRAY_BOOTSTRAP_PATH:-/etc/agent/xray.bootstrap.json}"
XRAY_PID_FILE="${XRAY_PID_FILE:-/var/run/agent-svc-plus/xray.pid}"
XRAY_TCP_PID_FILE="${XRAY_TCP_PID_FILE:-/var/run/agent-svc-plus/xray-tcp.pid}"
AGENT_PID_FILE="${AGENT_PID_FILE:-/var/run/agent-svc-plus/agent.pid}"
XRAY_STOP_FILE="${XRAY_STOP_FILE:-/var/run/agent-svc-plus/xray.stop}"
HEALTH_PORT="${HEALTH_PORT:-8080}"
AGENT_ID="${AGENT_ID:-${DOMAIN:-}}"
AGENT_CONTROLLER_URL="${AGENT_CONTROLLER_URL:-${CONTROLLER_URL:-}}"
AGENT_API_TOKEN="${AGENT_API_TOKEN:-${INTERNAL_SERVICE_TOKEN:-}}"

mkdir -p /etc/agent /usr/local/etc/xray /usr/local/etc/xray/templates /var/run/agent-svc-plus
rm -f "$XRAY_STOP_FILE"

if [ ! -f "$AGENT_CONFIG_PATH" ]; then
  cp "$AGENT_CONFIG_TEMPLATE" "$AGENT_CONFIG_PATH"
fi

if [ ! -s "$XRAY_CONFIG_PATH" ]; then
  cp "$XRAY_BOOTSTRAP_PATH" "$XRAY_CONFIG_PATH"
fi

update_yaml_value() {
  local key="$1"
  local value="$2"
  local file="$3"
  local escaped
  escaped="$(printf '%s' "$value" | sed -e 's/[\/&]/\\&/g')"
  sed -i -E "s|^([[:space:]]*${key}:[[:space:]]*).*$|\\1\"${escaped}\"|g" "$file"
}

if [ -n "${AGENT_ID}" ]; then
  update_yaml_value "id" "$AGENT_ID" "$AGENT_CONFIG_PATH"
fi
if [ -n "${AGENT_CONTROLLER_URL}" ]; then
  update_yaml_value "controllerUrl" "$AGENT_CONTROLLER_URL" "$AGENT_CONFIG_PATH"
fi
if [ -n "${AGENT_API_TOKEN}" ]; then
  update_yaml_value "apiToken" "$AGENT_API_TOKEN" "$AGENT_CONFIG_PATH"
fi

xray_loop() {
  while true; do
    /usr/local/bin/xray run -config "$XRAY_CONFIG_PATH" &
    local xray_pid=$!
    echo "$xray_pid" > "$XRAY_PID_FILE"
    wait "$xray_pid" || true

    if [ -f "$XRAY_STOP_FILE" ]; then
      break
    fi
    sleep 1
  done
}

/usr/local/bin/agent-healthz -listen ":${HEALTH_PORT}" &
HEALTH_PID=$!

xray_loop &
XRAY_LOOP_PID=$!

/usr/local/bin/agent-svc-plus -config "$AGENT_CONFIG_PATH" &
AGENT_PID=$!
echo "$AGENT_PID" > "$AGENT_PID_FILE"

shutdown() {
  touch "$XRAY_STOP_FILE"

  if [ -s "$XRAY_PID_FILE" ]; then
    kill "$(cat "$XRAY_PID_FILE")" 2>/dev/null || true
  fi
  kill "$AGENT_PID" 2>/dev/null || true
  kill "$XRAY_LOOP_PID" 2>/dev/null || true
  kill "$HEALTH_PID" 2>/dev/null || true
}

trap shutdown SIGINT SIGTERM

set +e
wait -n "$AGENT_PID" "$XRAY_LOOP_PID" "$HEALTH_PID"
EXIT_CODE=$?
set -e

shutdown
wait || true
exit "$EXIT_CODE"
