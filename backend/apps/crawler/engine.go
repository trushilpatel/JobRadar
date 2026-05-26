package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/location"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	"go.opentelemetry.io/otel/codes"
)

// Engine crawls company careers pages, deduplicates jobs, and publishes new ones to NATS JetStream.
//
// Phase 2: per-company scan configuration (adapter_type, adapter_config,
// active flag) lives on `company_scan_settings`. RunForCompany pulls the
// settings on every queue claim; companies without a settings row are
// logged-and-completed without invoking any adapter so the queue never loops
// on un-onboarded entries.
//
// All adapters are pure-HTTP since the Meta sitemap migration; the engine no
// longer needs a shared Chromium instance.
type Engine struct {
	settings  ScanSettingsReader
	ledger    PipelineLedger
	recorder  ScanRecorder
	publisher js.Publisher
	googleCfg google.Config
	log       logger.Logger
	// adapterFactory resolves an adapter implementation for a given adapter type.
	// Defaults to ats.RegistryWithGoogleConfig — overridden in tests to inject
	// a stub adapter without going through the real registry.
	adapterFactory func(adapterType string, cfg google.Config, log logger.Logger) (ats.Adapter, error)
}

func NewEngine(
	settings ScanSettingsReader,
	ledger PipelineLedger,
	recorder ScanRecorder,
	publisher js.Publisher,
	googleCfg google.Config,
	log logger.Logger,
) *Engine {
	return &Engine{
		settings:       settings,
		ledger:         ledger,
		recorder:       recorder,
		publisher:      publisher,
		googleCfg:      googleCfg,
		log:            log,
		adapterFactory: ats.RegistryWithGoogleConfig,
	}
}

