package apple

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/workers"
	"github.com/trushilpatel/jobradar/common/logger"
)

// CrawlerService is the per-adapter worker for the Apple ATS.
type CrawlerService struct {
	workers.BaseWorker
}

// NewCrawlerService constructs an AppleCrawlerService.
func NewCrawlerService(q workers.CrawlerQueueService, s workers.Scanner, log logger.Logger) *CrawlerService {
	return &CrawlerService{
		BaseWorker: workers.NewBaseWorker("apple", q, s, log, 30*time.Second),
	}
}

// AdapterType returns the adapter type string for this worker.
func (s *CrawlerService) AdapterType() string { return "apple" }

// Run starts the claim → scan → complete loop. Blocks until ctx is cancelled.
func (s *CrawlerService) Run(ctx context.Context) { s.RunLoop(ctx) }
