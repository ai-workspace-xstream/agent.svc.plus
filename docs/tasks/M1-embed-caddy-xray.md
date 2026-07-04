# M1 — Embed Caddy + Xray as libraries (monolith binary)

Status: **landed** on `feature/monolith-caddy-xray-embed` (PR #5, CI green).
This doc records what shipped and the **hardening items** still open before M1
can be called production-ready.

## Goal

One statically-linked `agent-svc-plus` binary that runs Caddy + Xray in-process
(two goroutines) and can also act as `xray`/`caddy` via argv[0] or subcommand,
replacing the three-binary + systemd orchestration from `scripts/setup-proxy.sh`.

## What shipped (grounded in code)

- `internal/xrayembed/runner.go:Runner.Apply` — parse → `core.New` (validate
  without binding) → close old → `Start` new. Hot-swap, fail-safe.
- `internal/xrayembed/cli.go:RunCLI` — `xray run/uuid/version/...` passthrough.
- `internal/caddyembed/instance.go:Apply/Stop/Version` — `caddy.Load`.
- `internal/caddyembed/config.go:BuildStandaloneConfig` — DNS-01 ACME + `/split`
  reverse-proxy to the xhttp unix socket.
- `cmd/agent/main.go` — multicall dispatch (argv[0] + subcommand).
- `internal/agentmode/embedded.go:RunEmbedded` — goroutines A (Caddy), B (Xray
  sync loop), C (status reporter).
- Config toggles `xray.embedded` / `caddy.embedded` in `internal/config/config.go`.
- CI ships one binary + `xray`/`caddy` symlinks
  (`.github/workflows/build-release-artifacts.yml`).
- Test: `internal/agentmode/embedded_test.go:TestEmbeddedSyncStartsXray`.

## Hardening items (open)

### H1 — tcp-vision inbound needs Caddy's cert files, embedded path doesn't wire them
`config/xray.tcp.template.json` reads cert/key from
`/var/lib/caddy/.local/share/caddy/certificates/.../{domain}.crt`. When Caddy
runs **embedded**, certmagic still writes to that storage path by default, so
the file paths *should* line up — but this is unverified end-to-end.
- **Do:** add an integration check that after `caddyembed.Apply` obtains a cert
  (staging ACME or a local CA), the tcp template's `certificateFile`/`keyFile`
  paths resolve. Document the storage path Caddy uses when embedded (it is
  `$XDG_DATA_HOME/caddy` or `$HOME/.local/share/caddy`; pin it via the
  `storage` key in `BuildStandaloneConfig` so it is deterministic regardless of
  the service user's `$HOME`).
- **Edit:** add a `"storage": {"module":"file_system","root":"/var/lib/caddy"}`
  block to the config in `config.go` so it matches the template's hardcoded path.

### H2 — graceful shutdown ordering
`RunEmbedded` closes Xray runners via `defer` and stops Caddy in goroutine A on
`ctx.Done()`. Order on shutdown is not guaranteed. Prefer: stop accepting new
(Caddy) → drain → close Xray. Low severity (process is exiting) but make it
explicit and logged.

### H3 — status report should reflect embedded versions & per-runner health
`internal/agentmode/runner.go:buildStatusReport` derives `running` from sync
recency. With embedded runners we have ground truth: `Runner.IsRunning()`.
- **Do:** thread `map[string]*xrayembed.Runner` into the reporter and set
  `XrayStatus.Running` from actual instance state; populate `ConfigHash` from
  the rendered bytes (sha256) so the controller can detect drift.

### H4 — `setup-proxy.sh` / ansible still install 3 binaries + 4 units
Installer and `deploy/ansible/roles/agent_svc_plus` predate the monolith.
- **Do (follow-up, can be its own task):** add an `--embedded` install path that
  drops one binary + `xray`/`caddy` symlinks and a single `agent-svc-plus.service`,
  and stops enabling `xray.service`/`xray-tcp.service`/`caddy.service`. Keep the
  legacy path until M3 mode-switch lands.

### H5 — xray-core is not a stable embedding API
`core.New`/`serial.LoadJSONConfig` are internal-ish; a version bump can break.
- **Do:** pin `github.com/xtls/xray-core` exactly (already `v1.260327.0`), add a
  build-tag smoke test (`go test ./internal/xrayembed -run TestApplyMinimal`) that
  fails loudly on API drift, and gate dependency bumps on it.

## Acceptance (M1 "done-done")
- [ ] H1: embedded Caddy issues a cert and tcp-vision Xray reads it (integration test or documented manual run).
- [ ] H3: `agent-svc-plus -v` and `/status` report linked caddy+xray versions and real per-runner `running`.
- [ ] `go test ./...` green; `CGO_ENABLED=0` cross-build amd64+arm64 static (already in CI).
- [ ] One-node manual soak: client connects via xhttp **and** tcp-vision through the single binary; config sync add/remove of a UUID takes effect within one `syncInterval`.
