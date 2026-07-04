# XStream Platform — agent.svc.plus role & multi-tenant design

Status: planning consolidation (feature branch `feature/monolith-caddy-xray-embed`).

This document places `agent.svc.plus` inside the XStream platform and pins down
the multi-tenant Xray-as-a-service contract it participates in: what runs on a
node, who owns which decision, and how a tenant's credential and traffic move
through the system.

## 1. Platform layers

```
                             XStream Platform
                                    │
   Clients ────────────────────────┼───────────────────────────────
        │                          │                              │
        ▼                          ▼                              ▼
  XStream Desktop            XStream iOS/Android          Secure Browser Access
  (Flutter, available)       (available)                  (planned)
        │                          │                              │
        └───────── VLESS (xhttp / tcp-vision) · SOCKS5/CONNECT ────┘
                                    │
                                    ▼
                     ┌─────────────────────────────┐
                     │  agent-proxy-server.svc.plus │   ← the NODE
                     │  "Secure Access Gateway"     │      (this repo runs here)
                     └─────────────────────────────┘
                                    │  reports up / pulls config
     ┌──────────────────────────────┼──────────────────────────────┐
     ▼                              ▼                               ▼
 accounts.svc.plus            billing-service                console.svc.plus
 (identity, node & user       (rating / reconcile)           (operator + tenant UI)
  registry, source of truth)
                                    │  dataplane egress
                                    ▼
             Internet · Home LAN · Company LAN · Cloud VPC
```

**Control plane = `accounts.svc.plus`** (source of truth for nodes, tenants,
UUIDs, entitlements). **Dataplane = the node** (`agent.svc.plus` + Caddy +
Xray). **Console** is the human surface over accounts/billing; it issues no
commands to nodes directly — it mutates state in accounts, and nodes converge
to it by polling. This poll-to-converge model is what makes "enable / pause /
cleanup a tenant" safe and restart-tolerant.

## 2. What runs on a node (`agent.svc.plus` bundle)

Today: four cooperating programs. Target (this branch): **one binary**, same
four responsibilities as in-process goroutines.

| Responsibility | Today | Monolith target |
| --- | --- | --- |
| TLS / ACME + HTTP routing | `caddy` (systemd) | `caddyembed` goroutine |
| VLESS **xhttp** inbound (QUIC-in-QUIC / MTU-safe, Claude-Code friendly) | `xray` (systemd) | `xrayembed.Runner` "xhttp" |
| VLESS **tcp-vision** inbound (fallback, direct TLS) | `xray-tcp` (systemd) | `xrayembed.Runner` "tcp" |
| Per-tenant traffic stats → accounts/billing | `xray-exporter` (systemd) | `exporter` goroutine (reads in-process stats) |
| Control loop: sync config, report node health | `agent-svc-plus` | supervisor `agentmode.RunEmbedded` |

Ports/sockets (standalone/VPS topology): Caddy owns `:80/:443` and terminates
TLS via ACME; xhttp Xray listens on a unix socket (`/dev/shm/xray.sock`) that
Caddy reverse-proxies for the `/split` path; tcp-vision Xray listens on `:1443`
with its own TLS reading Caddy's cert files.

## 3. Multi-tenant model

- **Tenant credential = a VLESS UUID.** The controller's client list
  (`GET /api/agent-server/v1/users`) is the set of UUIDs currently entitled on
  this node. `xrayconfig.Generator` writes them into `inbounds[].settings.clients[]`.
- **Billing key = client email.** The generator *requires* every client to
  carry an `email` (`internal/xrayconfig/generator.go`) precisely because Xray's
  stats API keys per-user counters by email. So one tenant ⇒ one UUID ⇒ one
  email label ⇒ one traffic bucket.
- **Node identity = `agent.id` / `nodeId`** plus `region` / `lineCode` /
  `pricingGroup`, carried on every status report.

Tenant lifecycle is expressed purely as membership in the controller's client
list — the node never decides, it converges:

| Operator action (in console → accounts) | What the node sees next sync | Node effect |
| --- | --- | --- |
| **Enable / provision** tenant | UUID appears in `/users` | added to `clients[]`, inbound now accepts it |
| **Controlled pause / suspend** | UUID removed (or flagged) in `/users` | dropped from `clients[]`; existing sessions cut at next config swap |
| **Cleanup / deprovision** | UUID absent | not written; local config no longer references it |

