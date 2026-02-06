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

func main() {
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

	logger.Info("starting agent", "id", cfg.Agent.ID)

	opts := agentmode.Options{
		Logger: logger,
		Agent:  cfg.Agent,
		Xray:   cfg.Xray,
	}

	if err := agentmode.Run(ctx, opts); err != nil {
		logger.Error("agent runtime failed", "err", err)
		os.Exit(1)
	}

	logger.Info("agent shutdown complete")
}
