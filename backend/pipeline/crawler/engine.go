package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/pipeline/ats"
	"github.com/trushilpatel/jobradar/pipeline/ats/google"
	scansvc "github.com/trushilpatel/jobradar/service/scan"
)

// Engine crawls company careers pages, deduplicates jobs, and publishes new ones to Kafka.
type Engine struct {
	companies CompanyReader
	ledger    PipelineLedger
	recorder  ScanRecorder
	publisher kafka.Publisher
	googleCfg google.Config
	log       logger.Logger
}

func NewEngine(
	companies CompanyReader,
	ledger PipelineLedger,
	recorder ScanRecorder,
	publisher kafka.Publisher,
	googleCfg google.Config,
	log logger.Logger,
) *Engine {
	return &Engine{
		companies: companies,
		ledger:    ledger,
		recorder:  recorder,
		publisher: publisher,
		googleCfg: googleCfg,
		log:       log,
	}
}

// RunOnce runs one full scan cycle across all active companies.
func (e *Engine) RunOnce(ctx context.Context) {
	companies, err := e.companies.GetActiveCompanies()
	if err != nil {
		e.log.Error("failed to load companies", err)
		return
	}
	e.log.Info("starting scan cycle", logger.Int("active_companies", len(companies)))
	for _, company := range companies {
		e.scanCompany(ctx, company)
	}
	e.log.Info("scan cycle complete")
}

// RunForCompany runs a scan for a single company by ID.
// Used when triggered via the company.scan_request Kafka topic.
func (e *Engine) RunForCompany(ctx context.Context, companyID string) {
	company, err := e.companies.GetCompanyByID(companyID)
	if err != nil {
		e.log.Error("company not found", err, logger.Str(logger.FieldCompany, companyID))
		return
	}
	if !company.Active {
		e.log.Warn("skipping inactive company",
			logger.Str(logger.FieldCompany, company.ID),
			logger.Str("name", company.Name),
		)
		return
	}
	e.log.Debug("triggered scan",
		logger.Str(logger.FieldCompany, company.ID),
		logger.Str("name", company.Name),
	)
	e.scanCompany(ctx, company)
}

func (e *Engine) scanCompany(ctx context.Context, company model.Company) {
	start := time.Now()
	companyLog := e.log.With(
		logger.Str(logger.FieldCompany, company.ID),
		logger.Str("name", company.Name),
	)

	adp, err := ats.RegistryWithGoogleConfig(company.AdapterType, e.googleCfg, companyLog)
	if err != nil {
		errMsg := fmt.Sprintf("no adapter for type %q: %v", company.AdapterType, err)
		companyLog.Error("no adapter found", err)
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)

		metrics.CrawlerScansTotal.WithLabelValues("error").Inc()
		return
	}

	result, err := adp.FetchJobs(ctx, company.ID)
	if err != nil {
		errMsg := fmt.Sprintf("fetch failed: %v", err)
		companyLog.Error("fetch jobs failed", err)
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)
		metrics.CrawlerScansTotal.WithLabelValues("error").Inc()
		return
	}

	fetched := len(result.Jobs)
	newCount := 0

	for _, item := range result.Jobs {
		seen, err := e.ledger.HasSeen(ctx, item.ID)
		if err != nil {
			companyLog.Error("seen_ledger check failed", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}
		if seen {
			continue
		}

		if err := e.ledger.MarkSeen(ctx, item.ID, company.ID, model.SeenReasonFetch); err != nil {
			companyLog.Error("failed to mark seen", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}

		rawJob := toRawJob(item, company.ID, company.AdapterType)
		if err := e.publish(ctx, rawJob); err != nil {
			companyLog.Error("publish failed", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}

		newCount++
	}

	// Detect removed jobs: diff current scan results against previously known active jobs.
	// This is important for full-scan adapters (e.g. Google) that return all jobs each time.
	e.detectRemovedJobs(ctx, company.ID, result.Jobs, companyLog)

	elapsed := time.Since(start)
	metrics.CrawlerScansTotal.WithLabelValues("success").Inc()
	metrics.CrawlerJobsFetchedTotal.Add(float64(fetched))
	metrics.CrawlerScanDuration.Observe(elapsed.Seconds())

	companyLog.Info("scan complete",
		logger.Int(logger.FieldFetched, fetched),
		logger.Int(logger.FieldNew, newCount),
		logger.Dur(logger.FieldDuration, elapsed.Round(time.Millisecond)),
	)
	e.recordResult(ctx, company.ID, fetched, newCount, elapsed, nil)
}

// toRawJob converts a FetchJobsItem into the RawJob Kafka message.
// CompanyID and AdapterType come from the company record, not the listing.
func toRawJob(item ats.FetchJobsItem, companyID, adapterType string) ats.RawJob {
	return ats.RawJob{
		ID:          item.ID,
		CompanyID:   companyID,
		Adapter:     adapterType,
		Title:       item.Title,
		Location:    item.Location,
		URL:         item.URL,
		EmployType:  item.EmploymentType,
		FullContent: item.FullContent,
	}
}

func (e *Engine) publish(ctx context.Context, job ats.RawJob) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return e.publisher.Publish(ctx, kafka.TopicJobListingsRaw, payload)
}

// detectRemovedJobs compares the current scan results against previously known
// active jobs for a company. Any job that was previously seen but is no longer
// in the current scan gets marked with removed_at in the ledger.
func (e *Engine) detectRemovedJobs(ctx context.Context, companyID string, currentJobs []ats.FetchJobsItem, log logger.Logger) {
	// Get all previously active job IDs for this company
	previousIDs, err := e.ledger.GetActiveJobIDsByCompany(ctx, companyID)
	if err != nil {
		log.Error("failed to load previous job IDs for removal detection", err)
		return
	}
	if len(previousIDs) == 0 {
		return // First scan or no active jobs — nothing to diff
	}

	// Build a set of current job IDs
	currentSet := make(map[string]struct{}, len(currentJobs))
	for _, job := range currentJobs {
		currentSet[job.ID] = struct{}{}
	}

	// Find IDs that were previously active but are no longer in the current scan
	var removedIDs []string
	for _, id := range previousIDs {
		if _, exists := currentSet[id]; !exists {
			removedIDs = append(removedIDs, id)
		}
	}

	if len(removedIDs) == 0 {
		return
	}

	if err := e.ledger.MarkRemovedBatch(ctx, removedIDs); err != nil {
		log.Error("failed to mark removed jobs", err, logger.Int("count", len(removedIDs)))
		return
	}

	log.Info("marked removed jobs",
		logger.Int("removed", len(removedIDs)),
		logger.Int("still_active", len(previousIDs)-len(removedIDs)),
	)
}

func (e *Engine) recordResult(ctx context.Context, companyID string, fetched, newCount int, duration time.Duration, errMsg *string) {
	result := scansvc.ScanResult{
		FetchedCount: fetched,
		NewCount:     newCount,
		Duration:     duration,
	}
	if errMsg != nil {
		result.ErrMsg = *errMsg
	}
	if err := e.recorder.RecordScanResult(companyID, result); err != nil {
		e.log.Error("failed to record scan result", err, logger.Str(logger.FieldCompany, companyID))
	}
}
