package job_listing_crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
)

// Engine is the scan worker. It reads companies from DB, fetches their jobs,
// deduplicates via seen_ledger, and publishes new jobs to Kafka.
type Engine struct {
	companies    store.CompanyReader
	ledger       store.SeenLedgerStore
	scanLog      store.CompanyScanLogWriter
	publisher    kafka.Publisher
	companyDelay time.Duration // sleep after each company job-list fetch
}

func NewEngine(
	companies store.CompanyReader,
	ledger store.SeenLedgerStore,
	scanLog store.CompanyScanLogWriter,
	publisher kafka.Publisher,
	companyDelay time.Duration,
) *Engine {
	return &Engine{
		companies:    companies,
		ledger:       ledger,
		scanLog:      scanLog,
		publisher:    publisher,
		companyDelay: companyDelay,
	}
}

// RunOnce runs one full scan cycle across all active companies.
// Call this on a ticker in main — the engine itself has no scheduler.
func (e *Engine) RunOnce(ctx context.Context) {
	companies, err := e.companies.GetActiveCompanies()
	if err != nil {
		log.Printf("[job_listing_crawler] failed to load companies: %v", err)
		return
	}

	log.Printf("[job_listing_crawler] starting cycle — %d active companies", len(companies))

	for _, company := range companies {
		e.scanCompany(ctx, company)
	}

	log.Printf("[job_listing_crawler] cycle complete")
}

// RunForCompany runs a scan for a single company by ID.
// Used when triggered via the company.scan_job Kafka topic.
func (e *Engine) RunForCompany(ctx context.Context, companyID string) {
	company, err := e.companies.GetCompanyByID(companyID)
	if err != nil {
		log.Printf("[job_listing_crawler] RunForCompany: company %q not found: %v", companyID, err)
		return
	}
	log.Printf("[job_listing_crawler] triggered scan for company %s (%s)", company.Name, company.ID)
	e.scanCompany(ctx, company)
}

// scanCompany runs the full scan for a single company.
func (e *Engine) scanCompany(ctx context.Context, company model.Company) {
	start := time.Now()

	adp, err := ats_fetcher.Registry(company.AdapterType)
	if err != nil {
		errMsg := fmt.Sprintf("no adapter for type %q: %v", company.AdapterType, err)
		log.Printf("[job_listing_crawler] %s: %s", company.Name, errMsg)
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)
		return
	}

	jobs, err := adp.FetchJobs(ctx, company.URL)
	if err != nil {
		errMsg := fmt.Sprintf("fetch failed: %v", err)
		log.Printf("[job_listing_crawler] %s: %s", company.Name, errMsg)
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)
		return
	}

	fetched := len(jobs)
	newCount := 0

	for _, job := range jobs {
		seen, err := e.ledger.HasSeen(job.ID)
		if err != nil {
			log.Printf("[job_listing_crawler] %s: seen_ledger check failed for %s: %v", company.Name, job.ID, err)
			continue
		}
		if seen {
			continue
		}

		// New job — attach company UUID so the enricher can use it as FK
		job.CompanyID = company.ID

		// mark it and publish to Kafka
		if err := e.ledger.MarkSeen(job.ID, company.ID, model.SeenReasonFetch); err != nil {
			log.Printf("[job_listing_crawler] %s: failed to mark seen for %s: %v", company.Name, job.ID, err)
			continue
		}

		if err := e.publish(ctx, job); err != nil {
			log.Printf("[job_listing_crawler] %s: failed to publish %s: %v", company.Name, job.ID, err)
			continue
		}

		newCount++
	}

	log.Printf("[job_listing_crawler] %s: fetched=%d new=%d duration=%s", company.Name, fetched, newCount, time.Since(start).Round(time.Millisecond))
	e.recordResult(ctx, company.ID, fetched, newCount, time.Since(start), nil)

	if e.companyDelay > 0 {
		log.Printf("[job_listing_crawler] sleeping %s before next company fetch", e.companyDelay)
		select {
		case <-time.After(e.companyDelay):
		case <-ctx.Done():
		}
	}
}

// publish serialises a RawJob to JSON and sends it to the jobs.raw topic.
func (e *Engine) publish(ctx context.Context, job ats_fetcher.RawJob) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return e.publisher.Publish(ctx, kafka.TopicJobListingsRaw, payload)
}

// recordResult writes the scan summary to job_scan_log and updates the company status.
func (e *Engine) recordResult(ctx context.Context, companyID string, fetched, newCount int, duration time.Duration, errMsg *string) {
	status := model.ScanStatusOK
	if errMsg != nil {
		status = model.ScanStatusError
	}

	durationMs := int(duration.Milliseconds())
	if err := e.scanLog.InsertCompanyScanLog(companyID, fetched, newCount, durationMs, errMsg); err != nil {
		log.Printf("[job_listing_crawler] failed to insert job_scan_log for %s: %v", companyID, err)
	}

	if err := e.scanLog.UpdateCompanyScanStatus(companyID, status); err != nil {
		log.Printf("[job_listing_crawler] failed to update company status for %s: %v", companyID, err)
	}
}
