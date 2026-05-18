package workday

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/workers"
	"github.com/trushilpatel/jobradar/common/logger"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
)

// CrawlerService is the per-adapter worker for Workday-hosted careers boards.
type CrawlerService struct {
	workers.BaseWorker
}

// NewCrawlerService constructs a Workday CrawlerService.
func NewCrawlerService(q workers.CrawlerQueueService, s workers.Scanner, log logger.Logger) *CrawlerService {
	return &CrawlerService{
		BaseWorker: workers.NewBaseWorker(companycommon.AdapterWorkday, q, s, log, 30*time.Second),
	}
}

// AdapterType returns the adapter type string for this worker.
func (s *CrawlerService) AdapterType() string { return companycommon.AdapterWorkday }

// Run starts the claim → scan → complete loop. Blocks until ctx is cancelled.
func (s *CrawlerService) Run(ctx context.Context) { s.RunLoop(ctx) }
