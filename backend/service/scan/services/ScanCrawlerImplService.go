package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/tracing"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// CrawlerService is the interface the crawler engine depends on for recording scan outcomes.
type CrawlerService interface {
	// RecordScanResult writes the scan log entry and updates the company's scan status.
	// companyID is the company being scanned.
	// result carries the counts and any error message from the crawl.
	RecordScanResult(ctx context.Context, companyID string, result dto.ScanResult) error
}

type crawlerService struct {
	log ScanLogWriter // writes company_scan_log rows + updates company status atomically
}

// NewCrawlerService constructs a CrawlerService.
func NewCrawlerService(log ScanLogWriter) CrawlerService {
	return &crawlerService{log: log}
}

// RecordScanResult atomically writes the scan log entry and updates the company's
// last_scan_at / last_scan_status in a single transaction.
func (c *crawlerService) RecordScanResult(ctx context.Context, companyID string, result dto.ScanResult) (err error) {
	ctx, span := tracing.Start(ctx, "scan.crawlerservice.RecordScanResult")
	defer tracing.End(span, &err)
	status := scancommon.ScanStatusOK
	var errPtr *string
	if result.ErrMsg != "" {
		status = scancommon.ScanStatusError
		errPtr = &result.ErrMsg
	}
	durationMs := int(result.Duration.Milliseconds())
	return c.log.RecordScanResult(ctx, companyID, status, result.FetchedCount, result.NewCount, durationMs, errPtr)
}
