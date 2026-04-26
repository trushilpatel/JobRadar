package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
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
	browser   *rod.Browser // shared Chromium; nil = adapters launch their own
	log       logger.Logger
}

// WithBrowser sets a shared Chromium instance that rod-based adapters (Google, Meta)
// will use instead of launching a fresh browser per scan.
func (e *Engine) WithBrowser(b *rod.Browser) {
	e.browser = b
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

	// Fetch known job IDs upfront — used for two purposes:
	//   1. Passed to the adapter (via KnownJobsProvider) to skip detail enrichment
	//      for jobs already in the ledger, so only new jobs get detail page visits.
	//   2. Reused in detectRemovedJobs to avoid a second DB round-trip.
	knownIDs, err := e.ledger.GetActiveJobIDsByCompany(ctx, company.ID)
	if err != nil {
		log.Error("failed to load known job IDs — enrichment will run for all jobs", err)
		knownIDs = nil
	}

	result, streamed, err := e.fetchJobs(ctx, company, knownIDs, log)
	if err != nil {
		log.Error("scan failed", err)
		errMsg := err.Error()
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)
		metrics.CrawlerScansTotal.WithLabelValues("error").Inc()
		return
	}

	// Streaming adapters publish per-page inside FetchJobs via the stream callback —
	// result.Jobs is what was already committed to Kafka and the ledger. Skip the
	// post-scan publish pass to avoid a redundant HasSeenBatch round-trip.
	var newCount int
	if streamed {
		newCount = len(result.Jobs)
	} else {
		newCount = e.publishNewJobs(ctx, result.Jobs, company, log)
	}
	e.detectRemovedJobs(ctx, result.Jobs, knownIDs, log)

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

func (e *Engine) fetchJobs(ctx context.Context, company companymodel.Company, knownIDs []string, log logger.Logger) (ats.FetchJobsResponse, bool, error) {
	adp, err := ats.RegistryWithGoogleConfig(company.AdapterType, e.googleCfg, log)
	if err != nil {
		return ats.FetchJobsResponse{}, false, fmt.Errorf("no adapter for type %q: %w", company.AdapterType, err)
	}

	// If the adapter supports skipping enrichment for known jobs, inject the set now.
	// Used by adapters that rely on a pre-scan static snapshot (legacy path).
	if setter, ok := adp.(ats.KnownJobsProvider); ok && len(knownIDs) > 0 {
		idSet := make(map[string]struct{}, len(knownIDs))
		for _, id := range knownIDs {
			idSet[id] = struct{}{}
		}
		setter.SetKnownJobIDs(idSet)
		log.Debug("injected known job IDs into adapter", logger.Int("count", len(idSet)))
	}

	// If the adapter supports live per-page seen checks, inject HasSeenBatch now.
	// Preferred over KnownJobsProvider — one targeted DB call per page instead of
	// a full pre-scan snapshot held in memory.
	if sc, ok := adp.(ats.SeenChecker); ok {
		sc.SetSeenChecker(func(ctx context.Context, ids []string) (map[string]bool, error) {
			return e.ledger.HasSeenBatch(ctx, ids)
		})
		log.Debug("live seen checker enabled for adapter")
	}

	// If the adapter supports streaming, wire up a publish callback so jobs enter
	// Kafka immediately after each page instead of waiting for the full scan.
	streamed := false
	if sp, ok := adp.(ats.StreamPublisher); ok {
		sp.SetStreamPublish(func(items []ats.FetchJobsItem) {
			e.publishNewJobs(ctx, items, company, log)
		})
		streamed = true
		log.Debug("streaming publish enabled for adapter")
	}

	if e.browser != nil {
		ats.SetBrowser(adp, e.browser)
	}
	result, err := adp.FetchJobs(ctx, company.ID)
	if err != nil {
		return ats.FetchJobsResponse{}, false, fmt.Errorf("fetch failed: %w", err)
	}
	return result, streamed, nil
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

// detectRemovedJobs compares the current scan results against previousIDs (fetched
// before the scan to also serve the KnownJobsProvider optimisation). Any job that
// was previously active but is absent from the current scan is marked removed.
func (e *Engine) detectRemovedJobs(ctx context.Context, currentJobs []ats.FetchJobsItem, previousIDs []string, log logger.Logger) {
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
