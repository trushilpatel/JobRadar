package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"strconv"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	auditmodel "github.com/trushilpatel/jobradar/service/audit/models"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
)

// stuckRetryThreshold is how far past next_attempt_at a row must be before
// EscalateStuckRetryJobs promotes it to reason='error'. 4 hours is chosen to
// match the minimum next_attempt_at floor set by the filter (AC-5) — any row
// still at reason='retry' after 4h has clearly not been re-published.
const stuckRetryThreshold = 4 * time.Hour

// stuckRetryLimit caps the number of rows escalated per scheduler tick to
// bound the DB work per cycle and prevent thundering-herd escalations after
// a prolonged outage.
const stuckRetryLimit = 100

// stuckFetchThreshold is how old a reason='fetch' ledger row must be before the
// reclaimer deletes it. Normal fetch→verdict latency is seconds-to-minutes; the
// worst-case legitimate in-flight window (NATS redelivery + filter retry
// backoff) is well under an hour. 1h is a conservative safety floor (~60-600x
// normal) that still recovers within the same incident. It is a module const,
// not a config var, to mirror the sibling retry sweep and keep the operator
// surface minimal.
const stuckFetchThreshold = 1 * time.Hour

// stuckFetchLimit caps the number of stranded fetch rows deleted per scheduler
// tick to bound DB work; a backlog drains across successive ticks. Set higher
// than stuckRetryLimit because fetch-row flow (~160/min observed) exceeds the
// retry-escalation rate, so a lower cap cannot drain a backlog faster than it
// accumulates.
const stuckFetchLimit = 500

// SweeperPublisher is the narrow NATS publishing contract the sweeper needs.
// Allows the scheduler service to publish without importing the full jetstream
// package at the call site.
//
// PublishWithHeaders is used by the sweeper to stamp X-Retry-Count on
// re-injected messages so the filter can read the monotonic count instead of
// resetting to DeliveryCount-1 on each sweeper cycle (AC-1).
type SweeperPublisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
	PublishWithHeaders(ctx context.Context, subject string, payload []byte, headers map[string]string) error
}

// AutoPauseAuditor is the narrow audit-writing contract the auto-pause sweep
// needs. It is satisfied by audit/services.Service.Append. Declared here (where
// the consuming struct lives) so the scheduler service depends on a single
// method, not the full audit Service — and so the dependency is optional
// (mirroring the Ledger/Publisher optional-deps pattern). When nil, the sweep
// still pauses companies but logs a WARN that audit is unconfigured; a failed
// Append is likewise non-fatal (audit must never block runaway-company pausing).
type AutoPauseAuditor interface {
	Append(ctx context.Context, entry auditmodel.Entry) error
}

// AutoPauseAction is the admin_audit_log action recorded for each company the
// scheduler auto-pauses. A dedicated const (not a raw string) keeps the audit
// vocabulary enforced by the compiler.
const AutoPauseAction = "company.scan.auto_pause"

// GaugeNodeReader is the narrow crawler_nodes read contract the gauge
// refresher needs — declared here (not the full crawlernode NodeStore) so
// the scheduler depends on one method, not the admin CRUD surface it never
// calls. Satisfied structurally by crawlernodesvc.NodeStore.
type GaugeNodeReader interface {
	CountOnline(ctx context.Context) (int, error)
}

// GaugeSettingsReader is the narrow company_scan_settings read contract the
// gauge refresher needs. Satisfied structurally by
// companysvc.ScanSettingsRepository.
type GaugeSettingsReader interface {
	CountAutoPaused(ctx context.Context) (int, error)
}

// schedulerService implements SchedulerService for the scheduler worker.
// It uses ScanQueueStore directly — not part of the REST decorator chain.
type schedulerService struct {
	queue          ScanQueueStore
	ledger         jobsvc.SeenLedgerStore
	publisher      SweeperPublisher
	auditor        AutoPauseAuditor
	nodeReader     GaugeNodeReader
	settingsReader GaugeSettingsReader
	maxRetries     int
	log            logger.Logger
}

// SchedulerServiceOptions groups the optional sweeper dependencies for
// NewSchedulerService. Zero-value means no sweeper (RetryDueJobs is a no-op).
type SchedulerServiceOptions struct {
	Ledger         jobsvc.SeenLedgerStore
	Publisher      SweeperPublisher
	Auditor        AutoPauseAuditor
	NodeReader     GaugeNodeReader
	SettingsReader GaugeSettingsReader
	MaxRetries     int
	Log            logger.Logger
}

