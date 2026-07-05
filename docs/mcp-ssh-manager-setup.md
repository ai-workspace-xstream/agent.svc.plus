# MCP SSH Manager (local MCP server) setup

This document records how to run `bvisible/mcp-ssh-manager` locally and connect it to OpenAI Codex.

## Local clone location

The repo is cloned here:

```
/Users/shenlan/workspaces/Cloud-Neutral-Toolkit/agent.svc.plus/mcp-ssh-manager
```

## Prerequisites

- Node.js 18+
- npm

Optional (only if you want extra features):

- `rsync`
- `sshpass`

## Install dependencies

From the clone:

```bash
cd /Users/shenlan/workspaces/Cloud-Neutral-Toolkit/agent.svc.plus/mcp-ssh-manager
npm install
```

## Configure Codex to start the MCP server

Edit `~/.codex/config.toml` and add:

```toml
[mcp_servers.ssh-manager]
command = "node"
args = ["/Users/shenlan/workspaces/Cloud-Neutral-Toolkit/agent.svc.plus/mcp-ssh-manager/src/index.js"]
env = { SSH_CONFIG_PATH = "/Users/shenlan/.codex/ssh-config.toml" }
startup_timeout_ms = 20000
```

Create `~/.codex/ssh-config.toml` (or update it):

```toml
[ssh_servers.production]
host = "prod.example.com"
user = "admin"
key_path = "~/.ssh/id_rsa"
port = 22
default_dir = "/var/www"
description = "Production server"
```

## Start / test

Codex will start the server automatically using the config above.

Optional CLI helpers (from the repo):

```bash
cd /Users/shenlan/workspaces/Cloud-Neutral-Toolkit/agent.svc.plus/mcp-ssh-manager
./node_modules/.bin/ssh-manager codex setup
./node_modules/.bin/ssh-manager codex test
```

## Notes

- The MCP server is stdio-based; it is normally launched by Codex, not started manually.
- To run manually (for debugging), you can execute:

```bash
node /Users/shenlan/workspaces/Cloud-Neutral-Toolkit/agent.svc.plus/mcp-ssh-manager/src/index.js
```
