package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/infra/tracing"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/scan/dto"
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

// RunForCompany runs a scan for a single company by ID.
func (e *Engine) RunForCompany(ctx context.Context, companyID string) {
	ctx, span := tracing.Start(ctx, "crawler.engine.RunForCompany")
	defer tracing.End(span, nil)
	company, err := e.companies.GetByID(ctx, companyID)
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

func (e *Engine) scanCompany(ctx context.Context, company companymodel.Company) {
	ctx, span := tracing.Start(ctx, "crawler.engine.scanCompany")
	defer tracing.End(span, nil)
	start := time.Now()
	log := e.log.With(
		logger.Str(logger.FieldCompany, company.ID),
		logger.Str("name", company.Name),
	)

	result, err := e.fetchJobs(ctx, company, log)
	if err != nil {
		errMsg := err.Error()
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)
		metrics.CrawlerScansTotal.WithLabelValues("error").Inc()
		return
	}

	newCount := e.publishNewJobs(ctx, result.Jobs, company, log)
	e.detectRemovedJobs(ctx, company.ID, result.Jobs, log)

	elapsed := time.Since(start)
	metrics.CrawlerScansTotal.WithLabelValues("success").Inc()
	metrics.CrawlerJobsFetchedTotal.Add(float64(len(result.Jobs)))
	metrics.CrawlerScanDuration.Observe(elapsed.Seconds())

	log.Info("scan complete",
		logger.Int(logger.FieldFetched, len(result.Jobs)),
		logger.Int(logger.FieldNew, newCount),
		logger.Dur(logger.FieldDuration, elapsed.Round(time.Millisecond)),
	)
	e.recordResult(ctx, company.ID, len(result.Jobs), newCount, elapsed, nil)
}

func (e *Engine) fetchJobs(ctx context.Context, company companymodel.Company, log logger.Logger) (ats.FetchJobsResponse, error) {
	adp, err := ats.RegistryWithGoogleConfig(company.AdapterType, e.googleCfg, log)
	if err != nil {
		log.Error("no adapter found", err)
		return ats.FetchJobsResponse{}, fmt.Errorf("no adapter for type %q: %w", company.AdapterType, err)
	}
	result, err := adp.FetchJobs(ctx, company.ID)
	if err != nil {
		log.Error("fetch jobs failed", err)
		return ats.FetchJobsResponse{}, fmt.Errorf("fetch failed: %w", err)
	}
	return result, nil
}

func (e *Engine) publishNewJobs(ctx context.Context, jobs []ats.FetchJobsItem, company companymodel.Company, log logger.Logger) int {
	if len(jobs) == 0 {
		return 0
	}

	ids := make([]string, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ID
	}

	seenMap, err := e.ledger.HasSeenBatch(ctx, ids)
	if err != nil {
		log.Error("batch seen check failed", err)
		return 0
	}

	var newJobs []ats.FetchJobsItem
	for _, job := range jobs {
		if !seenMap[job.ID] {
			newJobs = append(newJobs, job)
		}
	}
	if len(newJobs) == 0 {
		return 0
	}

	// Publish first, collect IDs that made it to Kafka.
	// MarkSeenBatch runs only after — if the process crashes between publish and mark,
	// the next scan re-publishes those jobs. Downstream filter handles duplicates.
	var publishedIDs []string
	for _, job := range newJobs {
		if err := e.publish(ctx, toRawJob(job, company.ID, company.AdapterType)); err != nil {
			log.Error("publish failed", err, logger.Str(logger.FieldJobID, job.ID))
			continue
		}
		publishedIDs = append(publishedIDs, job.ID)
	}
	if len(publishedIDs) == 0 {
		return 0
	}

	if err := e.ledger.MarkSeenBatch(ctx, publishedIDs, company.ID, jobcommon.SeenReasonFetch); err != nil {
		// Jobs are in Kafka but not marked seen — next scan will re-publish them.
		// Downstream filter deduplicates by job_id so this is safe.
		log.Error("batch mark seen failed (jobs published, will be re-sent on next scan)", err)
	}
	return len(publishedIDs)
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
	result := dto.ScanResult{
		FetchedCount: fetched,
		NewCount:     newCount,
		Duration:     duration,
	}
	if errMsg != nil {
		result.ErrMsg = *errMsg
	}
	if err := e.recorder.RecordScanResult(ctx, companyID, result); err != nil {
		e.log.Error("failed to record scan result", err, logger.Str(logger.FieldCompany, companyID))
	}
}
