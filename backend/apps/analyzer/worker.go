package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmcore "github.com/trushilpatel/jobradar/service/llm/services"
)

const analyzerMaxRetries = 3

var analyzerBackOff = []time.Duration{time.Minute, 10 * time.Minute, time.Hour}

// clock abstracts wall-clock reads and ctx-aware sleeps so tests can control
// both the inter-group pacing and the NextAttemptAt value stamped into the
// retry ledger row.
type clock interface {
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WorkerOptions collects the dependencies required to build a Worker.
type WorkerOptions struct {
	Consumer         MessageFetcher
	Publisher        js.Publisher
	Scorer           llmcore.JobPostEnricher
	Pipeline         jobsvc.PipelineService
	Ledger           EnricherLedger
	RateLimiter      RateLimiter
	Log              logger.Logger
	MaxExpYears      int
	FetchConcurrency int
	LLMBatchSize     int
	LLMCallInterval  time.Duration
	FlushTimeout     time.Duration
}

// Worker consumes job_listings.filtered (NATS JetStream), runs each fetch
// through two stages, and acks/naks each message individually:
//
//  1. Fetch stage  — parallel HTTP fetches, per-adapter rate limited.
//     Failures call Nak (retry) or Term (exhausted / malformed).
//  2. Enrich stage — successful (job, content) pairs are grouped into batches
//     of llmBatchSize and sent through EnrichBatch serially, paced by
//     llmCallInterval. Per-job terminal outcomes call Ack; transient failures
//     call Nak (within retry budget) or send to DLQ + Term (exhausted).
type Worker struct {
	consumer        MessageFetcher
	publisher       js.Publisher
	scorer          llmcore.JobPostEnricher
	pipeline        jobsvc.PipelineService
	ledger          EnricherLedger
	rateLimiter     RateLimiter
	client          *http.Client
	log             logger.Logger
	clock           clock
	registryFn      func(string) (ats.Adapter, error) // defaults to ats.Registry; replaced in tests
	maxExpYears     int
	fetchConc       int
	llmBatchSize    int
	llmCallInterval time.Duration
	flushTimeout    time.Duration
}

func NewWorker(opts WorkerOptions) *Worker {
	if opts.FetchConcurrency < 1 {
		opts.FetchConcurrency = 1
	}
	if opts.LLMBatchSize < 1 {
		opts.LLMBatchSize = 1
	}
	if opts.FlushTimeout <= 0 {
		opts.FlushTimeout = 120 * time.Second
	}
	return &Worker{
		consumer:        opts.Consumer,
		publisher:       opts.Publisher,
		scorer:          opts.Scorer,
		pipeline:        opts.Pipeline,
		ledger:          opts.Ledger,
		rateLimiter:     opts.RateLimiter,
		client:          &http.Client{Timeout: 15 * time.Second},
		log:             opts.Log,
		clock:           realClock{},
		registryFn:      ats.Registry,
		maxExpYears:     opts.MaxExpYears,
		fetchConc:       opts.FetchConcurrency,
		llmBatchSize:    opts.LLMBatchSize,
		llmCallInterval: opts.LLMCallInterval,
		flushTimeout:    opts.FlushTimeout,
	}
}

// fetchedJob carries an already-fetched job through stage 2. The NATS message
// is retained so per-job Ack/Nak/Term can be called at terminal routing points.
type fetchedJob struct {
	job        ats.RawJob
	content    ats.FetchJobContentResponse
	msg        *js.Msg
	retryCount int
	ctx        context.Context
}

// Run fetches messages from NATS JetStream in a loop and processes each batch
// through the two-stage pipeline. Each message is individually acked/nakked/
// termed at its terminal routing point — NATS handles redelivery via BackOff.
func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker started",
		logger.Int(logger.FieldFetchConcurrency, w.fetchConc),
		logger.Int(logger.FieldLLMBatchSize, w.llmBatchSize),
		logger.Dur(logger.FieldLLMCallInterval, w.llmCallInterval),
	)

	fetchSize := w.fetchConc * w.llmBatchSize
	if fetchSize < 1 {
		fetchSize = 1
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		msgs, err := w.consumer.Fetch(fetchSize, w.flushTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			w.log.Error("fetch failed", err)
			continue
		}
		if len(msgs) == 0 {
			continue
		}

		if perr := w.processFetch(ctx, msgs); perr != nil && ctx.Err() == nil {
			w.log.Error("processFetch error", perr)
		}
	}
}

