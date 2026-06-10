package services

import (
	"context"
	"fmt"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
)

// SweeperPublisher is the narrow NATS publishing contract the sweeper needs.
// Allows the scheduler service to publish without importing the full jetstream
// package at the call site.
type SweeperPublisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
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
	jobs, ferr := s.ledger.FetchDueRetryJobs(ctx, sweeperBatchLimit)
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

		// Reset the ledger row to reason='fetch' BEFORE publishing (ordering invariant).
		if rerr := s.ledger.ResetRetryToFetch(ctx, job.JobID); rerr != nil {
			metrics.SweeperRetriesFailedTotal.Inc()
			s.log.Error("sweeper: reset retry to fetch failed", rerr,
				logger.Str(logger.FieldJobID, job.JobID),
			)
			continue
		}

		subject := fmt.Sprintf(js.SubjectJobsRawFmt, job.Adapter)
		if perr := s.publisher.Publish(ctx, subject, job.Payload); perr != nil {
			// Publish failed: re-set the row to reason='retry' with a short delay
			// so the next sweeper tick can retry the publish.
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
			logger.Int(logger.FieldRetryCount, job.RetryCount),
		)
	}
	return nil
}
