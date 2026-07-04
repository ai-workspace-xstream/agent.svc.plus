# Cloudflare Workers Edge Proxy

This Worker provides a Cloudflare edge entrypoint for the agent protocol:

- `GET /healthz`
- `GET /api/agent-server/v1/users`
- `POST /api/agent-server/v1/status`

It is an edge proxy, not a replacement for the VM daemon. Xray config rendering,
local file writes, and process restart stay on the host where `agent-svc-plus`
runs.

## 1) Install

```bash
cd deploy/cloudflare/workers
npm install
```

## 2) Configure

Set regular var in `wrangler.toml`:

- `CONTROLLER_BASE_URL`: upstream controller URL (for example `https://accounts.svc.plus`)

Optional secrets:

```bash
npx wrangler secret put EDGE_ACCESS_TOKEN
npx wrangler secret put FORCE_SERVICE_TOKEN
```

- `EDGE_ACCESS_TOKEN`: if set, inbound token must match.
- `FORCE_SERVICE_TOKEN`: if set, Worker uses this token to call upstream.

## 3) Local run

```bash
npm run dev
```

## 4) Deploy

```bash
npm run deploy
```

After deployment, set Agent config:

```yaml
agent:
  controllerUrl: "https://<your-worker-subdomain>.workers.dev"
  apiToken: "token-used-by-edge"
```

If `EDGE_ACCESS_TOKEN` is configured, set `apiToken` to that value.
Otherwise use the controller service token directly.