// NewSchedulerService constructs the minimal service the scheduler worker needs.
// Pass a non-nil SchedulerServiceOptions.Ledger and Publisher to enable the
// sweeper (RetryDueJobs). Without them, RetryDueJobs is a no-op.
func NewSchedulerService(queue ScanQueueStore) SchedulerService {
	return &schedulerService{queue: queue, maxRetries: 10}
}

// NewSchedulerServiceWithSweeper constructs the service with sweeper support.
func NewSchedulerServiceWithSweeper(queue ScanQueueStore, opts SchedulerServiceOptions) SchedulerService {
	maxRetries := opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 10
	}
	log := opts.Log
	if log == nil {
		log = logger.NewNoop()
	}
	return &schedulerService{
		queue:          queue,
		ledger:         opts.Ledger,
		publisher:      opts.Publisher,
		auditor:        opts.Auditor,
		nodeReader:     opts.NodeReader,
		settingsReader: opts.SettingsReader,
		maxRetries:     maxRetries,
		log:            log,
	}
}

func (s *schedulerService) EnqueueDue(ctx context.Context, intervalMinutes int) (_ SchedulerCycleResult, err error) {
	ctx, span := tracing.Start(ctx, "scan.service.EnqueueDue")
	defer tracing.End(span, &err)
	res, err := s.queue.EnqueueDue(ctx, intervalMinutes, scancommon.QueueSourceScheduler)
	if err != nil {
		return SchedulerCycleResult{}, apperr.ErrInternal.WithMessage("failed to enqueue due companies")
	}
	return SchedulerCycleResult(res), nil
}

func (s *schedulerService) ResetStuck(ctx context.Context, staleDur time.Duration) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.service.ResetStuck")
	defer tracing.End(span, &err)
	n, err := s.queue.ResetStuck(ctx, staleDur)
	if err != nil {
		return 0, apperr.ErrInternal.WithMessage("failed to reset stuck queue entries")
	}
	return n, nil
}

func (s *schedulerService) PurgeInactive(ctx context.Context) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.service.PurgeInactive")
	defer tracing.End(span, &err)
	n, err := s.queue.PurgeInactive(ctx)
	if err != nil {
		return 0, apperr.ErrInternal.WithMessage("failed to purge inactive company queue entries")
	}
	return n, nil
}

// RetryDueJobs fetches all ledger rows with reason='retry' and
// next_attempt_at <= now(), then either re-publishes them to
// jobs.raw.{adapter} or escalates them to reason='error'.
//
// Per-job error isolation: a failure on one job is logged and counted but
// never aborts the remaining jobs in the batch.
//
// If the sweeper was not configured (no ledger or publisher), this is a no-op.
func (s *schedulerService) RetryDueJobs(ctx context.Context) (err error) {
	if s.ledger == nil || s.publisher == nil {
		return nil
	}

	ctx, span := tracing.Start(ctx, "scan.service.RetryDueJobs")
	defer tracing.End(span, &err)

	// sweeperBatchLimit caps rows per tick to prevent post-outage storms
	// when many retries become due simultaneously.
	const sweeperBatchLimit = 50
	// AC-3: use the payload-filtered variant so the sweeper never tries to
	// publish rows that lack a stored payload or adapter.
	jobs, ferr := s.ledger.FetchDueRetryJobsWithPayload(ctx, sweeperBatchLimit)
	if ferr != nil {
		return fmt.Errorf("RetryDueJobs: fetch: %w", ferr)
	}
	if len(jobs) == 0 {
		return nil
	}

	for _, job := range jobs {
		if job.RetryCount >= s.maxRetries {
			// Budget exhausted: escalate to reason='error'.
			if eerr := s.ledger.EscalateRetryToError(ctx, job.JobID); eerr != nil {
				metrics.SweeperRetriesFailedTotal.Inc()
				s.log.Error("sweeper: escalate retry to error failed", eerr,
					logger.Str(logger.FieldJobID, job.JobID),
					logger.Int(logger.FieldRetryCount, job.RetryCount),
				)
				continue
			}
			metrics.SweeperRetriesEscalatedTotal.Inc()
			s.log.Info("sweeper: retry escalated to error (budget exhausted)",
				logger.Str(logger.FieldJobID, job.JobID),
				logger.Int(logger.FieldRetryCount, job.RetryCount),
			)
			continue
		}

		// AC-1: stamp X-Retry-Count = (job.RetryCount + 1) so the filter reads
		// the monotonic count across sweeper cycles rather than resetting to
		// DeliveryCount-1. The ledger row stays at reason='retry' while in-flight
		// (no ResetRetryToFetch) — the filter transitions it once it processes
		// the message. This closes the race where a filter crash between Reset
		// and Publish left the row at reason='fetch' forever.
		nextRetry := job.RetryCount + 1
		subject := fmt.Sprintf(js.SubjectJobsRawFmt, job.Adapter)
		headers := map[string]string{js.HeaderRetryCount: strconv.Itoa(nextRetry)}
		if perr := s.publisher.PublishWithHeaders(ctx, subject, job.Payload, headers); perr != nil {
			// Publish failed: re-queue with a short delay so the next sweeper
			// tick can retry. Row stays at reason='retry' — no state transition.
			metrics.SweeperRetriesFailedTotal.Inc()
			s.log.Error("sweeper: publish failed; re-queuing retry", perr,
				logger.Str(logger.FieldJobID, job.JobID),
				logger.Str(logger.FieldAdapter, job.Adapter),
			)
			// Re-write the retry row with a 1-minute delay to avoid hot-looping.
			reentry := []jobsvc.RetryLedgerEntryWithPayload{{
				JobID:         job.JobID,
				Adapter:       job.Adapter,
				Payload:       job.Payload,
				RetryCount:    job.RetryCount, // keep same count — the failed publish doesn't count
				NextAttemptAt: time.Now().Add(time.Minute),
			}}
			if werr := s.ledger.UpdateRetryBatchWithPayload(ctx, reentry); werr != nil {
				s.log.Error("sweeper: failed to re-queue after publish failure", werr,
					logger.Str(logger.FieldJobID, job.JobID),
				)
			}
			continue
		}

		metrics.SweeperRetriesPublishedTotal.Inc()
		s.log.Info("sweeper: retry re-published",
			logger.Str(logger.FieldJobID, job.JobID),
			logger.Str(logger.FieldAdapter, job.Adapter),
			logger.Int(logger.FieldRetryCount, nextRetry),
		)
	}
	return nil
}

