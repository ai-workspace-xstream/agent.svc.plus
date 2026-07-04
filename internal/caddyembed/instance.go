// Package caddyembed runs Caddy as an in-process library instead of the
// separately xcaddy-built "caddy" binary that agent.svc.plus's release
// pipeline used to produce and ship alongside agent-svc-plus. Caddy is
// explicitly designed to be embedded this way (it is what xcaddy itself
// generates a main package to do); the plugin set below matches the one
// previously passed to `xcaddy build` in
// .github/workflows/build-release-artifacts.yml.
package caddyembed

import (
	"encoding/json"
	"fmt"

	"github.com/caddyserver/caddy/v2"

	// Plugins. Keep this list in sync with the historical xcaddy build
	// flags: --with github.com/caddy-dns/cloudflare
	//        --with github.com/caddy-dns/alidns
	//        --with github.com/mholt/caddy-l4
	_ "github.com/caddy-dns/alidns"
	_ "github.com/caddy-dns/cloudflare"
	_ "github.com/caddyserver/caddy/v2/modules/standard"
	_ "github.com/mholt/caddy-l4"
)

// Apply loads (or hot-reloads) cfg into the single process-wide Caddy
// instance. Caddy's own config diffing decides what actually needs to
// restart (e.g. an unchanged TLS automation policy won't force a new ACME
// handshake), so this is safe to call on every sync tick.
func Apply(cfg map[string]any) error {
	buf, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("caddyembed: encode config: %w", err)
	}
	if err := caddy.Load(buf, true); err != nil {
		return fmt.Errorf("caddyembed: load config: %w", err)
	}
	return nil
}

// Stop shuts down the process-wide Caddy instance.
func Stop() error {
	return caddy.Stop()
}

// Version returns the linked Caddy version, e.g. for status reporting.
func Version() string {
	_, full := caddy.Version()
	if full == "" {
		return "unknown"
	}
	return full
}