// processFetch is the two-stage pipeline for one NATS fetch batch. Stage 1
// fetches content for all messages concurrently (rate limited per adapter).
// Stage 2 groups the successfully-fetched jobs, sleeps llmCallInterval between
// successive EnrichBatch calls, and routes each job to a terminal outcome.
func (w *Worker) processFetch(ctx context.Context, msgs []*js.Msg) (err error) {
	if len(msgs) == 0 {
		return nil
	}

	// TODO: extract W3C trace context from msg headers via tracing.ExtractNATS once available

	fetchID := uuid.NewString()
	fetchLog := w.log.With(logger.Str(logger.FieldFetchID, fetchID))

	fetchCtx, span := tracing.Start(ctx, "analyzer.worker.processFetch")
	span.SetAttributes(
		attribute.String("fetch.id", fetchID),
		attribute.Int("fetch.msgs", len(msgs)),
	)
	defer tracing.End(span, &err)

	fetchLog.Info("fetch received", logger.Int(logger.FieldMsgs, len(msgs)))

	start := time.Now()
	fetched := w.fetchAll(fetchCtx, fetchLog, msgs)
	fetchDuration := time.Since(start)

	if len(fetched) == 0 {
		fetchLog.Info("fetch processed",
			logger.Int(logger.FieldMsgs, len(msgs)),
			logger.Int(logger.FieldFetched, 0),
			logger.Dur(logger.FieldFetchDuration, fetchDuration),
		)
		return nil
	}

	groups := chunkFetched(fetched, w.llmBatchSize)
	span.SetAttributes(
		attribute.Int("fetch.fetched", len(fetched)),
		attribute.Int("fetch.groups", len(groups)),
	)

	metrics.EnricherGroupsPerFetch.Observe(float64(len(groups)))

	enrichStart := time.Now()
	for i, group := range groups {
		if i > 0 {
			fetchLog.Debug("inter-group sleep",
				logger.Dur(logger.FieldSleep, w.llmCallInterval),
				logger.Int(logger.FieldChunkIndex, i),
			)
			if serr := w.clock.Sleep(fetchCtx, w.llmCallInterval); serr != nil {
				// ctx cancelled mid-sleep; unacked messages are redelivered by
				// NATS after AckWait expires — no manual intervention needed.
				return nil
			}
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					w.log.Error("panic in processGroup, recovering",
						fmt.Errorf("panic: %v", r),
						logger.Str(logger.FieldFetchID, fetchID),
						logger.Int(logger.FieldChunkIndex, i),
						logger.Str(logger.FieldStack, string(debug.Stack())),
					)
					for _, fj := range group {
						w.markError(fetchCtx, fj.job.ID)
						if terr := fj.msg.Term(); terr != nil {
							w.log.Warn("msg term failed", logger.Err(terr))
						}
					}
				}
			}()
			w.processGroup(fetchCtx, fetchLog, group, fetchID, i, len(groups))
		}()

		if fetchCtx.Err() != nil {
			return nil
		}
	}

	fetchLog.Info("fetch processed",
		logger.Int(logger.FieldMsgs, len(msgs)),
		logger.Int(logger.FieldFetched, len(fetched)),
		logger.Int(logger.FieldBatches, len(groups)),
		logger.Dur(logger.FieldFetchDuration, fetchDuration),
		logger.Dur(logger.FieldEnrichDuration, time.Since(enrichStart)),
	)
	return nil
}

