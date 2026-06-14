package services

import (
	"context"
	"fmt"
	"strconv"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
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

// schedulerService implements SchedulerService for the scheduler worker.
// It uses ScanQueueStore directly — not part of the REST decorator chain.
type schedulerService struct {
	queue      ScanQueueStore
	ledger     jobsvc.SeenLedgerStore
	publisher  SweeperPublisher
	maxRetries int
	log        logger.Logger
}

// SchedulerServiceOptions groups the optional sweeper dependencies for
// NewSchedulerService. Zero-value means no sweeper (RetryDueJobs is a no-op).
type SchedulerServiceOptions struct {
	Ledger     jobsvc.SeenLedgerStore
	Publisher  SweeperPublisher
	MaxRetries int
	Log        logger.Logger
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
		queue:      queue,
		ledger:     opts.Ledger,
		publisher:  opts.Publisher,
		maxRetries: maxRetries,
		log:        log,
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
