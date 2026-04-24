package google

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/workers"
	"github.com/trushilpatel/jobradar/common/logger"
)

// CrawlerService is the per-adapter worker for Google Careers.
// It keeps warm-browser handling at the Run level — relaunching Chromium only
// on crash rather than per scan. Phase 1: delegates directly to RunLoop
// (engine creates its own browser per scan via RodGoogle.FetchJobs).
// Phase 2 can inject a shared browser instance into the engine.
type CrawlerService struct {
	workers.BaseWorker
	cfg Config
	log logger.Logger
}

// NewCrawlerService constructs a GoogleCrawlerService.
func NewCrawlerService(q workers.CrawlerQueueService, s workers.Scanner, cfg Config, log logger.Logger) *CrawlerService {
	return &CrawlerService{
		BaseWorker: workers.NewBaseWorker("google", q, s, log, 30*time.Second),
		cfg:        cfg,
		log:        log,
	}
}

// AdapterType returns the adapter type string for this worker.
func (s *CrawlerService) AdapterType() string { return "google" }

// Run starts the claim → scan → complete loop. Blocks until ctx is cancelled.
// Phase 1: delegates directly to RunLoop. Phase 2 will keep a browser warm here.
func (s *CrawlerService) Run(ctx context.Context) {
	s.RunLoop(ctx)
}
