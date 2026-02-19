# Cloudflare Containers / Cloud Run Runtime Evaluation

This directory provides a migration-evaluation runtime where:

- `xray` runs as a managed subprocess (`/usr/local/bin/xray run -config /usr/local/etc/xray/config.json`)
- `agent-svc-plus` runs with YAML config (`/usr/local/bin/agent-svc-plus -config /etc/agent/account-agent.yaml`)

Both processes run in the same container using `container-entrypoint.sh`.

The following agent fields are configurable via environment variables:

- `agent.id` -> `AGENT_ID` (fallback: `DOMAIN`)
- `agent.controllerUrl` -> `AGENT_CONTROLLER_URL` (fallback: `CONTROLLER_URL`)
- `agent.apiToken` -> `AGENT_API_TOKEN` (fallback: `INTERNAL_SERVICE_TOKEN`)

## Cloudflare Dashboard Configuration

When creating from Cloudflare dashboard (Workers & Pages -> Create -> Import a repository),
configure this project with:

- Path: `deploy/cloudflare/containers`
- Deploy command: `npm run deploy`

Set runtime variables/secrets:

- Variable `AGENT_ID` = your node id (for example `hk-xhttp.svc.plus`)
- Variable `AGENT_CONTROLLER_URL` = accounts controller URL
- Secret `AGENT_API_TOKEN` = service token

Equivalent Wrangler commands:

```bash
npx wrangler secret put AGENT_API_TOKEN
```

## Files

- `Dockerfile`: builds `agent-svc-plus` + health binary, installs `xray`.
- `wrangler.toml`: Cloudflare Containers binding.
- `worker/src/index.js`: control-plane Worker routes.
- `runtime/*`: entrypoint, restart script, bootstrap config.
- `cloud-run.service.yaml`: Cloud Run deployment sample for the same image.

## Local Build Validation

```bash
docker build -f deploy/cloudflare/containers/Dockerfile -t agent-svc-plus-runtime:local .
docker run --rm -p 8080:8080 \
  -e AGENT_ID=hk-xhttp.svc.plus \
  -e AGENT_CONTROLLER_URL=https://accounts-svc-plus-266500572462.asia-northeast1.run.app \
  -e AGENT_API_TOKEN=replace-with-token \
  agent-svc-plus-runtime:local
```

Then check:

```bash
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/debug/processes
```

## Cloudflare Containers

Install and validate:

```bash
cd deploy/cloudflare/containers
npm install
npm run check
```

Deploy:

```bash
npm run deploy
```

Set secret token before deploy:

```bash
npx wrangler secret put AGENT_API_TOKEN
```

Control routes:

- `/healthz`
- `/container/healthz?node=<node-id>`
- `/container/readyz?node=<node-id>`
- `/container/debug/processes?node=<node-id>`

## Cloud Run

Build and deploy with same image:

```bash
gcloud builds submit --tag REGION-docker.pkg.dev/PROJECT_ID/REPOSITORY/agent-svc-plus-runtime:latest .
gcloud run services replace deploy/cloudflare/containers/cloud-run.service.yaml --region REGION
```

## Feasibility Notes

- Process model (`xray + agent` in one container): feasible on both platforms.
- `agent` sync/restart behavior: supported via `restart-xray.sh` (no systemd required).
- Xray traffic plane constraints:
  - Cloudflare Containers: not a direct replacement for public L4 ingress in this setup; keep it as runtime evaluation/control-plane.
  - Cloud Run: only HTTP(S) ingress model; not suitable for exposing Xray raw TCP/1443 data plane.

## Recommendation

For evaluating "can both processes run together", use **Cloudflare Containers first** because `wrangler` + DO lifecycle is closer to container runtime control.

For production proxy data plane (especially TCP/443/1443 semantics), keep VM/K8s style runtime; Cloud Run is the least suitable option among these two.
