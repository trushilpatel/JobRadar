package google

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/workers"
	"github.com/trushilpatel/jobradar/common/logger"
)

// CrawlerService is the per-adapter worker for Google Careers.
// Delegates the claim → scan → complete loop to the BaseWorker; the actual
// HTTP-based scan is performed by GoogleAdapter (see http.go).
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
func (s *CrawlerService) Run(ctx context.Context) {
	s.RunLoop(ctx)
}
