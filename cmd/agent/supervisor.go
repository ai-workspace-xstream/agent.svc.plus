package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"agent.svc.plus/internal/agentmode"
	"agent.svc.plus/internal/config"
)

// runAgent is the supervisor personality: the long-lived process that owns
// the node. It replaces the old three-process layout (systemd caddy.service
// + xray.service + agent-svc-plus.service) with one process that embeds
// Caddy and Xray as libraries and drives them from the same control loop.
//
// Call ordering (see agentmode.Run for the loop internals):
//
//  1. Load config (account-agent.yaml + env overrides).
//  2. Register/announce the node to the controller and fetch the initial
//     client set (first ListClients acts as registration + inventory).
//  3. Start embedded Caddy so TLS/ACME is live before Xray needs the certs.
//  4. Render + start embedded Xray instances (xhttp, tcp) in-process.
//  5. Enter the periodic sync + status-report loop until a signal arrives.
//
// Steps 3–5 are orchestrated inside agentmode.Run; this function is the thin
// entrypoint that prepares options and owns the signal-scoped context.
func runAgent() {
	configPath := flag.String("config", "account-agent.yaml", "path to configuration file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load configuration", "path", *configPath, "err", err)
		os.Exit(1)
	}

	if cfg.Mode != "agent" {
		logger.Error("invalid run mode", "expected", "agent", "got", cfg.Mode)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("starting agent supervisor", "id", cfg.Agent.ID)

	opts := agentmode.Options{
		Logger:  logger,
		Agent:   cfg.Agent,
		Xray:    cfg.Xray,
		Caddy:   cfg.Caddy,
		Billing: cfg.Billing,
	}

	// Embedded runtime: run Caddy + Xray as in-process libraries (the
	// monolith). Otherwise fall back to the legacy path that writes config
	// files and drives external xray/caddy services.
	run := agentmode.Run
	if cfg.Xray.Embedded || cfg.Caddy.Embedded {
		logger.Info("using embedded (monolithic) runtime")
		run = agentmode.RunEmbedded
	}

	if err := run(ctx, opts); err != nil {
		logger.Error("agent runtime failed", "err", err)
		os.Exit(1)
	}

	logger.Info("agent supervisor shutdown complete")
}
