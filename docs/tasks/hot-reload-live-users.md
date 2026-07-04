# Research — Xray config hot-reload with minimal disconnection

Status: **IMPLEMENTED** in `internal/xrayembed/runner.go` (classify-then-reload)
+ `config/xray.{xhttp,tcp}.template.json` (added `"tag":"vless-in"`). Tests in
`internal/xrayembed/runner_test.go`. Relates to M1 (`xrayembed.Runner.Apply`)
and M2 (stats). Investigated against vendored `third_party/xray-core@v1.260327.0`.

## Problem

Today `internal/xrayembed/runner.go:Runner.Apply` reloads by **rebuilding the
whole `core.Instance`**: `core.New(next)` → `oldInstance.Close()` → `next.Start()`.
`Close()` releases the listener, so **every in-flight connection is dropped on
every sync tick that produces any config change** — even when the only change is
"one tenant was added". For a proxy carrying long-lived Claude-Code / streaming
sessions this is exactly the disruption we want to avoid.

The sync loop's dominant change is **client-set churn** (tenants added/removed
via the controller list). The insight: that specific change does *not* require
touching the listener at all.

## Taxonomy of config changes vs. required disruption

| Change | Disruption needed | Mechanism |
| --- | --- | --- |
| Add/remove tenant (UUID in `clients[]`) | **none** for unaffected users | live `AddUser`/`RemoveUser` (§A) |
| TLS certificate renewal | **none** | already hot-reloaded (§B) |
| Listen addr/port, transport (tcp↔xhttp), routing, fallbacks | full rebuild (drop) | current `Apply` (§C) |

## §A — Live user add/remove (verified, in-process, no rebind)

xray-core exposes per-user management on a **running** inbound. No gRPC needed
when embedded — we call it directly on the instance:

```go
im := inst.GetFeature(inbound.ManagerType()).(inbound.Manager) // features/inbound/inbound.go:27
h, _ := im.GetHandler(ctx, tag)                                 // needs a stable inbound tag
um := h.(proxy.GetInbound).GetInbound().(proxy.UserManager)      // proxy/proxy.go:77
um.AddUser(ctx, memoryUser)   // proxy/vless/inbound/inbound.go:240
um.RemoveUser(ctx, email)     // proxy/vless/inbound/inbound.go:245
```

- The gRPC `AddUserOperation.ApplyInbound` (`app/proxyman/command/command.go:38`)
  does exactly this cast+call; we replicate it in-process.
- **VLESS inbound implements `UserManager`** — confirmed at the lines above. So
  both the xhttp and tcp-vision inbounds support live user churn.
- Adding a user does **not** touch the listener or other sessions → zero
  disconnection. Removing a user stops them reconnecting and cuts their own live
  session (desired for suspend/cleanup).

Building the `*protocol.MemoryUser` for a VLESS client:
```go
account := &vless.Account{Id: uuid, Flow: flow /* "" for xhttp, xtls-rprx-vision for tcp */}
u := &protocol.User{Level: 0, Email: email, Account: serial.ToTypedMessage(account)}
mUser, err := u.ToMemoryUser()
```
(Mirror how the generator already decides flow per network in
`internal/xrayconfig/generator.go:updateClients` — xhttp gets no flow.)

## §B — TLS certificate renewal is already non-disruptive

`transport/internet/tls/config.go:98-107`: unless `OneTimeLoading` is set, Xray
**re-reads cert/key files on a ticker** (default 3600s, or the `ocspStapling`
interval). The tcp-vision template already sets `"ocspStapling": 3600`, so when
Caddy renews the on-disk cert, Xray picks it up within the hour with **no
restart**. For xhttp, TLS lives in Caddy (Xray listens plaintext on the unix
socket), so renewal never touches Xray at all.

**Action: none** — just never set `OneTimeLoading: true` on the tcp template.
Optionally lower the reload interval if faster cert pickup is wanted.

