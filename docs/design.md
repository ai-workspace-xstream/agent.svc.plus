# Agent Service Plus Design & Architecture

## Overview
`agent.svc.plus` is a lightweight control agent designed to run on a VM (Virtual Machine). It serves as the runtime for Xray, managing connectivity and configuration synchronization with the central `accounts.svc.plus` service while leaving traffic metric translation and billing to separate services.

## Architecture

*   **Runtime**: Native Binary (Go), running on VM (Systemd managed).
*   **Proxy Core**: Xray (XTLS).
*   **Web Server**: Caddy (with DNS plugins for ACME).
*   **Orchestration**: `agent.svc.plus` binary.

### Key Components

1.  **Agent (Go Binary)**:
    *   **Role**: Controller / orchestrator.
    *   **Responsibilities**:
        *   Authenticate with `accounts.svc.plus`.
        *   Fetch configuration (UUIDs, Routing rules).
        *   Generate Xray configuration files at `/usr/local/etc/xray/`.
        *   Reload `xray` services (`systemctl reload xray`, `systemctl reload xray-tcp`).
        *   Schedule reconciliation jobs and future control actions.

2.  **Xray (Proxy)**:
    *   **Mode 1: XHTTP (Split Mode)**:
        *   Listens on `/dev/shm/xray.sock` (Unix Socket).
        *   Served via Caddy reverse proxy on port 443 (HTTPS).
        *   Path: `/split/*`.
    *   **Mode 2: TCP (Direct Mode)**:
        *   Listens on port 1443.
        *   Direct TCP/TLS termination.

3.  **Caddy (Web/Reverse Proxy)**:
    *   Manages port 80/443.
    *   Handles ACME (Let's Encrypt/ZeroSSL) automatically.
    *   Reverse proxies `/split/*` to Xray via Unix socket.

4.  **xray-exporter / billing-service (separate control plane utilities)**:
    *   `xray-exporter` polls raw Xray stats and emits Prometheus metrics.
    *   `billing-service` consumes minute-level traffic deltas and writes PostgreSQL billing rows.
    *   `agent` may trigger or observe these jobs, but does not own their data model.

## Acceptance

See `docs/testing/agent-reconciliation-acceptance.md` for the orchestration-only
acceptance checklist used by CRT-007.

## Interaction Flow

1.  **Startup**:
    *   `agent` starts via systemd.
    *   `agent` reads `account-agent.yaml` for auth token and API URL.

2.  **Sync Loop**:
    *   `agent` polls `accounts.svc.plus` (e.g., every minute).
    *   Endpoint: `GET /api/agent/sync` (or `GET /api/agent/config`).
    *   Payload includes: User UUIDs, Traffic limits (optional).

3.  **计费与观测边界**:
    *   Xray 仅提供原始流量数据。
    *   exporter 负责指标化，不参与计费。
    *   PostgreSQL 负责计费真相源。

4.  **Reconfiguration**:
    *   If config changes:
        *   Agent regenerates `config.json` and `tcp-config.json`.
        *   Agent executes reload command.

## Deployment Specification

| Dimension | Requirement | Description |
| :--- | :--- | :--- |
| **OS** | Linux (Ubuntu/Debian recommended) | |
| **Network** | Public IP + Domain | DNS must be pre-resolved |
| **Ports** | 80, 443, 1443 (+ optional 5443) | 80(ACME), 443(XHTTP), 1443(TCP), 5443(stunnel/PostgreSQL co-location) |
| **Resources** | 1C/1G (Min), 2C/4G (Rec) | |

## Installation (One-Shell)

The `init_vhost.sh` script automates the setup:
1.  Installs core dependencies (curl, wget, git, socat).
2.  Installs **Xray** (official script).
3.  Installs **Go** and builds **Xcaddy** (custom Caddy with DNS plugins).
4.  Configures `systemd` services for Xray and Agent.
5.  Sets up file structure (`/usr/local/etc/xray`, `/var/log/xray`).

## Configuration Structure

### `account-agent.yaml` (Local Config)
```yaml
api_url: "https://accounts-svc-plus-266500572462.asia-northeast1.run.app"
agent_id: "example-agent-id"
agent_token: "example-secret-token"
sync_interval: "60s"
```

### Xray Configs
*   `/usr/local/etc/xray/config.json` (XHTTP Inbound)
*   `/usr/local/etc/xray/tcp-config.json` (TCP Inbound)
