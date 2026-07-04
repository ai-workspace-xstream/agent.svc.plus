# agent.svc.plus — implementation task plans

Landable, file-level task specs for the monolith consolidation. Each doc is
self-contained: goal, current state (grounded in real files/symbols), exact
edits, data contracts, acceptance tests, and risks.

Read order and dependency:

| Task | Goal | Depends on | Status |
| --- | --- | --- | --- |
| [M1](M1-embed-caddy-xray.md) | Embed Caddy + Xray as libraries; multicall binary; two-goroutine supervisor | — | landed (PR #5), hardening items remain |
| [M2](M2-embed-exporter.md) | Read per-tenant traffic from the in-process Xray StatsManager; retire the `:10085` scrape + separate exporter unit | M1 | planned |
| [M3](M3-runtime-modes.md) | `runtime.mode` switch: standalone/VPS vs Cloud Run vs Cloudflare Containers (edge-TLS) | M1 | planned |
| [M4](M4-wireguard-inbound.md) | WireGuard access path alongside VLESS under the same tenant/UUID + accounting model | M2 | exploratory |

Cross-refs:
- Platform context & responsibility boundaries: [../architecture/xstream-platform-plan.md](../architecture/xstream-platform-plan.md)
- Binary structure & call ordering: [../architecture/proxy-server/monolith-embed-plan.md](../architecture/proxy-server/monolith-embed-plan.md)

Conventions used in these docs:
- **`file:symbol`** points at code that exists today.
- **NEW** marks a file/symbol to be created.
- Acceptance items are written so they can become CI checks or `go test` cases.
