# M4 — WireGuard access path (exploratory)

Status: **exploratory** — spec only, not yet committed to. Depends on M2 (so WG
traffic is accounted the same way as VLESS).

## Goal

Offer WireGuard as a parallel inbound to VLESS (per the platform diagram's
"Future Protocols" / WireGuard branch), managed under the **same tenant model**:
one tenant ↔ one credential ↔ one accounting bucket, provisioned by the same
controller poll-to-converge loop.

## Why / positioning

- VLESS-xhttp is the MTU-safe, censorship-resistant path for app traffic.
- WireGuard suits full-tunnel / LAN-to-LAN ("Home LAN / Company LAN / Cloud VPC"
  in the diagram) where a kernel/userspace WG endpoint is preferable.
- Reusing the tenant + accounting plumbing avoids a second control plane.

## Open questions to resolve before committing

1. **Where does WG terminate?**
   - (a) `wireguard-go` embedded as another goroutine in the monolith
     (userspace, `CGO_ENABLED=0` friendly — check `golang.zx2c4.com/wireguard`),
     or
   - (b) kernel WG managed via `wg`/`wg-quick` (fast, but breaks the single-
     static-binary story and needs root/netlink).
   Lean (a) for consistency with the monolith thesis; benchmark throughput.
2. **Credential model.** WG uses per-peer public keys, not UUIDs. The controller
   `/users` contract is UUID+email today. Need a parallel field: per-tenant WG
   public key + allowed IPs. Decide whether to extend `ClientListResponse`
   (`internal/agentproto/types.go`) with an optional `WireguardPeers` block or
   add a sibling endpoint `/api/agent-server/v1/wg-peers`.
3. **Accounting.** Xray's StatsManager (M2) won't see WG traffic. Options:
   per-peer byte counters from the WG device (`wireguard-go` exposes
   `Device.IpcGet()` transfer stats; kernel WG via `wg show transfer`). Fold
   these into the M2 `Snapshot.Users` with a `Protocol` field so billing treats
   VLESS and WG uniformly.
4. **IP address management.** WG needs a per-tenant tunnel IP. Who allocates —
   controller (authoritative, like UUIDs) or node-local pool? Prefer controller.

## Rough shape (if we proceed)

- NEW `internal/wgembed/`: `Device` lifecycle (up/down), peer set apply
  (converge to controller list), transfer-stats read.
- Extend the sync loop: after Xray targets, converge WG peers from the same
  `ListClients` response (new field).
- Extend M2 collector: merge WG per-peer transfer into the snapshot with
  `Protocol: "wireguard"`.
- Config: `wireguard.enabled`, listen port, interface name/IP, MTU.

## Acceptance (when promoted from exploratory)
- [ ] Decision recorded for Q1–Q4 above.
- [ ] A tenant provisioned once appears on both VLESS and WG (if entitled) via one controller update.
- [ ] WG per-peer traffic shows up in `/v1/snapshots/latest` with `Protocol: "wireguard"`.
- [ ] Single static binary still builds for amd64+arm64 (if userspace WG chosen).

## Non-goals
- Replacing VLESS. WG is additive for full-tunnel/LAN use cases.
- Kernel-module distribution/packaging (unless (b) is explicitly chosen).
