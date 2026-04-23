package main

import (
	"context"
	stderrors "errors"
	"net/http"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmcore "github.com/trushilpatel/jobradar/service/llm/services"
)

// Worker consumes job_listings.filtered, runs each Kafka fetch through two
// stages, and commits offsets only after both stages finish:
//
//  1. Fetch stage  — parallel HTTP fetches, per-adapter rate limited.
//     Failures route to retry/DLQ for that single job.
//  2. Enrich stage — successful (job, content) pairs are grouped into batches
//     of llmBatchSize and sent through one EnrichBatch call
//     each. The model returns one result per job_id.
//     Per-job: drop check → save or mark dropped.
//
// Stages run sequentially within one PollBatch — Stage 2 begins only after
// Stage 1's WaitGroup releases. This keeps offset-commit semantics simple:
// we commit only after every job in the fetch is either saved, dropped, or
// routed to retry/DLQ.
type Worker struct {
	consumer     *kafka.Consumer
	publisher    kafka.Publisher
	scorer       llmcore.JobPostEnricher
	pipeline     jobsvc.PipelineService
	rateLimiter  *AdapterRateLimiter
	client       *http.Client
	log          logger.Logger
	maxExpYears  int
	fetchConc    int
	llmBatchSize int
}

// NewWorker constructs the analyzer worker.
//
//   - fetchConcurrency  caps in-flight HTTP fetches inside one PollBatch
//     cycle. IO-bound — set higher than enrich (e.g. 15).
//   - llmBatchSize       jobs grouped per single EnrichBatch call. 1 falls
//     back to single-job behaviour without any code change.
//   - rateLimiter        shared per-adapter limiter so concurrent fetches
//     don't trip ATS rate limits.
func NewWorker(
	consumer *kafka.Consumer,
	publisher kafka.Publisher,
	scorer llmcore.JobPostEnricher,
	pipeline jobsvc.PipelineService,
	rateLimiter *AdapterRateLimiter,
	log logger.Logger,
	maxExpYears int,
	fetchConcurrency int,
	llmBatchSize int,
) *Worker {
	if fetchConcurrency < 1 {
		fetchConcurrency = 1
	}
	if llmBatchSize < 1 {
		llmBatchSize = 1
	}
	return &Worker{
		consumer:     consumer,
		publisher:    publisher,
		scorer:       scorer,
		pipeline:     pipeline,
		rateLimiter:  rateLimiter,
		client:       &http.Client{Timeout: 15 * time.Second},
		log:          log,
		maxExpYears:  maxExpYears,
		fetchConc:    fetchConcurrency,
		llmBatchSize: llmBatchSize,
	}
}

// fetchedJob carries an already-fetched job through stage 2. The original
// Kafka payload + retryCount are kept so we can route to retry/DLQ if the
// LLM batch fails.
type fetchedJob struct {
	job        ats.RawJob
	content    ats.FetchJobContentResponse
	value      []byte
	retryCount int
	ctx        context.Context
}

// Run consumes records via PollBatch and processes each fetch synchronously.
// Returning nil from the callback is what triggers the Kafka offset commit,
// so we only return after every record in the fetch has been routed somewhere
// (saved, dropped, marked, or pushed to retry/DLQ).
func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker started",
		logger.Int("fetch_concurrency", w.fetchConc),
		logger.Int("llm_batch_size", w.llmBatchSize),
	)

	return w.consumer.PollBatch(ctx, func(records []*kgo.Record) error {
		w.processFetch(ctx, records)
		return nil
	})
}

