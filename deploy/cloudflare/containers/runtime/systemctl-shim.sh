#!/usr/bin/env bash
set -euo pipefail

if [ "${1:-}" != "restart" ]; then
  echo "systemctl-shim only supports: restart <unit>" >&2
  exit 1
fi

UNIT="${2:-}"
case "$UNIT" in
  xray.service|xray-tcp.service)
    exec /usr/local/bin/restart-xray.sh "$UNIT"
    ;;
  *)
    echo "unsupported unit for systemctl-shim: $UNIT" >&2
    exit 1
    ;;
esac
