package meta

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/workers"
	"github.com/trushilpatel/jobradar/common/logger"
)

// CrawlerService is the per-adapter worker for Meta Careers.
// Meta uses a Node.js Playwright subprocess — the subprocess is launched fresh
// per scan (no persistent process to keep warm), so this follows the same
// simple RunLoop pattern as API adapters.
type CrawlerService struct {
	workers.BaseWorker
}

// NewCrawlerService constructs a MetaCrawlerService.
func NewCrawlerService(q workers.CrawlerQueueService, s workers.Scanner, log logger.Logger) *CrawlerService {
	return &CrawlerService{
		BaseWorker: workers.NewBaseWorker("meta", q, s, log, 30*time.Second),
	}
}

// AdapterType returns the adapter type string for this worker.
func (s *CrawlerService) AdapterType() string { return "meta" }

// Run starts the claim → scan → complete loop. Blocks until ctx is cancelled.
func (s *CrawlerService) Run(ctx context.Context) { s.RunLoop(ctx) }