// processFetch is the two-stage pipeline for one Kafka fetch. Stage 1 fetches
// content for all jobs concurrently (rate limited per adapter); stage 2 sends
// the successfully-fetched jobs through the LLM in batches.
func (w *Worker) processFetch(ctx context.Context, records []*kgo.Record) {
	if len(records) == 0 {
		return
	}

	fetchCtx, span := tracing.Start(ctx, "analyzer.worker.processFetch")
	defer tracing.End(span, nil)

	start := time.Now()
	fetched := w.fetchAll(fetchCtx, records)
	fetchDuration := time.Since(start)

	if len(fetched) == 0 {
		w.log.Info("fetch processed",
			logger.Int("records", len(records)),
			logger.Int("fetched", 0),
			logger.Dur("fetch_duration", fetchDuration),
		)
		return
	}

	enrichStart := time.Now()
	groups := chunkFetched(fetched, w.llmBatchSize)
	w.enrichAllGroups(fetchCtx, groups)

	w.log.Info("fetch processed",
		logger.Int("records", len(records)),
		logger.Int("fetched", len(fetched)),
		logger.Int("batches", len(groups)),
		logger.Dur("fetch_duration", fetchDuration),
		logger.Dur("enrich_duration", time.Since(enrichStart)),
	)
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

// enrichAllGroups runs each group's EnrichBatch concurrently. Each group is
// a fully independent LLM call — failure of one group never affects another.
// We wait for all groups before returning so the Kafka offset commit only
// fires once everything is routed.
func (w *Worker) enrichAllGroups(ctx context.Context, groups [][]fetchedJob) {
	var wg sync.WaitGroup
	for _, group := range groups {
		wg.Add(1)
		go func(g []fetchedJob) {
			defer wg.Done()
			w.enrichGroup(ctx, g)
		}(group)
	}
	wg.Wait()
}

// ── retry / DLQ routing ──────────────────────────────────────────────────────

// handleFailure routes a single failed job to the appropriate Kafka topic.
// Validation errors (our own bug) skip retries entirely and go straight to DLQ.
// Transient errors move through the retry ladder; exhausted attempts go to DLQ.
// Ledger is only marked `error` for DLQ-bound messages.
//
// Context cancellation during shutdown short-circuits — Kafka will redeliver
// on restart rather than us racing to publish during shutdown.
func (w *Worker) handleFailure(ctx context.Context, job ats.RawJob, value []byte, retryCount int, err error) {
	if stderrors.Is(err, context.Canceled) || ctx.Err() != nil {
		return
	}

	jobLog := w.log.With(
		logger.Str(logger.FieldJobID, job.ID),
		logger.Int("retry_count", retryCount),
	)

	if stderrors.Is(err, domainerr.ErrLLMValidation) {
		w.sendToDLQ(ctx, jobLog, job.ID, value, retryCount, err)
		return
	}

	tiers := kafka.AnalyzerRetryTiers()
	if retryCount >= len(tiers) {
		w.sendToDLQ(ctx, jobLog, job.ID, value, retryCount, err)
		return
	}

	tier := tiers[retryCount]
	nextAttemptAt := time.Now().Add(tier.Delay)
	headers := kafka.BuildRetryHeaders(retryCount+1, nextAttemptAt)
	if perr := w.publisher.PublishWithHeaders(ctx, tier.Topic, value, headers); perr != nil {
		jobLog.Error("publish retry failed", perr, logger.Str("topic", tier.Topic))
		return
	}
	jobLog.Info("scheduled retry",
		logger.Str("topic", tier.Topic),
		logger.Dur("delay", tier.Delay),
	)
}

// sendToDLQ publishes the original message to the DLQ topic and marks the
// ledger row as errored. Used for both validation errors and exhausted retries.
func (w *Worker) sendToDLQ(ctx context.Context, jobLog logger.Logger, jobID string, value []byte, retryCount int, err error) {
	headers := kafka.BuildDLQHeaders(retryCount, err.Error())
	if perr := w.publisher.PublishWithHeaders(ctx, kafka.TopicJobListingsFilteredDLQ, value, headers); perr != nil {
		jobLog.Error("publish DLQ failed", perr)
		return
	}
	w.markError(ctx, jobID)
	jobLog.Warn("moved to DLQ", logger.Err(err))
	metrics.EnricherJobsTotal.WithLabelValues("dlq").Inc()
}

// markError updates the ledger so failed jobs aren't re-processed endlessly.
func (w *Worker) markError(ctx context.Context, jobID string) {
	if err := w.pipeline.MarkLedgerError(ctx, jobID); err != nil {
		w.log.Error("failed to mark error in seen_ledger", err, logger.Str(logger.FieldJobID, jobID))
	}
}

// markIrrelevant marks a job as llm_irrelevant so it is skipped cleanly without
// counting as an error.
func (w *Worker) markIrrelevant(ctx context.Context, jobID string) {
	if err := w.pipeline.MarkLedgerIrrelevant(ctx, jobID); err != nil {
		w.log.Error("failed to mark irrelevant in seen_ledger", err, logger.Str(logger.FieldJobID, jobID))
	}
}
