#!/usr/bin/env bash
set -euo pipefail

SERVICE_UNIT="${1:-xray.service}"

case "$SERVICE_UNIT" in
  xray.service)
    PID_FILE="${XRAY_PID_FILE:-/var/run/agent-svc-plus/xray.pid}"
    ;;
  xray-tcp.service)
    PID_FILE="${XRAY_TCP_PID_FILE:-/var/run/agent-svc-plus/xray-tcp.pid}"
    ;;
  *)
    echo "unsupported service: $SERVICE_UNIT" >&2
    exit 1
    ;;
esac

if [ ! -s "$PID_FILE" ]; then
  exit 0
fi

PID="$(cat "$PID_FILE")"
if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
  kill "$PID"
fi