// EscalateStuckRetryJobs promotes reason='retry' ledger rows whose
// next_attempt_at is more than stuckRetryThreshold (4h) overdue to
// reason='error'. This is a safety net for persistent NATS outages where the
// sweeper cannot re-publish — jobs eventually surface as errors so they can be
// inspected rather than silently remaining at reason='retry' forever.
//
// Returns (0, nil) when no ledger was configured (no-op) or no stuck rows exist.
func (s *schedulerService) EscalateStuckRetryJobs(ctx context.Context) (_ int64, err error) {
	if s.ledger == nil {
		return 0, nil
	}

	ctx, span := tracing.Start(ctx, "scan.service.EscalateStuckRetryJobs")
	defer tracing.End(span, &err)

	n, serr := s.ledger.EscalateStuckRetryJobs(ctx, stuckRetryThreshold, stuckRetryLimit)
	if serr != nil {
		return 0, fmt.Errorf("EscalateStuckRetryJobs: %w", serr)
	}
	if n > 0 {
		metrics.SweeperStuckEscalatedTotal.Add(float64(n))
		s.log.Info("sweeper: stuck retry jobs escalated to error",
			logger.Int(logger.FieldCount, int(n)),
		)
	}
	return n, nil
}

// ReclaimStuckFetchRows DELETEs job_pipeline_ledger rows stranded at
// reason='fetch' (removed_at IS NULL, first_seen_at older than
// stuckFetchThreshold), bounded by stuckFetchLimit per call. These are jobs the
// crawler marked seen before publishing but the filter never transitioned to a
// terminal verdict. Because HasSeenBatch ignores reason and removed_at, deleting
// the row is the only way to clear the dedup block so the next crawl can
// re-drive the job — escalating to reason='error' would keep it deduped forever.
//
// Returns (0, nil) when no ledger was configured (no-op) or no stuck rows exist.
func (s *schedulerService) ReclaimStuckFetchRows(ctx context.Context) (_ int64, err error) {
	if s.ledger == nil {
		return 0, nil
	}

	ctx, span := tracing.Start(ctx, "scan.service.ReclaimStuckFetchRows")
	defer tracing.End(span, &err)

	n, serr := s.ledger.ReclaimStuckFetchRows(ctx, stuckFetchThreshold, stuckFetchLimit)
	if serr != nil {
		return 0, fmt.Errorf("ReclaimStuckFetchRows: %w", serr)
	}
	if n > 0 {
		metrics.SchedulerStuckFetchReclaimedTotal.Add(float64(n))
		s.log.Info("reclaimed stuck fetch ledger rows",
			logger.Int(logger.FieldCount, int(n)),
		)
	}
	return n, nil
}

