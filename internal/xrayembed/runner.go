// Package xrayembed runs Xray as an in-process core.Instance instead of a
// separate binary/subprocess. It exists so agent-svc-plus can ship as one
// monolithic executable: the same process that syncs client config from the
// controller also terminates VLESS traffic, with no exec.Command/systemctl
// hop in between.
package xrayembed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/inbound"
	confserial "github.com/xtls/xray-core/infra/conf/serial" // xray json → *core.Config
	xproxy "github.com/xtls/xray-core/proxy"
	"github.com/xtls/xray-core/proxy/vless"

	// Blank import registers every inbound/outbound/transport Xray ships
	// with (VLESS, XHTTP, TLS, REALITY, stats, router, ...). Without it
	// core.New fails on "unknown config type" for anything beyond the
	// bare minimum.
	_ "github.com/xtls/xray-core/main/distro/all"
)

// Runner supervises a single logical Xray listener (e.g. "xhttp" or "tcp")
// as an in-process core.Instance.
//
// It supports two reload strategies, chosen automatically per Apply:
//
//   - Live user reload (no disconnection): when only the VLESS client set
//     changed, users are added/removed on the *running* inbound via
//     proxy.UserManager. The listener is never rebound, so in-flight
//     connections for unaffected users are untouched.
//   - Full rebuild (drops connections): when anything structural changed
//     (listen address, transport, routing, ...) or when the live path is not
//     applicable, the whole instance is rebuilt. This is the fallback and is
//     always correct.
//
// See docs/tasks/hot-reload-live-users.md for the investigation behind this.
type Runner struct {
	name   string
	logger *slog.Logger

	mu       sync.Mutex
	instance *core.Instance

	// inboundTag is the tag of the user-managed VLESS inbound in the current
	// config. Empty means the live path is unavailable (no tagged VLESS
	// inbound) and every reload is a full rebuild.
	inboundTag string
	// prevClients is the last-applied client set, keyed by email.
	prevClients map[string]clientEntry
	// structHash fingerprints the current config with all client lists
	// blanked, so client-only churn does not change it.
	structHash string
}

type clientEntry struct {
	id    string
	email string
	flow  string
}

// New creates a Runner. name is used only for logging/status and should
// match the sync target name (e.g. "xhttp", "tcp").
func New(name string) *Runner {
	return &Runner{name: name, logger: slog.Default().With("component", "xrayembed", "target", name)}
}

