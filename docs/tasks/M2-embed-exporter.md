# M2 — Embed the traffic exporter (read in-process Xray stats)

Status: **planned**. Depends on M1.

## Goal

Report per-tenant traffic by reading the **same in-process `core.Instance`'s
StatsManager** instead of scraping `http://127.0.0.1:10085/debug/vars` with the
separate `xray-exporter` binary/unit. Keep the external report surface
(`/metrics`, `/v1/snapshots/latest`, accounts enrichment) byte-compatible so
billing-service and Prometheus are unaffected.

## Why

- Kills the `:10085` stats port, its `XRAY_STATS_TOKEN`, and a whole systemd unit.
- Counter reads are consistent with the exact config the node is running.
- Completes the "one binary, one config, one unit" story from M1.

## Verified facts (xray-core `v1.260327.0`)

- Enabling user stats requires **both** a `stats` app object **and** a `policy`
  with per-level `statsUserUplink/statsUserDownlink: true` and a `system` policy
  with `statsInboundUplink/Downlink` (see `third_party/xray-core/app/policy`).
  The current `config/xray.xhttp.template.json` has **neither** — this is a
  required template change (§Step 1).
- The dispatcher registers counters named
  `user>>>{email}>>>traffic>>>uplink` and `...>>>downlink`
  (`third_party/xray-core/app/dispatcher/default.go:165`). Since the generator
  already forces a unique `email` per client, **email = tenant billing key**.
- Access from the running instance:
  ```go
  mgr := inst.GetFeature(stats.ManagerType()).(stats.Manager) // core/xray.go:376
  up   := mgr.GetCounter("user>>>"+email+">>>traffic>>>uplink")   // may be nil until first byte
  online := mgr.GetAllOnlineUsers()                                // []string of emails
  // Counter.Value() int64  (features/stats/stats.go)
  ```

## Scope

**In:** in-process stats reader; snapshot builder; `/metrics` + `/healthz` +
`/v1/snapshots/latest` HTTP endpoints served by the agent; accounts enrichment
call; template stats/policy blocks; config for listen addr + report interval.

**Out:** changing the accounts/billing wire formats; the legacy standalone
`xray-exporter` repo (kept for scrape-a-node-you-don't-own scenarios — see
platform-plan §6 open decision).

## Steps

### Step 1 — enable stats in the Xray templates  *(config/xray.*.template.json)*
Add to both `xray.xhttp.template.json` and `xray.tcp.template.json`:
```jsonc
"stats": {},
"policy": {
  "levels": { "0": { "statsUserUplink": true, "statsUserDownlink": true } },
  "system": { "statsInboundUplink": true, "statsInboundDownlink": true }
}
```
Acceptance: after `Runner.Apply`, `mgr.GetCounter("user>>>u@x>>>traffic>>>uplink")`
is non-nil once that user passes traffic.

### Step 2 — NEW `internal/xrayembed`: expose the StatsManager
Add `Runner.Stats() (stats.Manager, bool)` returning
`inst.GetFeature(stats.ManagerType()).(stats.Manager)` under the mutex.
Acceptance: unit test registers a counter, `Add(100)`, reads `Value()==100`.

### Step 3 — NEW `internal/exporter/` package
- `collector.go`: given `map[name]*xrayembed.Runner` + the current client set
  (emails, from the sync loop's last `ListClients`), build a snapshot:
  ```go
  type UserTraffic struct { Email string; Uplink, Downlink int64; Online bool }
  type Snapshot struct { NodeID string; Env string; TakenAt time.Time; Users []UserTraffic }
  ```
  Read counters by name; treat nil counter as 0; `Online` from `GetAllOnlineUsers`.
- `enrich.go`: resolve `Email → account/tenant labels` via `ACCOUNTS_BASE_URL`
  (`INTERNAL_SERVICE_TOKEN`), mirroring the standalone exporter. Cache labels;
  refresh on unknown email.
- `server.go`: `http.ServeMux` with `/metrics` (Prometheus text), `/healthz`,
  `/v1/snapshots/latest` (JSON = `Snapshot`). Keep field names identical to the
  standalone exporter's `/v1/snapshots/latest` so billing-service is unchanged
  (verify against the ai-workspace-xstream/xray-exporter response schema before
  finalizing).

### Step 4 — wire as goroutine D in the supervisor  *(internal/agentmode/embedded.go)*
- Add `config.Exporter{ Enabled bool; ListenAddr string; Interval time.Duration; Env string }`.
- In `RunEmbedded`, when `Exporter.Enabled`, start `wg`-tracked goroutine D that
  ticks every `Interval`, snapshots via the collector, updates the Prometheus
  gauges, and pushes/serves the latest snapshot. Share the client-email set with
  the sync loop (the loop already fetches it — pass emails via a mutex-guarded
  latest-clients holder rather than re-fetching).
- Bind `ListenAddr` (default `127.0.0.1:9108`, matching current
  `deploy/.../defaults/main.yml:xray_exporter_listen_addr`).

### Step 5 — retire the separate exporter (deploy)
- Ansible role: gate `xray-exporter.service` behind `not embedded`.
- `setup-proxy.sh --embedded`: don't install `xray-exporter`.
- Keep the standalone binary buildable for non-embedded/legacy nodes.

## Data contracts (unchanged externally)
- `GET /metrics` → Prometheus text; per-user counters labeled by node + tenant.
- `GET /v1/snapshots/latest` → `Snapshot` JSON consumed by billing-service.
- Enrichment: `ACCOUNTS_BASE_URL` + `INTERNAL_SERVICE_TOKEN` + `EXPORTER_NODE_ID`.

## Risks
- **Counter lifecycle on hot-swap:** `Runner.Apply` builds a *new* instance;
  counters reset to 0 on swap. The snapshot consumer must treat counters as
  monotonic-with-resets (billing already deltas; confirm) or the collector must
  carry-forward last values across swaps. **Decide before Step 3.**
- **Email uniqueness:** enrichment assumes email↔tenant is 1:1. The generator
  enforces non-empty email; confirm the controller guarantees uniqueness.
- **Schema drift:** `/v1/snapshots/latest` must match the standalone exporter
  exactly; pin it with a golden-file test.

## Acceptance
- [ ] Templates enable stats; counter present after traffic (integration test).
- [ ] `internal/exporter` unit tests: collector maps counters→snapshot; nil-counter=0; online flag.
- [ ] Golden test: agent `/v1/snapshots/latest` JSON == standalone exporter schema.
- [ ] Manual: two tenants push traffic through one node; `/metrics` shows distinct per-email counters; billing-service ingests a snapshot with no code change.
- [ ] `:10085` no longer opened in embedded mode.
