package agentmode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"agent.svc.plus/internal/caddyembed"
	"agent.svc.plus/internal/config"
	"agent.svc.plus/internal/xrayconfig"
	"agent.svc.plus/internal/xrayembed"
)

// RunEmbedded is the monolithic supervisor: a single process that launches
// Caddy and Xray as in-process libraries — no exec.Command, no systemctl, no
// second binary. It is the concrete "one main.go, two goroutines" runtime.
//
// Two long-lived goroutines run under one signal-scoped context:
//
//	goroutine A (Caddy)  — loads the TLS/ACME + reverse-proxy config once and
//	                       lets Caddy manage its own listeners/cert renewal;
//	                       it is torn down via caddyembed.Stop on shutdown.
//	goroutine B (Xray)   — the sync loop: polls the controller for the client
//	                       set, renders each inbound's JSON in memory, and
//	                       hot-swaps it into the matching xrayembed.Runner.
//
// A third goroutine reports status back to the controller, exactly as in the
// legacy Run path. Because both dataplanes live in this process, a config
// fetch turns into a direct function call rather than a file write plus a
// service restart.
func RunEmbedded(ctx context.Context, opts Options) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	controllerURL := strings.TrimSpace(opts.Agent.ControllerURL)
	if controllerURL == "" {
		return errors.New("agent.controllerUrl is required")
	}
	token := strings.TrimSpace(opts.Agent.APIToken)
	if token == "" {
		return errors.New("agent.apiToken is required")
	}

	syncInterval := firstPositiveDuration(opts.Agent.SyncInterval, opts.Xray.Sync.Interval, 5*time.Minute)
	statusInterval := firstPositiveDuration(opts.Agent.StatusInterval, time.Minute)
	httpTimeout := firstPositiveDuration(opts.Agent.HTTPTimeout, 15*time.Second)

	// Controller client. The first ListClients doubles as node
	// registration + initial inventory.
	client, err := NewClient(controllerURL, token, ClientOptions{
		Timeout:            httpTimeout,
		InsecureSkipVerify: opts.Agent.TLS.InsecureSkipVerify,
		UserAgent:          buildUserAgent(opts.Agent.ID),
		AgentID:            opts.Agent.ID,
	})
	if err != nil {
		return err
	}

	tracker := newSyncTracker()
	source := NewHTTPClientSource(client, tracker)

	var wg sync.WaitGroup

	// ── goroutine A: Caddy ────────────────────────────────────────────────
	// Started before Xray so TLS certs exist by the time inbounds want them.
	if opts.Caddy.Embedded {
		caddyCfg, err := caddyembed.BuildStandaloneConfig(caddyembed.StandaloneOptions{
			Domain:       opts.Agent.Domain,
			DNSProvider:  opts.Caddy.DNSProvider,
			DNSAPIToken:  opts.Caddy.DNSAPIToken,
			AliKeyID:     opts.Caddy.AliKeyID,
			AliKeySecret: opts.Caddy.AliKeySecret,
			XHTTPSocket:  opts.Caddy.XHTTPSocket,
		})
		if err != nil {
			return fmt.Errorf("build caddy config: %w", err)
		}
		if err := caddyembed.Apply(caddyCfg); err != nil {
			return fmt.Errorf("start embedded caddy: %w", err)
		}
		logger.Info("embedded caddy started", "domain", opts.Agent.Domain, "version", caddyembed.Version())

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ctx.Done()
			if err := caddyembed.Stop(); err != nil {
				logger.Warn("embedded caddy shutdown", "err", err)
			}
		}()
	}

	// ── goroutine B: Xray sync loop ───────────────────────────────────────
	runners, generators, err := buildXrayTargets(opts, logger)
	if err != nil {
		return err
	}
	defer func() {
		for _, r := range runners {
			_ = r.Close()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		runXraySyncLoop(ctx, source, runners, generators, tracker, syncInterval, logger)
	}()

	// ── goroutine C: status reporter ──────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		runStatusReporter(ctx, client, tracker, opts.Agent, statusInterval, syncInterval, logger)
	}()

	<-ctx.Done()
	wg.Wait()
	return nil
}

// buildXrayTargets constructs one in-process Xray runner + one config
// generator per sync target (e.g. "xhttp", "tcp").
func buildXrayTargets(opts Options, logger *slog.Logger) (map[string]*xrayembed.Runner, map[string]xrayconfig.Generator, error) {
	targets := resolveSyncTargets(opts.Xray.Sync)
	runners := make(map[string]*xrayembed.Runner, len(targets))
	generators := make(map[string]xrayconfig.Generator, len(targets))

	for _, target := range targets {
		name := strings.TrimSpace(target.Name)
		if name == "" {
			name = "default"
		}

		gen := xrayconfig.Generator{
			Definition: xrayconfig.DefaultDefinition(),
			// OutputPath is required by the generator for validation even
			// though embedded mode renders to memory, not disk.
			OutputPath: firstNonEmpty(target.OutputPath, "/dev/null"),
			Domain:     opts.Agent.Domain,
		}
		if tmplPath := strings.TrimSpace(target.TemplatePath); tmplPath != "" {
			payload, err := os.ReadFile(tmplPath)
			if err != nil {
				return nil, nil, fmt.Errorf("load xray template %s: %w", tmplPath, err)
			}
			gen.Definition = xrayconfig.JSONDefinition{Raw: payload}
		}

		runners[name] = xrayembed.New(name)
		generators[name] = gen
		logger.Info("registered embedded xray target", "target", name)
	}
	return runners, generators, nil
}

func runXraySyncLoop(ctx context.Context, source xrayconfig.ClientSource, runners map[string]*xrayembed.Runner, generators map[string]xrayconfig.Generator, tracker *syncTracker, interval time.Duration, logger *slog.Logger) {
	apply := func() {
		clients, err := source.ListClients(ctx)
		if err != nil {
			tracker.MarkError(fmt.Errorf("list clients: %w", err), time.Now().UTC())
			if ctx.Err() == nil {
				logger.Error("xray sync: list clients failed", "err", err)
			}
			return
		}

		for name, gen := range generators {
			rendered, err := gen.Render(clients)
			if err != nil {
				tracker.MarkError(fmt.Errorf("render %s: %w", name, err), time.Now().UTC())
				logger.Error("xray sync: render failed", "target", name, "err", err)
				return
			}
			if err := runners[name].Apply(rendered); err != nil {
				tracker.MarkError(err, time.Now().UTC())
				logger.Error("xray sync: apply failed", "target", name, "err", err)
				return
			}
		}
		tracker.MarkSuccess(time.Now().UTC())
		logger.Info("embedded xray synchronized", "clients", len(clients))
	}

	apply()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			apply()
		}
	}
}

func resolveSyncTargets(sync config.XraySync) []config.SyncTarget {
	if len(sync.Targets) > 0 {
		return sync.Targets
	}
	if sync.OutputPath != "" {
		return []config.SyncTarget{{
			Name:         "default",
			OutputPath:   sync.OutputPath,
			TemplatePath: sync.TemplatePath,
		}}
	}
	return []config.SyncTarget{{Name: "default", OutputPath: "/usr/local/etc/xray/config.json"}}
}

func firstPositiveDuration(values ...time.Duration) time.Duration {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}