// Apply loads jsonConfig, choosing a live user reload when only the client set
// changed and a full rebuild otherwise. It is safe to call on every sync tick.
func (r *Runner) Apply(jsonConfig []byte) error {
	root, err := parseConfig(jsonConfig)
	if err != nil {
		return fmt.Errorf("xrayembed[%s]: parse config: %w", r.name, err)
	}
	tag, clients := extractVLESSInbound(root)
	structHash, err := structuralHash(root)
	if err != nil {
		return fmt.Errorf("xrayembed[%s]: fingerprint config: %w", r.name, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// First start.
	if r.instance == nil {
		return r.startLocked(jsonConfig, tag, clients, structHash)
	}

	// Live path: same structure and a known, user-managed inbound tag.
	if tag != "" && tag == r.inboundTag && structHash == r.structHash {
		if err := r.applyUserDiffLocked(clients); err != nil {
			r.logger.Warn("live user reload failed, falling back to full rebuild", "err", err)
		} else {
			r.prevClients = clients
			r.logger.Debug("live user reload applied", "clients", len(clients))
			return nil
		}
	}

	// Fallback: full rebuild (drops in-flight connections).
	return r.rebuildLocked(jsonConfig, tag, clients, structHash)
}

// startLocked builds and starts a fresh instance. Caller holds r.mu.
func (r *Runner) startLocked(jsonConfig []byte, tag string, clients map[string]clientEntry, structHash string) error {
	next, err := buildInstance(jsonConfig)
	if err != nil {
		return fmt.Errorf("xrayembed[%s]: %w", r.name, err)
	}
	if err := next.Start(); err != nil {
		return fmt.Errorf("xrayembed[%s]: start instance: %w", r.name, err)
	}
	r.instance = next
	r.inboundTag = tag
	r.prevClients = clients
	r.structHash = structHash
	return nil
}

// rebuildLocked validates the new config, then swaps it in for the running
// instance. Caller holds r.mu.
//
// Ordering matters: buildInstance (core.New) constructs Xray's features
// without binding any listener, so a malformed config is rejected before the
// previous instance is touched. Only after the new instance is known valid do
// we Close the old one (releasing its listen address/socket) and Start the new
// one. A bad config can never take down a healthy running node.
func (r *Runner) rebuildLocked(jsonConfig []byte, tag string, clients map[string]clientEntry, structHash string) error {
	next, err := buildInstance(jsonConfig)
	if err != nil {
		return fmt.Errorf("xrayembed[%s]: %w", r.name, err)
	}
	if r.instance != nil {
		_ = r.instance.Close()
		r.instance = nil
	}
	if err := next.Start(); err != nil {
		return fmt.Errorf("xrayembed[%s]: start instance: %w", r.name, err)
	}
	r.instance = next
	r.inboundTag = tag
	r.prevClients = clients
	r.structHash = structHash
	return nil
}

// applyUserDiffLocked adds/removes users on the running inbound to match next.
// Caller holds r.mu. Returns an error (so Apply can fall back to a rebuild) if
// the inbound cannot be managed live.
func (r *Runner) applyUserDiffLocked(next map[string]clientEntry) error {
	im, ok := r.instance.GetFeature(inbound.ManagerType()).(inbound.Manager)
	if !ok || im == nil {
		return fmt.Errorf("no inbound manager")
	}
	handler, err := im.GetHandler(context.Background(), r.inboundTag)
	if err != nil {
		return fmt.Errorf("get handler %q: %w", r.inboundTag, err)
	}
	gi, ok := handler.(xproxy.GetInbound)
	if !ok {
		return fmt.Errorf("handler %q is not a GetInbound", r.inboundTag)
	}
	um, ok := gi.GetInbound().(xproxy.UserManager)
	if !ok {
		return fmt.Errorf("inbound %q is not a UserManager", r.inboundTag)
	}

	ctx := context.Background()

	// Removals: present before, absent (or changed) now.
	for email, old := range r.prevClients {
		cur, keep := next[email]
		if keep && cur == old {
			continue
		}
		if err := um.RemoveUser(ctx, email); err != nil {
			return fmt.Errorf("remove user %q: %w", email, err)
		}
	}

	// Additions: present now, absent before or changed (already removed above).
	for email, c := range next {
		old, existed := r.prevClients[email]
		if existed && old == c {
			continue
		}
		mUser, err := buildMemoryUser(c)
		if err != nil {
			return fmt.Errorf("build user %q: %w", email, err)
		}
		if err := um.AddUser(ctx, mUser); err != nil {
			return fmt.Errorf("add user %q: %w", email, err)
		}
	}
	return nil
}

// buildMemoryUser constructs a VLESS MemoryUser identical to what the config
// loader produces for an inbound client (infra/conf/vless.go): account carries
// only Id + Flow; Encryption must stay empty for inbound clients.
func buildMemoryUser(c clientEntry) (*protocol.MemoryUser, error) {
	if c.id == "" {
		return nil, fmt.Errorf("client id is required")
	}
	if c.email == "" {
		return nil, fmt.Errorf("client email is required")
	}
	acct := &vless.Account{Id: c.id, Flow: c.flow}
	u := &protocol.User{Level: 0, Email: c.email, Account: serial.ToTypedMessage(acct)}
	return u.ToMemoryUser()
}

// buildInstance parses jsonConfig and constructs (but does not start) an
// instance.
func buildInstance(jsonConfig []byte) (*core.Instance, error) {
	cfg, err := confserial.LoadJSONConfig(bytes.NewReader(jsonConfig))
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	inst, err := core.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("build instance: %w", err)
	}
	return inst, nil
}

// IsRunning reports whether the current instance is up.
func (r *Runner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.instance != nil && r.instance.IsRunning()
}

// Close stops the current instance, if any.
func (r *Runner) Close() error {
	r.mu.Lock()
	inst := r.instance
	r.instance = nil
	r.mu.Unlock()

	if inst == nil {
		return nil
	}
	return inst.Close()
}

// Version returns the linked Xray-core version, e.g. for status reporting.
func Version() string {
	v := core.Version()
	if v == "" {
		return "unknown"
	}
	return v
}

// ── config introspection ──────────────────────────────────────────────────

func parseConfig(jsonConfig []byte) (map[string]any, error) {
	var root map[string]any
	if err := json.Unmarshal(jsonConfig, &root); err != nil {
		return nil, err
	}
	return root, nil
}

// extractVLESSInbound returns the tag and client set of the first tagged VLESS
// inbound. Tag is empty when no such inbound exists, which forces the
// full-rebuild path (backward compatible with untagged templates).
func extractVLESSInbound(root map[string]any) (string, map[string]clientEntry) {
	inbounds, _ := root["inbounds"].([]any)
	for _, raw := range inbounds {
		inb, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if proto, _ := inb["protocol"].(string); proto != "vless" {
			continue
		}
		tag, _ := inb["tag"].(string)
		if tag == "" {
			continue
		}
		settings, _ := inb["settings"].(map[string]any)
		rawClients, _ := settings["clients"].([]any)
		clients := make(map[string]clientEntry, len(rawClients))
		for _, rc := range rawClients {
			cm, ok := rc.(map[string]any)
			if !ok {
				continue
			}
			email, _ := cm["email"].(string)
			if email == "" {
				// Without a stable email key we cannot diff/remove by email;
				// signal "unavailable" so Apply rebuilds instead.
				return "", nil
			}
			id, _ := cm["id"].(string)
			flow, _ := cm["flow"].(string)
			clients[email] = clientEntry{id: id, email: email, flow: flow}
		}
		return tag, clients
	}
	return "", nil
}

// structuralHash fingerprints the config with every inbound's client list
// blanked, so adding/removing clients does not change the hash but any
// structural change (listen, transport, routing, fallbacks, ...) does.
func structuralHash(root map[string]any) (string, error) {
	// Deep copy via round-trip so we don't mutate the caller's map.
	buf, err := json.Marshal(root)
	if err != nil {
		return "", err
	}
	var clone map[string]any
	if err := json.Unmarshal(buf, &clone); err != nil {
		return "", err
	}
	if inbounds, ok := clone["inbounds"].([]any); ok {
		for _, raw := range inbounds {
			inb, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if settings, ok := inb["settings"].(map[string]any); ok {
				if _, has := settings["clients"]; has {
					settings["clients"] = []any{}
				}
			}
		}
	}
	normalized, err := json.Marshal(clone)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(normalized)
	return hex.EncodeToString(sum[:]), nil
}