// AutoPauseChronicFailures sets auto_paused=true for every company whose
// consecutive_errors has crossed AutoPauseThreshold and is not yet paused,
// writing one admin_audit_log row per newly paused company. It is idempotent —
// the store's WHERE auto_paused=false guard means an already-paused company is
// neither re-paused nor re-audited. Audit is NON-FATAL: a nil auditor or a
// failed Append is logged at WARN and never blocks or fails the pause (pausing a
// runaway company matters more than the audit row; the metric + log still record
// it). Returns the number of companies newly paused — zero is not an error.
func (s *schedulerService) AutoPauseChronicFailures(ctx context.Context) (_ int, err error) {
	if s.queue == nil {
		return 0, nil
	}

	ctx, span := tracing.Start(ctx, "scan.service.AutoPauseChronicFailures")
	defer tracing.End(span, &err)

	ids, serr := s.queue.AutoPauseChronicFailures(ctx, scancommon.AutoPauseThreshold)
	if serr != nil {
		return 0, fmt.Errorf("AutoPauseChronicFailures: %w", serr)
	}

	for _, id := range ids {
		companyID := id
		if s.auditor == nil {
			s.log.Warn("auto-pause: audit unconfigured, skipping audit row",
				logger.Str(logger.FieldCompany, companyID),
			)
			continue
		}
		targetType := logAutoPauseTargetType
		entry := auditmodel.Entry{
			Action:     AutoPauseAction,
			TargetType: &targetType,
			TargetID:   &companyID,
		}
		if aerr := s.auditor.Append(ctx, entry); aerr != nil {
			s.log.Warn("auto-pause: audit append failed (non-fatal)",
				logger.Str(logger.FieldCompany, companyID),
				logger.Err(aerr),
			)
		}
	}

	n := len(ids)
	if n > 0 {
		metrics.SchedulerCompaniesAutoPausedTotal.Add(float64(n))
		s.log.Info("auto-paused chronic-failing companies",
			logger.Int(logger.FieldCount, n),
		)
	}
	return n, nil
}

// logAutoPauseTargetType is the audit target_type recorded for auto-pause rows.
const logAutoPauseTargetType = "company"

// RefreshGaugeMetrics recomputes the 5 current-state gauge metrics (queue
// depth, oldest-pending age, online node count, currently-paused company
// count, retry-ladder backlog) from their source-of-truth tables and sets
// them on the package-level Prometheus gauges. Called once per scheduler
// tick (runCycle).
//
// Each of the 5 reads is independent — a failure on one is joined into the
// returned error but never prevents the others from running or setting
// their gauge. NodeReader and SettingsReader are optional (nil-safe,
// mirroring the Ledger/Publisher optional-deps pattern); when nil, that
// gauge is simply not refreshed this tick rather than treated as an error.
func (s *schedulerService) RefreshGaugeMetrics(ctx context.Context) (err error) {
	ctx, span := tracing.Start(ctx, "scan.service.RefreshGaugeMetrics")
	defer tracing.End(span, &err)

	var errs []error

	if n, qerr := s.queue.CountQueued(ctx); qerr != nil {
		errs = append(errs, fmt.Errorf("count queued: %w", qerr))
	} else {
		metrics.SchedulerQueuePending.Set(float64(n))
	}

	if age, aerr := s.queue.OldestQueuedAgeSeconds(ctx); aerr != nil {
		errs = append(errs, fmt.Errorf("oldest queued age: %w", aerr))
	} else {
		metrics.SchedulerQueueOldestPendingAgeSeconds.Set(age)
	}

	if s.nodeReader != nil {
		if n, nerr := s.nodeReader.CountOnline(ctx); nerr != nil {
			errs = append(errs, fmt.Errorf("count online nodes: %w", nerr))
		} else {
			metrics.CrawlerNodesOnline.Set(float64(n))
		}
	}

	if s.settingsReader != nil {
		if n, serr := s.settingsReader.CountAutoPaused(ctx); serr != nil {
			errs = append(errs, fmt.Errorf("count auto-paused: %w", serr))
		} else {
			metrics.SchedulerCompaniesAutoPausedCurrent.Set(float64(n))
		}
	}

	if s.ledger != nil {
		if n, lerr := s.ledger.CountByReason(ctx, jobcommon.SeenReasonRetry); lerr != nil {
			errs = append(errs, fmt.Errorf("count retry ledger: %w", lerr))
		} else {
			metrics.JobPipelineLedgerRetryCount.Set(float64(n))
		}
	}

	return stderrors.Join(errs...)
}