// chunkFetched splits the fetched list into groups of at most size each.
func chunkFetched(s []fetchedJob, size int) [][]fetchedJob {
	if size < 1 {
		size = 1
	}
	groups := make([][]fetchedJob, 0, (len(s)+size-1)/size)
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		groups = append(groups, s[i:end])
	}
	return groups
}

// handleFailure routes a single failed job via NATS dispositions:
//   - Validation errors → DLQ immediately (Term).
//   - Within retry budget → Nak (NATS redelivers with consumer BackOff).
//   - Budget exhausted → DLQ (Term).
//
// Only stage="enrich" failures write retry state to job_pipeline_ledger.
// Returns true when the job reached a terminal routing state; false on ctx
// cancel, Nak failure, or DLQ publish failure.
func (w *Worker) handleFailure(ctx context.Context, fetchLog logger.Logger, fj fetchedJob, stage string, err error) bool {
	if stderrors.Is(err, context.Canceled) || ctx.Err() != nil {
		return false
	}

	jobLog := fetchLog.With(
		logger.Str(logger.FieldJobID, fj.job.ID),
		logger.Int(logger.FieldRetryCount, fj.retryCount),
	)

	if stderrors.Is(err, domainerr.ErrLLMValidation) {
		return w.sendToDLQ(ctx, jobLog, fj, "validation", err)
	}

	if fj.retryCount >= analyzerMaxRetries {
		cause := "exhausted"
		if stage == "fetch" {
			cause = "fetch_failed"
		}
		return w.sendToDLQ(ctx, jobLog, fj, cause, err)
	}

	// Within retry budget — Nak so NATS redelivers with consumer BackOff.
	if nerr := fj.msg.Nak(); nerr != nil {
		metrics.EnricherRetryPublishFailures.WithLabelValues("nats", stage).Inc()
		jobLog.Error("nak failed", nerr, logger.Str(logger.FieldStage, stage))
		return false
	}
	metrics.EnricherRetryPublishesTotal.WithLabelValues("nats", stage).Inc()

	nextAttemptAt := w.clock.Now().Add(analyzerBackOff[fj.retryCount])

	if stage == "enrich" {
		entry := jobsvc.RetryLedgerEntry{
			JobID:         fj.job.ID,
			RetryCount:    fj.retryCount + 1,
			NextAttemptAt: nextAttemptAt,
		}
		if lerr := w.markLedgerRetryWithRetry(ctx, entry); lerr != nil {
			metrics.EnricherRetryLedgerFailuresTotal.Inc()
			jobLog.Error("mark ledger retry failed after retry", lerr,
				logger.Int(logger.FieldRetryCount, fj.retryCount+1),
			)
		}
	}

	jobLog.Info("scheduled retry via NATS BackOff",
		logger.Str(logger.FieldNextAttemptAt, nextAttemptAt.UTC().Format(time.RFC3339)),
		logger.Str(logger.FieldStage, stage),
	)
	return true
}

// markLedgerRetryWithRetry attempts UpdateRetryBatch with one inline retry on
// transient failure before giving up. NATS is authoritative for retry scheduling;
// a stale ledger row is a brief observability gap, not a correctness issue.
func (w *Worker) markLedgerRetryWithRetry(ctx context.Context, entry jobsvc.RetryLedgerEntry) error {
	err := w.ledger.UpdateRetryBatch(ctx, []jobsvc.RetryLedgerEntry{entry})
	if err == nil {
		return nil
	}
	// A missing ledger row is permanent, not transient — retrying the UPDATE
	// won't make the row appear. Record the metric and return without the second
	// attempt. NATS remains authoritative for retry scheduling.
	if stderrors.Is(err, domainerr.ErrLedgerRowMissing) {
		metrics.EnricherLedgerMissing.WithLabelValues(jobcommon.SeenReasonRetry).Inc()
		w.log.Warn("ledger row missing on retry update — skipping second attempt",
			logger.Str(logger.FieldJobID, entry.JobID),
			logger.Str(logger.FieldReason, jobcommon.SeenReasonRetry),
		)
		return err
	}
	if sleepErr := w.clock.Sleep(ctx, 100*time.Millisecond); sleepErr != nil {
		return err
	}
	return w.ledger.UpdateRetryBatch(ctx, []jobsvc.RetryLedgerEntry{entry})
}