There is no separate "delete UUID" RPC to the node: absence from the authoritative
list *is* the cleanup. A node rebuilt from scratch reaches the same state on its
first sync. (Hard-kill of in-flight sessions happens at the config hot-swap; see
`xrayembed.Runner.Apply` — new instance validated, old one closed.)

## 4. Control-plane flows

### 4.1 Node registration & heartbeat (agent → accounts)
- First `GET /api/agent-server/v1/users` doubles as **registration + inventory
  pull** (auth: `Authorization: Bearer` + `X-Service-Token` + `X-Agent-ID`).
- `POST /api/agent-server/v1/status` every `statusInterval` carries health,
  client count, sync revision, and node facts (`nodeId/region/lineCode/pricingGroup`).
  This is how console/accounts show a node as live and which line/pricing it serves.

### 4.2 Tenant config sync (agent ← accounts)
Every `syncInterval`: `ListClients` → render each inbound (xhttp, tcp) →
apply. Embedded mode renders in memory and hot-swaps `core.Instance`; legacy
mode writes JSON + `systemctl restart`. Enable/pause/cleanup all ride this one
loop (§3).

### 4.3 Traffic accounting (exporter → accounts/billing)
`xray-exporter` is the **v1 translation layer** for the billing/control plane:
1. **Scrape** raw counters from Xray stats (`XRAY_STATS_URL`, today
   `http://127.0.0.1:10085/debug/vars`).
2. **Enrich** — resolve per-email counters to tenant/account identity via
   `ACCOUNTS_BASE_URL` (auth `INTERNAL_SERVICE_TOKEN`), tagged by `EXPORTER_NODE_ID`.
3. **Expose / report** — `/metrics` (Prometheus), `/healthz`, and
   `/v1/snapshots/latest` (normalized snapshot consumed by billing-service).

### 4.4 Billing orchestration (agent → billing-service)
When `billing.enabled`, the agent periodically POSTs `/v1/jobs/collect-and-rate`
(`collectInterval`) and `/v1/jobs/reconcile` (`reconcileInterval`). The agent
only *triggers* jobs; it does not own billing data (`billing_client.go`).

## 5. Responsibility boundaries

| Concern | Owner | Not responsible |
| --- | --- | --- |
| Who may use which node, entitlements, UUIDs | accounts.svc.plus | node decides nothing |
| Operator/tenant UI, provisioning intent | console.svc.plus | never talks to nodes directly |
| Rating, invoices, reconciliation | billing-service | agent only triggers jobs |
| TLS certs, routing, inbound crypto | node (Caddy + Xray) | control plane never holds node TLS keys |
| Traffic measurement → identity mapping | xray-exporter (+ accounts labels) | agent supervisor doesn't parse stats |
| Converging local Xray to the entitled UUID set | agent supervisor | — |

## 6. Monolith consolidation — what changes for the exporter

The embedded direction removes the exporter's HTTP scrape entirely when Xray is
in-process. Instead of polling `/debug/vars` over localhost, an in-process
`exporter` goroutine can read the **same `core.Instance`'s StatsManager
directly** (query per-email up/downlink counters in memory). Benefits:

- No `10085` stats port to expose or firewall; no scrape auth token.
- Counter reads are consistent with the exact config the node is running.
- One binary, one config, one systemd unit — matches §2.

The external report surface (`/metrics`, `/v1/snapshots/latest`, accounts
enrichment) stays identical, so billing-service and Prometheus are unaffected.
This is a follow-up to the current branch, tracked below.

## 7. Roadmap

- **M1 (this branch):** embed Caddy + Xray as libraries; k3s-style multicall
  binary; two-goroutine supervisor; CI builds one static binary + `xray`/`caddy`
  symlinks. *(done / in review — PR #5)*
- **M2:** embed the exporter as a 4th goroutine reading in-process Xray stats;
  retire `xray-exporter.service` and the `:10085` scrape path.
- **M3:** runtime-mode switch (standalone/VPS vs Cloud Run vs Cloudflare
  Containers) — see `monolith-embed-plan.md` §Deferred. Cloud Run/CF terminate
  TLS at the edge, so Caddy/ACME is skipped and xhttp runs plaintext behind the
  edge.
- **M4:** WireGuard inbound alongside VLESS (per platform diagram "Future
  Protocols") under the same tenant/UUID + accounting model.

## Related docs
- `docs/architecture/proxy-server/monolith-embed-plan.md` — binary structure & call ordering.
- `docs/architecture/proxy-server/overview.md` — runtime API matrix.
- xray-exporter: https://github.com/ai-workspace-xstream/xray-exporter
