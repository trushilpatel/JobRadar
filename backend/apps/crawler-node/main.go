package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	ats "github.com/trushilpatel/jobradar/apps/crawler/ats"
	googleats "github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/tracing"
)

func main() {
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "crawler-node: %v\n", err)
		os.Exit(1)
	}

	log := logger.Init("crawler-node")

	shutdown := tracing.Init("jobradar-crawler-node", "", log)
	defer shutdown()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	googleCfg := googleats.Config{
		MinDelayMs: cfg.AdapterTimeoutSeconds * 33, // sensible defaults derived from timeout
		MaxDelayMs: cfg.AdapterTimeoutSeconds * 67,
		TimeoutMs:  cfg.AdapterTimeoutSeconds * 1000,
		MaxPages:   0,
	}

	client := newHTTPClient(cfg)
	exec := newAdapterExecutor(client, ats.RegistryWithGoogleConfig, googleCfg, log)
	loop := NewPollLoop(cfg, client, exec)
	loop.log = log

	log.Info("crawler-node starting",
		logger.Str("orchestrator_url", cfg.OrchestratorURL),
		logger.Int("max_concurrent", cfg.MaxConcurrent),
		logger.Int("poll_interval_seconds", cfg.PollIntervalSeconds),
	)

	// Run blocks until ctx is cancelled (signal) and all in-flight workers drain.
	loop.Run(ctx)
	log.Info("crawler-node shut down cleanly")
}
