# Agent Service Plus - Example Configurations

This directory contains example configuration files for setting up and running the Agent Service Plus.

## Files

### `account-agent.yaml`
Template configuration file for the agent service. This file should be copied to `/etc/agent/account-agent.yaml` on your agent node and customized with your specific settings.

**Key Configuration Items:**
- `agent.id`: Unique identifier for this agent node (e.g., `hk-xhttp.svc.plus`)
- `agent.controllerUrl`: URL of the accounts.svc.plus controller
- `agent.apiToken`: Shared authentication token (must match `INTERNAL_SERVICE_TOKEN` in accounts.svc.plus)
- `xray.sync.targets`: Xray transport configurations (XHTTP and TCP)

## Quick Start

1. **Copy the template**:
   ```bash
   sudo cp example/account-agent.yaml /etc/agent/account-agent.yaml
   ```

2. **Edit the configuration**:
   ```bash
   sudo nano /etc/agent/account-agent.yaml
   ```
   
   Update the following fields:
   - `agent.id`: Your node's domain name
   - `agent.apiToken`: Your internal service token

3. **Restart the agent service**:
   ```bash
   sudo systemctl restart agent-svc-plus
   ```

## Automated Setup

For automated installation, use the `setup-proxy.sh` script which handles configuration automatically:

```bash
curl -fsSL https://raw.githubusercontent.com/cloud-neutral-toolkit/agent.svc.plus/main/scripts/setup-proxy.sh | \
  bash -s -- --node your-node.svc.plus \
  --auth-url https://accounts-svc-plus-266500572462.asia-northeast1.run.app \
  --internal-service-token your-token-here
```

The script will:
- Install all dependencies (Go, Caddy, Xray)
- Build and install the agent binary
- Generate configuration from the template
- Set up systemd services
- Start all services

## Configuration Notes

- **Multi-Agent Support**: Multiple agents can use the same `apiToken` (shared token authentication)
- **Agent ID**: Each agent must have a unique `id` to identify itself to the controller
- **Heartbeat**: Agents send status updates every `statusInterval` (default: 1 minute)
- **Sync**: Xray client configurations are synchronized every `syncInterval` (default: 5 minutes)