## §C — Structural changes still need a rebuild

Listen address, transport/network, routing rules, fallbacks, adding/removing an
entire inbound — these can't be done on a live handler and keep semantics. The
existing full-rebuild `Apply` remains the fallback. These changes are rare
(template edits, not tenant churn), so an occasional drop is acceptable.

`inbound.Manager` also supports `AddHandler`/`RemoveHandler` — adding a *new*
inbound doesn't disturb existing ones, so even "add a new listener" can be
partially non-disruptive later. Out of scope for v1.

## Prerequisite (blocking §A)

**Inbound handlers need a stable `tag`.** Current templates tag only the
*outbounds* (`direct`/`blocked`); the VLESS **inbound has no tag**
(`config/xray.xhttp.template.json`, `config/xray.tcp.template.json`).
`GetHandler(ctx, tag)` can't find an untagged inbound.
- **Edit:** add `"tag": "vless-in"` to the inbound in both templates.

## Proposed design — classify-then-reload

Replace the unconditional rebuild in `Runner.Apply` with a diff-driven path:

```
Apply(rendered):
  parse rendered → next
  if r.instance == nil:                       // first start
      full start (current path)
  else if onlyClientsChanged(prev, next):     // structural bytes identical except clients[]
      diff clients by email:
        for added:   um.AddUser(ctx, buildMemoryUser(c))
        for removed: um.RemoveUser(ctx, c.email)
      keep the same instance  → NO disconnection
  else:                                         // structural change
      full rebuild (current close/start)        → drop, but rare
  remember prev = next (or the client set)
```

`onlyClientsChanged` compares the two rendered configs with `clients[]` blanked
out (normalize both, compare remainder). Cheap and robust; avoids guessing which
fields are structural.

### Files
- `internal/xrayembed/runner.go` — add `applyLiveUserDiff`, `buildMemoryUser`,
  keep `prevClients`/`prevStructuralHash` on `Runner`; blank import already pulls
  `proxyman` + VLESS.
- `internal/xrayconfig` — expose the per-target `[]Client` alongside rendered
  bytes so the runner can diff without re-parsing JSON (or diff from parsed
  config — either works; parsing the rendered bytes keeps `xrayembed` self-contained).
- Templates — add inbound `tag`.

## Risks / decisions
- **Tag stability:** the tag must be constant across renders; hardcode it in the
  template, don't derive it from anything dynamic.
- **Stats counters on live add:** unlike a full rebuild (which zeroes all
  counters), live add/remove **preserves** existing users' counters — this is
  actually *better* for M2 billing (fewer resets). Note the interaction: prefer
  the live path partly for this reason.
- **RemoveUser cutting a live session:** acceptable/intended for suspend and
  cleanup. Confirm the controller's "pause" semantics expect immediate cutoff
  (vs. lame-duck). If lame-duck is wanted, that's a controller-side policy, not
  a node change.
- **Concurrency:** user-diff calls happen inside `Apply` under `Runner.mu`;
  ensure the sync loop never calls `Apply` concurrently (it doesn't — single
  loop per target).
- **Non-VLESS inbounds:** the `UserManager` cast fails for protocols that don't
  implement it; fall back to full rebuild in that case (guard the cast).

## Acceptance
- [ ] Templates carry a stable inbound `tag`; `GetHandler` resolves it.
- [ ] Unit test: start instance with 1 user; `Apply` a config adding a 2nd user;
      assert the `*core.Instance` pointer is unchanged (no rebuild) and both
      users authenticate.
- [ ] Unit test: removing a user via `Apply` calls `RemoveUser` (no rebuild).
- [ ] Unit test: a structural change (different listen port) *does* rebuild.
- [ ] Manual soak: hold a long-lived download through the node; add and remove
      an unrelated tenant; the download is uninterrupted.
- [ ] Cert renewal (staging ACME) picked up by tcp-vision without restart.
