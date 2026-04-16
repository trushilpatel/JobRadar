package scan

import (
	"time"

	"github.com/trushilpatel/jobradar/model"
)

// CrawlerService is the interface the crawler engine depends on for recording scan outcomes.
type CrawlerService interface {
	// RecordScanResult writes the scan log entry and updates the company's scan status.
	// companyID is the company being scanned.
	// result carries the counts and any error message from the crawl.
	RecordScanResult(companyID string, result ScanResult) error
}

// ScanResult carries the outcome of one company crawl.
type ScanResult struct {
	FetchedCount int
	NewCount     int
	Duration     time.Duration
	ErrMsg       string // empty string means success
}

// companyScanUpdater is the narrow interface for updating company scan status.
type companyScanUpdater interface {
	UpdateScanStatus(id, status string) error
}

type crawlerService struct {
	log     ScanLogWriter      // writes company_scan_log rows
	company companyScanUpdater // updates companies.last_scan_at + last_scan_status
}

// NewCrawlerService constructs a CrawlerService from a log writer and a company scan updater.
func NewCrawlerService(log ScanLogWriter, company companyScanUpdater) CrawlerService {
	return &crawlerService{log: log, company: company}
}

// RecordScanResult writes the scan log entry and updates the company's scan status.
func (c *crawlerService) RecordScanResult(companyID string, result ScanResult) error {
	status := model.ScanStatusOK
	var errPtr *string
	if result.ErrMsg != "" {
		status = model.ScanStatusError
		errPtr = &result.ErrMsg
	}

	durationMs := int(result.Duration.Milliseconds())
	if err := c.log.InsertCompanyScanLog(companyID, result.FetchedCount, result.NewCount, durationMs, errPtr); err != nil {
		return err
	}

	return c.company.UpdateScanStatus(companyID, status)
}