// RunForCompany runs a scan for a single company by ID.
//
// Phase 2 graceful-skip contract:
//   - scan_settings row missing  → log at INFO once, return; the worker
//     completes the queue entry so the row never re-loops.
//   - scan_settings.active=false → log at WARN, return; same handling.
//   - any other Get error        → log at ERROR, return; the queue entry
//     completes (no requeue) so a corrupt company can't block the lane.
func (e *Engine) RunForCompany(ctx context.Context, companyID string) {
	ctx, span := tracing.Start(ctx, "crawler.engine.RunForCompany")
	defer span.End()
	settings, err := e.settings.Get(ctx, companyID)
	if err != nil {
		var sErr *apperr.ServiceError
		if stderrors.As(err, &sErr) && sErr.Code == "NOT_FOUND" {
			e.log.Info("skipping company — no scan_settings configured",
				logger.Str(logger.FieldCompany, companyID),
			)
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		e.log.Error("failed to load scan_settings", err, logger.Str(logger.FieldCompany, companyID))
		return
	}
	if !settings.Active {
		e.log.Warn("skipping inactive company",
			logger.Str(logger.FieldCompany, companyID),
			logger.Str(logger.FieldAdapterType, settings.AdapterType),
		)
		return
	}
	e.log.Debug("triggered scan",
		logger.Str(logger.FieldCompany, companyID),
		logger.Str(logger.FieldAdapterType, settings.AdapterType),
	)
	e.scanCompany(ctx, companyID, settings)
}

func (e *Engine) scanCompany(ctx context.Context, companyID string, settings *companymodel.CompanyScanSettings) {
	ctx, span := tracing.Start(ctx, "crawler.engine.scanCompany")
	defer span.End()
	start := time.Now()
	log := e.log.With(
		logger.Str(logger.FieldCompany, companyID),
		logger.Str(logger.FieldAdapterType, settings.AdapterType),
	)
	scanID := uuid.NewString()
	log = log.With(logger.Str(logger.FieldScanID, scanID))

	// Fetch known job IDs upfront — used for two purposes:
	//   1. Passed to the adapter (via KnownJobsProvider) to skip detail enrichment
	//      for jobs already in the ledger, so only new jobs get detail page visits.
	//   2. Reused in detectRemovedJobs to avoid a second DB round-trip.
	knownIDs, err := e.ledger.GetActiveJobIDsByCompany(ctx, companyID)
	if err != nil {
		log.Error("failed to load known job IDs — enrichment will run for all jobs", err)
		knownIDs = nil
	}

	result, streamed, err := e.fetchJobs(ctx, companyID, settings.AdapterType, json.RawMessage(settings.AdapterConfig), knownIDs, log)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		log.Error("scan failed", err)
		errMsg := err.Error()
		e.recordResult(ctx, companyID, 0, 0, time.Since(start), &errMsg)
		metrics.CrawlerScansTotal.WithLabelValues("error").Inc()
		return
	}

	// Streaming adapters publish per-page inside FetchJobs via the stream callback —
	// result.Jobs is what was already published to NATS and written to the ledger.
	// Skip the post-scan publish pass to avoid a redundant HasSeenBatch round-trip.
	var newCount int
	if streamed {
		newCount = len(result.Jobs)
	} else {
		newCount = e.publishNewJobs(ctx, result.Jobs, companyID, settings.AdapterType, log)
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
	e.recordResult(ctx, companyID, len(result.Jobs), newCount, elapsed, nil)
}

func (e *Engine) fetchJobs(ctx context.Context, companyID, adapterType string, adapterConfig json.RawMessage, knownIDs []string, log logger.Logger) (ats.FetchJobsResponse, bool, error) {
	adp, err := e.adapterFactory(adapterType, e.googleCfg, log)
	if err != nil {
		return ats.FetchJobsResponse{}, false, fmt.Errorf("no adapter for type %q: %w", adapterType, err)
	}

	// If the adapter supports skipping enrichment for known jobs, inject the set now.
	// Used by adapters that rely on a pre-scan static snapshot (legacy path).
	if setter, ok := adp.(ats.KnownJobsProvider); ok && len(knownIDs) > 0 {
		idSet := make(map[string]struct{}, len(knownIDs))
		for _, id := range knownIDs {
			idSet[id] = struct{}{}
		}
		setter.SetKnownJobIDs(idSet)
		log.Debug("injected known job IDs into adapter", logger.Int(logger.FieldCount, len(idSet)))
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

	// If the adapter supports streaming, wire up a publish callback so jobs are
	// published to NATS immediately after each page instead of waiting for the full scan.
	streamed := false
	if sp, ok := adp.(ats.StreamPublisher); ok {
		sp.SetStreamPublish(func(items []ats.FetchJobsItem) {
			e.publishNewJobs(ctx, items, companyID, adapterType, log)
		})
		streamed = true
		log.Debug("streaming publish enabled for adapter")
	}

	// If the adapter supports per-company configuration AND scan_settings carries
	// a non-empty adapter_config blob, use the configurable entry point. Otherwise
	// fall back to plain FetchJobs — preserves compatibility with every existing
	// adapter (Greenhouse, Lever, Ashby, Gem, Deel, …) that derives everything
	// from companyID alone.
	var (
		result ats.FetchJobsResponse
	)
	if ca, ok := adp.(ats.ConfigurableAdapter); ok && len(adapterConfig) > 0 {
		log.Debug("adapter supports per-company config; using FetchJobsWithConfig",
			logger.Int(logger.FieldConfigBytes, len(adapterConfig)))
		result, err = ca.FetchJobsWithConfig(ctx, companyID, adapterConfig)
	} else {
		result, err = adp.FetchJobs(ctx, companyID)
	}
	if err != nil {
		return ats.FetchJobsResponse{}, false, fmt.Errorf("fetch failed: %w", err)
	}
	return result, streamed, nil
}

func (e *Engine) publishNewJobs(ctx context.Context, jobs []ats.FetchJobsItem, companyID, adapterType string, log logger.Logger) int {
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

	// Non-USA gate: when LocationKnown=true and all locations are outside the USA,
	// drop the job before publishing. LocationKnown=false (Workday, Meta thin
	// listing) bypasses the gate — location is resolved later at analyzer time.
	// Per-job isolation: a ledger failure for one dropped job does not stop others.
	var filteredJobs []ats.FetchJobsItem
	for _, job := range newJobs {
		if job.LocationKnown && location.AnyNonUSA(job.AllLocations) {
			if err := e.ledger.MarkSeenBatch(ctx, []string{job.ID}, companyID, jobcommon.SeenReasonNonUSA); err != nil {
				log.Error("failed to mark non-USA job as seen (skipping publish anyway)", err,
					logger.Str(logger.FieldJobID, job.ID),
				)
			}
			metrics.CrawlerJobsNonUSADropped.WithLabelValues(adapterType).Inc()
			log.Info("dropped non-USA job before publish",
				logger.Str(logger.FieldJobID, job.ID),
				logger.Str(logger.FieldLocation, job.Location),
			)
			continue
		}
		filteredJobs = append(filteredJobs, job)
	}
	if len(filteredJobs) == 0 {
		return 0
	}

	// Insert ledger rows (reason='fetch') BEFORE publishing so the row always
	// exists by the time the filter can consume the message. This closes the
	// crawler→filter race where a fast consumer would UPDATE-against-empty and
	// either silently no-op or create a phantom row. MarkSeenBatch uses ON
	// CONFLICT DO NOTHING, so a repeat scan that already inserted these rows is
	// a cheap no-op.
	filteredIDs := make([]string, len(filteredJobs))
	for i, job := range filteredJobs {
		filteredIDs[i] = job.ID
	}
	if err := e.ledger.MarkSeenBatch(ctx, filteredIDs, companyID, jobcommon.SeenReasonFetch); err != nil {
		// Could not record the ledger rows — do NOT publish, or the filter would
		// race a missing row. The next scan re-discovers these jobs.
		log.Error("batch mark seen failed before publish (skipping publish; recovered next scan)", err,
			logger.Int(logger.FieldCount, len(filteredIDs)))
		return 0
	}

	// Publish each job. The ledger row is already 'fetch'. A publish failure
	// leaves the row at 'fetch'; the next scan sees it via HasSeenBatch and
	// skips it — operators query stuck 'fetch' rows via the cleanup migration.
	var publishedCount int
	for _, job := range filteredJobs {
		if err := e.publish(ctx, toRawJob(job, companyID, adapterType)); err != nil {
			metrics.CrawlerPublishFailures.WithLabelValues(adapterType).Inc()
			log.Error("publish failed (ledger row stays at 'fetch'; recovered next scan)", err,
				logger.Str(logger.FieldJobID, job.ID))
			continue
		}
		publishedCount++
	}
	return publishedCount
}

// toRawJob converts a FetchJobsItem into the RawJob message payload.
// CompanyID and AdapterType come from the company record, not the listing.
func toRawJob(item ats.FetchJobsItem, companyID, adapterType string) ats.RawJob {
	return ats.RawJob{
		ID:            item.ID,
		CompanyID:     companyID,
		Adapter:       adapterType,
		Title:         item.Title,
		Location:      item.Location,
		URL:           item.URL,
		EmployType:    item.EmploymentType,
		FullContent:   item.FullContent,
		LocationKnown: item.LocationKnown,
		AllLocations:  item.AllLocations,
	}
}

func (e *Engine) publish(ctx context.Context, job ats.RawJob) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	subject := fmt.Sprintf(js.SubjectJobsRawFmt, job.Adapter)
	return e.publisher.PublishWithMsgID(ctx, subject, payload, job.ID)
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
		log.Error("failed to mark removed jobs", err, logger.Int(logger.FieldCount, len(removedIDs)))
		return
	}

	log.Info("marked removed jobs",
		logger.Int(logger.FieldRemoved, len(removedIDs)),
		logger.Int(logger.FieldStillActive, len(previousIDs)-len(removedIDs)),
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
