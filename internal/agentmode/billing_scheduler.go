package agentmode

import (
	"context"
	"log/slog"
	"time"
)

type billingTriggerer interface {
	TriggerCollectAndRate(ctx context.Context) error
	TriggerReconcile(ctx context.Context) error
}

func startBillingSchedulers(ctx context.Context, client billingTriggerer, cfg billingScheduleConfig, logger *slog.Logger) {
	if client == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.collectInterval > 0 {
		go runBillingLoop(ctx, cfg.collectInterval, cfg.httpTimeout, logger.With("component", "billing-collect"), client.TriggerCollectAndRate)
	}
	if cfg.reconcileInterval > 0 {
		go runBillingLoop(ctx, cfg.reconcileInterval, cfg.httpTimeout, logger.With("component", "billing-reconcile"), client.TriggerReconcile)
	}
}

type billingScheduleConfig struct {
	httpTimeout       time.Duration
	collectInterval   time.Duration
	reconcileInterval time.Duration
}

func runBillingLoop(ctx context.Context, interval, timeout time.Duration, logger *slog.Logger, trigger func(context.Context) error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	invoke := func() {
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := trigger(runCtx); err != nil {
			logger.Warn("billing job trigger failed", "err", err)
			return
		}
		logger.Info("billing job trigger succeeded")
	}

	invoke()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			invoke()
		}
	}
}
