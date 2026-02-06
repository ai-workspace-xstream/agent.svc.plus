# Agent Service Plus

`agent.svc.plus` is the lightweight runtime agent for the Cloud Neutral Toolkit. It manages Xray instances on Virtual Machines (VMs), handling configuration synchronization, traffic reporting, and certificate management via Caddy.

## Features

*   **Zero-Dependency Runtime**: Single binary deployment (Go).
*   **Xray Management**: Automatically configures and reloads Xray (XHTTP & TCP modes).
*   **Secure Communication**: Authenticated synchronization with `accounts.svc.plus`.
*   **Automated TLS**: Integrated with Caddy for automatic HTTPS/Certificates.

## 🚀 Quick Start (One-Shell)

Deploy `agent.svc.plus` with a single command. This script sets up Xray, Caddy, Go, and the Agent service.

> Note: `scripts/init_vhost.sh` is kept as a compatibility shim and now forwards to `scripts/setup-proxy.sh`.

* 默认当前主机名作为域名*

```bash
curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | bash
```

### Default Installation
Installs the latest stable version and uses the current system hostname as the domain.

```bash
curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | bash
```

### Custom Domain Installation
Specify a custom domain if your hostname is not configured or you wish to use a different one.

```bash
curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | \
  bash -s -- --node xhttp.example.com
```

### Controller Registration (recommended)
Pass controller URL and token via env so the node can register immediately:

```bash
AUTH_URL=https://accounts-svc-plus-266500572462.asia-northeast1.run.app \
INTERNAL_SERVICE_TOKEN=replace-with-token \
curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | \
  bash -s -- --node hk-xhttp.svc.plus
```

## Configuration

The agent is configured via `/etc/systemd/system/agent.service` environment variables or a YAML config file.

**Default Config Location**: `./account-agent.yaml` or `/etc/agent/account-agent.yaml`

```yaml
agent:
  id: "your-node-id"
  controllerUrl: "https://accounts.svc.plus"
  apiToken: "your-secret-token"
xray:
  sync:
    enabled: true
    outputPath: "/usr/local/etc/xray/config.json"
```

## Architecture

See [Design Spec](docs/design.md) for detailed architecture and interaction flows.
