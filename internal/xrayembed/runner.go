// Package xrayembed runs Xray as an in-process core.Instance instead of a
// separate binary/subprocess. It exists so agent-svc-plus can ship as one
// monolithic executable: the same process that syncs client config from the
// controller also terminates VLESS traffic, with no exec.Command/systemctl
// hop in between.
package xrayembed

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"

	// Blank import registers every inbound/outbound/transport Xray ships
	// with (VLESS, XHTTP, TLS, REALITY, stats, router, ...). Without it
	// core.New fails on "unknown config type" for anything beyond the
	// bare minimum.
	_ "github.com/xtls/xray-core/main/distro/all"
)

// Runner supervises a single logical Xray listener (e.g. "xhttp" or "tcp")
// as an in-process core.Instance and swaps it in place when the rendered
// config changes.
type Runner struct {
	name string

	mu       sync.Mutex
	instance *core.Instance
}

// New creates a Runner. name is used only for logging/status and should
// match the sync target name (e.g. "xhttp", "tcp").
func New(name string) *Runner {
	return &Runner{name: name}
}

// Apply parses jsonConfig and hot-swaps it in for the currently running
// instance, if any.
//
// Ordering matters: core.New builds Xray's features (router, stats,
// inbound/outbound handlers) without binding any listener, so a malformed
// config is rejected here before the previous instance is touched. Only
// after the new instance is known to be structurally valid do we Close the
// old one (releasing its listen address/socket) and Start the new one
// (binding it). This is not a hitless reload — in-flight connections on the
// old instance are dropped during the swap, same as the systemctl restart
// this replaces — but it is a same-process function call instead of a
// fork/exec + service-manager round trip, and a bad config can never take
// down a healthy running node.
func (r *Runner) Apply(jsonConfig []byte) error {
	cfg, err := serial.LoadJSONConfig(bytes.NewReader(jsonConfig))
	if err != nil {
		return fmt.Errorf("xrayembed[%s]: parse config: %w", r.name, err)
	}

	next, err := core.New(cfg)
	if err != nil {
		return fmt.Errorf("xrayembed[%s]: build instance: %w", r.name, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.instance != nil {
		_ = r.instance.Close()
		r.instance = nil
	}

	if err := next.Start(); err != nil {
		return fmt.Errorf("xrayembed[%s]: start instance: %w", r.name, err)
	}
	r.instance = next
	return nil
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