// sendToDLQ publishes the job to the DLQ stream and Terms the NATS message.
// If the DLQ publish fails the message is NOT Termed so NATS will redeliver.
// Returns true on successful publish + Term.
func (w *Worker) sendToDLQ(ctx context.Context, jobLog logger.Logger, fj fetchedJob, cause string, err error) bool {
	payload, merr := json.Marshal(fj.job)
	if merr != nil {
		jobLog.Error("marshal job for DLQ failed, terminating message", merr)
		if terr := fj.msg.Term(); terr != nil {
			jobLog.Warn("msg term failed", logger.Err(terr))
		}
		w.markError(ctx, fj.job.ID)
		return false
	}

	if perr := w.publisher.Publish(ctx, js.SubjectJobsDLQ, payload); perr != nil {
		jobLog.Error("publish DLQ failed", perr, logger.Str(logger.FieldCause, cause))
		return false
	}
	metrics.EnricherDLQPublishesTotal.WithLabelValues(cause).Inc()
	if terr := fj.msg.Term(); terr != nil {
		jobLog.Warn("msg term failed", logger.Err(terr))
	}
	w.markError(ctx, fj.job.ID)
	jobLog.Warn("moved to DLQ", logger.Err(err), logger.Str(logger.FieldCause, cause))
	metrics.EnricherJobsTotal.WithLabelValues("dlq").Inc()
	return true
}

// ledgerMissingTolerated reports whether err is ErrLedgerRowMissing. At the
// enricher stage a missing ledger row (cleaned out by a maintenance migration,
// or never written) is non-fatal: it logs WARN, increments a metric, and the
// worker proceeds rather than crashing. Any other error is logged at ERROR.
func (w *Worker) ledgerMissingTolerated(err error, jobID, reason string) {
	if err == nil {
		return
	}
	if stderrors.Is(err, domainerr.ErrLedgerRowMissing) {
		metrics.EnricherLedgerMissing.WithLabelValues(reason).Inc()
		w.log.Warn("ledger row missing — continuing (non-fatal at enricher stage)",
			logger.Str(logger.FieldJobID, jobID),
			logger.Str(logger.FieldReason, reason),
		)
		return
	}
	w.log.Error("failed to mark ledger in seen_ledger", err,
		logger.Str(logger.FieldJobID, jobID),
		logger.Str(logger.FieldReason, reason),
	)
}

// markError updates the ledger so failed jobs aren't re-processed endlessly.
func (w *Worker) markError(ctx context.Context, jobID string) {
	w.ledgerMissingTolerated(w.pipeline.MarkLedgerError(ctx, jobID), jobID, jobcommon.SeenReasonError)
}

// markIrrelevant marks a job as llm_irrelevant so it is skipped cleanly without
// counting as an error.
func (w *Worker) markIrrelevant(ctx context.Context, jobID string) {
	w.ledgerMissingTolerated(w.pipeline.MarkLedgerIrrelevant(ctx, jobID), jobID, jobcommon.SeenReasonLLMIrrelevant)
}

// markNonUSA marks a job as non_usa_location so it is skipped cleanly without
// counting as an error.
func (w *Worker) markNonUSA(ctx context.Context, jobID string) {
	w.ledgerMissingTolerated(w.pipeline.MarkLedgerNonUSA(ctx, jobID), jobID, jobcommon.SeenReasonNonUSA)
}
