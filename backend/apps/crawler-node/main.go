package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters"
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

	// adapterHTTPClient is the HTTP client passed to crawler-v2 adapters; its
	// timeout caps a single adapter fetch. Separate from the orchestrator client.
	adapterHTTPClient := &http.Client{
		Timeout: time.Duration(cfg.AdapterTimeoutSeconds) * time.Second,
	}

	client := newHTTPClient(cfg)
	exec := newAdapterExecutor(client, adapters.New, adapterHTTPClient, log)
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
