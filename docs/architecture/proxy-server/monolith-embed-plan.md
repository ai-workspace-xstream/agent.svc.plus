# Monolithic agent binary: embedded Caddy + Xray

Status: feature branch `feature/monolith-caddy-xray-embed` (WIP).

## Goal

Ship `agent-svc-plus` as **one statically-linked binary** that carries the
node's whole dataplane in-process, instead of orchestrating three separate
programs (`caddy`, `xray`, `agent-svc-plus`) via `setup-proxy.sh` + systemd.

Two consequences drive the design:

- **One process, two goroutines.** Caddy and Xray run as *libraries* inside
  the agent process. A config change fetched from the controller becomes a
  direct function call (`runner.Apply(bytes)`), not a file write followed by
  `systemctl restart xray`.
- **One file to ship.** `CGO_ENABLED=0` cross-compile for `linux/amd64` and
  `linux/arm64` yields a single static artifact. Upgrade = replace one file.
  (Verified: ~68 MB amd64 / ~64 MB arm64, statically linked.)

Decision: embed **upstream `github.com/xtls/xray-core`** directly (not
`libXray`). libXray is the FFI/gomobile wrapper the XStream *client* uses;
for a Go server we build against xray-core's own `core.Instance` API, which
is smaller to reason about and already exercised here.

Pinned versions:

| Component | Module | Version |
| --- | --- | --- |
| Xray | `github.com/xtls/xray-core` | `v1.260327.0` (core 26.3.27) |
| Caddy | `github.com/caddyserver/caddy/v2` | `v2.11.3` |
| L4 | `github.com/mholt/caddy-l4` | `v0.1.1` |
| DNS | `github.com/caddy-dns/{cloudflare,alidns}` | `v0.2.4 / v1.0.29` |

## Binary structure (k3s-style multicall)

One compiled file, three "personalities" chosen at startup — by `argv[0]`
(symlink) or by first subcommand. Existing scripts/systemd that call
`/usr/local/bin/xray run ...` keep working if `xray` is a symlink to this
binary.

```
cmd/agent/main.go          # dispatcher (argv[0] + subcommand → personality)
cmd/agent/supervisor.go    # personality: agent supervisor (runAgent)

internal/xrayembed/        # Xray as a library
  runner.go                #   Runner: build/hot-swap a core.Instance
  cli.go                   #   RunCLI: `agent xray ...` == the xray binary
internal/caddyembed/       # Caddy as a library
  instance.go              #   Apply/Stop/Version (caddy.Load)
  config.go                #   JSON config builder (TLS DNS-01 + xhttp proxy)
internal/agentmode/
  embedded.go              #   RunEmbedded: the two-goroutine supervisor
  runner.go                #   Run: legacy external-process path (kept)
```

Dispatch table (`main.go`):

| Invocation | Personality |
| --- | --- |
| `agent` / `agent server` / `agent run` | supervisor (`runAgent`) |
| `agent xray ...` or symlink `xray` | `xrayembed.RunCLI()` |
| `agent caddy ...` or symlink `caddy` | `caddycmd.Main()` |
| `agent version` | combined version string |

Verified working: `xray uuid`, `xray version`, `xray run -c` (binds a real
VLESS inbound in-process), `caddy version`, `caddy list-modules` (shows
`layer4` + `dns.providers.cloudflare/alidns`), and both symlink forms.

## Supervisor call ordering (`agentmode.RunEmbedded`)

```
runAgent (supervisor.go)
  └─ config.Load(account-agent.yaml)          # + env overrides
  └─ if xray.embedded || caddy.embedded → RunEmbedded, else legacy Run
        │
        ├─ NewClient(controllerUrl, token)     # controller HTTP client
        │
        ├─ goroutine A · Caddy
        │     caddyembed.BuildStandaloneConfig(domain, dnsProvider, …)
        │     caddyembed.Apply(cfg)            # ACME/TLS live before Xray
        │     <-ctx.Done() → caddyembed.Stop()
        │
        ├─ goroutine B · Xray sync loop
        │     buildXrayTargets()               # one Runner+Generator per target
        │     loop every syncInterval:
        │        source.ListClients(ctx)        # 1st call == node registration
        │        for each target:
        │           gen.Render(clients)         # JSON in memory (no disk)
        │           runner.Apply(bytes)         # validate → swap core.Instance
        │        tracker.MarkSuccess()
        │
        └─ goroutine C · status reporter
              client.ReportStatus() every statusInterval
```

Node registration is implicit: the first `ListClients` against
`/api/agent-server/v1/users` (Bearer + `X-Service-Token` + `X-Agent-ID`)
announces the node and returns its inventory. Status heartbeats to
`/api/agent-server/v1/status` carry `NodeID/Region/LineCode/PricingGroup`.
This contract is unchanged from the legacy runtime.

## Hot-swap semantics (`xrayembed.Runner.Apply`)

`core.New(cfg)` builds Xray's features **without binding listeners**, so a
malformed config is rejected before the running instance is touched. Only
after the new instance validates do we `Close()` the old one (releasing its
socket) and `Start()` the new one. Not hitless — in-flight connections drop
during the swap, same as the `systemctl restart` it replaces — but a bad
config can never take down a healthy node, and there is no fork/exec.

## Config surface (new)

```yaml
xray:
  embedded: true        # run inbounds as in-process core.Instances
  sync:
    targets: [ … ]       # unchanged; ValidateCommand/RestartCommand ignored

caddy:
  embedded: true
  dnsProvider: cloudflare   # or alidns
  dnsApiToken: "…"          # cloudflare
  # aliKeyId / aliKeySecret # alidns
  xhttpSocket: /dev/shm/xray.sock
```

If neither `xray.embedded` nor `caddy.embedded` is set, the supervisor uses
the legacy external-process path (`agentmode.Run`) unchanged.

## Deferred (explicitly out of scope for this branch)

- Runtime-mode split (standalone/VPS vs Cloud Run vs Cloudflare Containers).
  `caddyembed.BuildStandaloneConfig` currently assumes the standalone/VPS
  topology (Caddy owns ACME). Cloud Run / CF terminate TLS at their edge and
  would skip Caddy entirely — to be modelled as a mode switch later.
- Release pipeline change: drop the separate `xcaddy build` step and ship the
  single `agent-svc-plus` binary + `xray`/`caddy` symlinks.
```
