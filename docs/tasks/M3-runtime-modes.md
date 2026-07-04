# M3 — Runtime-mode switch (standalone / Cloud Run / Cloudflare Containers)

Status: **planned**. Depends on M1.

## Goal

A single `runtime.mode` config that selects the deployment topology, because
TLS ownership differs: on standalone/VPS **Caddy owns ACME**, but on Cloud Run
and Cloudflare Containers **the platform edge terminates TLS** and the container
can't get :80/:443 or issue certs. Today `caddyembed.BuildStandaloneConfig`
hardcodes the standalone assumption (its doc comment already says so).

## Why

Without the switch, embedded mode only works on standalone/VPS. The platform
plan (§7 M3) needs Cloud Run / CF to run the *same binary* with Caddy disabled
and Xray listening plaintext behind the edge.

## Current state (grounded)

- `internal/caddyembed/config.go:BuildStandaloneConfig` — standalone only (:443,
  DNS-01 ACME, reverse-proxy to unix socket).
- `deploy/gcp/cloud-run/agent-sidecar-service.yaml` + `config/account-agent.cloudrun.yaml`
  — Cloud Run runs Xray XHTTP on `:8080` h2c, TLS at Google's LB; agent is a sidecar.
- `deploy/cloudflare/containers/runtime/entrypoint.sh` — CF runs xhttp on a unix
  socket proxied by a Worker; TLS at Cloudflare.
- `internal/agentmode/embedded.go:RunEmbedded` — currently *always* starts Caddy
  when `Caddy.Embedded`.

## Design

Add `runtime.mode` ∈ {`standalone`, `cloudrun`, `cloudflare`} (default
`standalone`). Mode drives three decisions:

| Decision | standalone | cloudrun | cloudflare |
| --- | --- | --- | --- |
| Start embedded Caddy? | yes (ACME) | **no** | **no** |
| xhttp inbound listen | unix socket, behind Caddy | `:8080` h2c plaintext | unix socket, behind Worker |
| tcp-vision inbound | `:1443` own TLS | n/a (single port) | n/a |
| Cert source for tcp | Caddy storage (`/var/lib/caddy/...`) | — | — |

## Steps

### Step 1 — config  *(internal/config/config.go)*
Add:
```go
type Runtime struct { Mode string `yaml:"mode"` } // standalone|cloudrun|cloudflare
```
on `Config`. Validate in `Load`; default empty → `standalone`. Env override
`RUNTIME_MODE`.

### Step 2 — gate Caddy  *(internal/agentmode/embedded.go)*
Start goroutine A only when `mode == standalone` **and** `Caddy.Embedded`.
For cloudrun/cloudflare, skip Caddy entirely; log the mode and that TLS is
edge-terminated.

### Step 3 — mode-aware xhttp listen address
The xhttp inbound listen must differ by mode. Two options:
- (a) Select the template per mode (`xhttp.socket.template.json` vs
  `xhttp.plaintext-8080.template.json`), or
- (b) Post-process the rendered config to set `inbounds[xhttp].listen`/`port`
  from mode.
Prefer (a): explicit, no structural rewriting. Add a `listenTemplate` per mode
resolved in `buildXrayTargets`.

### Step 4 — docs & deploy
- Update `monolith-embed-plan.md` §Deferred → point here.
- Cloud Run: replace the two-container sidecar YAML with a single container
  running the monolith in `cloudrun` mode (Xray only). CF: single container in
  `cloudflare` mode behind the existing Worker.

## Acceptance
- [ ] `runtime.mode: standalone` → Caddy up, xhttp on socket, tcp on :1443 (M1 behavior unchanged).
- [ ] `runtime.mode: cloudrun` → Caddy **not** started; xhttp binds `:8080` h2c; container serves through Google LB.
- [ ] `runtime.mode: cloudflare` → Caddy not started; xhttp on socket; Worker proxies through.
- [ ] Invalid mode → startup error with the three valid values listed.
- [ ] One binary image runs in all three modes selected purely by config/env.

## Risks
- Cloud Run gives one ingress port; tcp-vision (:1443) has no place there —
  document that tcp-vision is standalone-only, xhttp is the portable path.
- CF Worker↔container socket contract must stay as the existing entrypoint expects.
