package main

import (
	"context"
	stderrors "errors"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmcore "github.com/trushilpatel/jobradar/service/llm/services"
)

// clock abstracts wall-clock reads and ctx-aware sleeps so tests can control
// both the inter-group pacing and the NextAttemptAt value stamped into the
// retry header + ledger row.
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

// recordCommitter is the slice of *kafka.Consumer the worker actually needs
// for per-group commits. Kept as a package-local interface so tests can
// substitute a fake without depending on the full Consumer type.
type recordCommitter interface {
	CommitRecords(ctx context.Context, records ...*kgo.Record) error
}

// WorkerOptions collects the dependencies required to build a Worker. Using
// an options struct keeps NewWorker's signature stable as the worker grows.
type WorkerOptions struct {
	Consumer         *kafka.Consumer
	Publisher        kafka.Publisher
	Scorer           llmcore.JobPostEnricher
	Pipeline         jobsvc.PipelineService
	Ledger           EnricherLedger
	RateLimiter      *AdapterRateLimiter
	Log              logger.Logger
	MaxExpYears      int
	FetchConcurrency int
	LLMBatchSize     int
	LLMCallInterval  time.Duration
}

// Worker consumes job_listings.filtered, runs each Kafka fetch through two
// stages, and commits offsets per enriched group:
//
//  1. Fetch stage  — parallel HTTP fetches, per-adapter rate limited.
//     Failures route to retry/DLQ for that single job (Stage 1 — stage="fetch").
//  2. Enrich stage — successful (job, content) pairs are grouped into batches
//     of llmBatchSize and sent through EnrichBatch serially, paced by
//     llmCallInterval. Per-group commits shrink restart replay window from the
//     whole fetch to a single group. Per-job failures route via retry ladder
//     or DLQ (Stage 2 — stage="enrich") and write retry state to
//     job_pipeline_ledger.
type Worker struct {
	consumer        *kafka.Consumer
	committer       recordCommitter
	publisher       kafka.Publisher
	scorer          llmcore.JobPostEnricher
	pipeline        jobsvc.PipelineService
	ledger          EnricherLedger
	rateLimiter     *AdapterRateLimiter
	client          *http.Client
	log             logger.Logger
	clock           clock
	maxExpYears     int
	fetchConc       int
	llmBatchSize    int
	llmCallInterval time.Duration
}

// NewWorker constructs the analyzer worker from an options struct.
//
//   - FetchConcurrency  caps in-flight HTTP fetches inside one PollBatch
//     cycle. IO-bound — set higher than enrich (e.g. 15).
//   - LLMBatchSize       jobs grouped per single EnrichBatch call. 1 falls
//     back to single-job behaviour without any code change.
//   - LLMCallInterval    inter-group sleep between successive EnrichBatch
//     calls; provides a wall-clock floor independent of the LLM layer's
//     slot/cooldown pacing.
//   - RateLimiter        shared per-adapter limiter so concurrent fetches
//     don't trip ATS rate limits.
func NewWorker(opts WorkerOptions) *Worker {
	if opts.FetchConcurrency < 1 {
		opts.FetchConcurrency = 1
	}
	if opts.LLMBatchSize < 1 {
		opts.LLMBatchSize = 1
	}
	return &Worker{
		consumer:        opts.Consumer,
		committer:       opts.Consumer,
		publisher:       opts.Publisher,
		scorer:          opts.Scorer,
		pipeline:        opts.Pipeline,
		ledger:          opts.Ledger,
		rateLimiter:     opts.RateLimiter,
		client:          &http.Client{Timeout: 15 * time.Second},
		log:             opts.Log,
		clock:           realClock{},
		maxExpYears:     opts.MaxExpYears,
		fetchConc:       opts.FetchConcurrency,
		llmBatchSize:    opts.LLMBatchSize,
		llmCallInterval: opts.LLMCallInterval,
	}
}

// fetchedJob carries an already-fetched job through stage 2. The original
// Kafka record is retained so the worker can commit offsets per-group and
// route to retry/DLQ if the LLM batch fails.
//
// do not hold record across PollBatch boundaries — franz-go reuses the
// record storage after the batch handler returns.
type fetchedJob struct {
	job        ats.RawJob
	content    ats.FetchJobContentResponse
	record     *kgo.Record
	retryCount int
	ctx        context.Context
}

// Run consumes records via PollBatch and processes each fetch synchronously.
// processFetch commits offsets per group; returning nil from the callback
// triggers PollBatch's trailing CommitUncommittedOffsets (a no-op when every
// group already committed, and the escape hatch for the all-Stage-1-failed
// case).
func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker started",
		logger.Int("fetch_concurrency", w.fetchConc),
		logger.Int("llm_batch_size", w.llmBatchSize),
		logger.Dur("llm_call_interval", w.llmCallInterval),
	)

	return w.consumer.PollBatch(ctx, func(records []*kgo.Record) error {
		return w.processFetch(ctx, records)
	})
}

// processFetch is the two-stage pipeline for one Kafka fetch. Stage 1 fetches
// content for all jobs concurrently (rate limited per adapter). Stage 2
// groups the successfully-fetched jobs, sleeps llmCallInterval between
// successive EnrichBatch calls, and commits each group's records after it is
// fully routed.
func (w *Worker) processFetch(ctx context.Context, records []*kgo.Record) (err error) {
	if len(records) == 0 {
		return nil
	}

	fetchID := uuid.NewString()
	fetchLog := w.log.With(logger.Str(logger.FieldFetchID, fetchID))

	var (
		firstOffset int64 = -1
		lastOffset  int64 = -1
		partition   int32
	)
	for _, r := range records {
		if firstOffset < 0 {
			firstOffset = r.Offset
			partition = r.Partition
		}
		if r.Offset > lastOffset {
			lastOffset = r.Offset
		}
	}

	fetchCtx, span := tracing.Start(ctx, "analyzer.worker.processFetch")
	span.SetAttributes(
		attribute.String("fetch.id", fetchID),
		attribute.Int("fetch.records", len(records)),
		attribute.Int("kafka.partition", int(partition)),
	)
	defer tracing.End(span, &err)

	fetchLog.Info("fetch received",
		logger.Int("records", len(records)),
		logger.Int(logger.FieldPartition, int(partition)),
		logger.Int("first_offset", int(firstOffset)),
		logger.Int("last_offset", int(lastOffset)),
	)

	start := time.Now()
	fetched := w.fetchAll(fetchCtx, fetchLog, records)
	fetchDuration := time.Since(start)

	if len(fetched) == 0 {
		fetchLog.Info("fetch processed",
			logger.Int("records", len(records)),
			logger.Int("fetched", 0),
			logger.Dur("fetch_duration", fetchDuration),
		)
		return nil
	}

	// Groups may span partitions; CommitRecords commits per-partition max so
	// this is safe; do not assume one partition per group. Sorting by
	// (partition, offset) keeps group composition deterministic and guarantees
	// per-partition offsets advance monotonically as successive groups commit.
	sortFetched(fetched)

	groups := chunkFetched(fetched, w.llmBatchSize)
	span.SetAttributes(attribute.Int("fetch.fetched", len(fetched)))
	span.SetAttributes(attribute.Int("fetch.groups", len(groups)))

	metrics.EnricherGroupsPerFetch.Observe(float64(len(groups)))

	enrichStart := time.Now()
	for i, group := range groups {
		if i > 0 {
			fetchLog.Debug("inter-group sleep",
				logger.Dur("sleep", w.llmCallInterval),
				logger.Int(logger.FieldChunkIndex, i),
			)
			if serr := w.clock.Sleep(fetchCtx, w.llmCallInterval); serr != nil {
				// ctx cancelled mid-sleep; uncommitted tail replays on next
				// poll. Return nil so PollBatch skips its trailing commit path
				// for the unprocessed records.
				return nil
			}
		}

		routed := w.processGroup(fetchCtx, fetchLog, group, fetchID, i, len(groups))
		if !routed {
			// Ctx cancel mid-EnrichBatch or mid-publish inside handleFailure
			// means this group's jobs did not all reach a terminal routing.
			// Skip commit so Kafka redelivers the whole tail including this
			// group on next poll.
			return nil
		}

		recs := recordsOf(group)
		if cerr := w.committer.CommitRecords(fetchCtx, recs...); cerr != nil {
			metrics.EnricherCommitFailuresTotal.Inc()
			fetchLog.Error("commit failed", cerr,
				logger.Int(logger.FieldChunkIndex, i),
				logger.Int(logger.FieldPartition, int(partition)),
			)
			// Continue — next fetch will redeliver uncommitted records.
			continue
		}
		if len(recs) > 0 {
			last := recs[len(recs)-1]
			fetchLog.Debug("group committed",
				logger.Int(logger.FieldChunkIndex, i),
				logger.Int(logger.FieldChunkCount, len(groups)),
				logger.Int(logger.FieldOffset, int(last.Offset)),
				logger.Int(logger.FieldPartition, int(last.Partition)),
			)
		}
	}

	fetchLog.Info("fetch processed",
		logger.Int("records", len(records)),
		logger.Int("fetched", len(fetched)),
		logger.Int("batches", len(groups)),
		logger.Dur("fetch_duration", fetchDuration),
		logger.Dur("enrich_duration", time.Since(enrichStart)),
	)
	return nil
}

// sortFetched stable-sorts by (partition, offset) so groups composed off the
// slice commit in per-partition ascending order. Exported-for-test helper.
func sortFetched(s []fetchedJob) {
	sort.SliceStable(s, func(i, j int) bool {
		ri, rj := s[i].record, s[j].record
		if ri == nil || rj == nil {
			return false
		}
		if ri.Partition != rj.Partition {
			return ri.Partition < rj.Partition
		}
		return ri.Offset < rj.Offset
	})
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

// recordsOf returns the *kgo.Record slice for a group, used to commit offsets
// after the group completes processing.
func recordsOf(group []fetchedJob) []*kgo.Record {
	out := make([]*kgo.Record, 0, len(group))
	for _, fj := range group {
		if fj.record != nil {
			out = append(out, fj.record)
		}
	}
	return out
}

// ── retry / DLQ routing ──────────────────────────────────────────────────────

// handleFailure routes a single failed job to the appropriate Kafka topic.
// Validation errors skip retries entirely and go straight to DLQ (cause=validation).
// Transient errors move through the retry ladder; exhausted attempts go to DLQ.
// The `stage` argument distinguishes Stage 1 ("fetch") from Stage 2 ("enrich"):
//   - Only Stage 2 writes retry state to job_pipeline_ledger via UpdateRetryBatch.
//   - Only Stage 1's exhausted DLQ publishes carry cause="fetch_failed" (caller
//     drives the DLQ cause via sendToDLQ).
//
// Context cancellation during shutdown short-circuits — Kafka will redeliver
// on restart rather than us racing to publish during shutdown.
//
// Returns true when the job reached a terminal state (retry-published OK or
// DLQ'd OK). False on ctx cancel or publish failure — callers use this to
// decide whether to skip the group commit (preserving at-least-once).
func (w *Worker) handleFailure(ctx context.Context, fetchLog logger.Logger, job ats.RawJob, value []byte, retryCount int, stage string, err error) bool {
	if stderrors.Is(err, context.Canceled) || ctx.Err() != nil {
		return false
	}

	jobLog := fetchLog.With(
		logger.Str(logger.FieldJobID, job.ID),
		logger.Int(logger.FieldRetryCount, retryCount),
	)

	if stderrors.Is(err, domainerr.ErrLLMValidation) {
		return w.sendToDLQ(ctx, jobLog, job.ID, value, retryCount, "validation", err)
	}

	tiers := kafka.AnalyzerRetryTiers()
	if retryCount >= len(tiers) {
		cause := "exhausted"
		if stage == "fetch" {
			cause = "fetch_failed"
		}
		return w.sendToDLQ(ctx, jobLog, job.ID, value, retryCount, cause, err)
	}

	tier := tiers[retryCount]
	tierName := tier.Name()
	newCount := retryCount + 1
	nextAttemptAt := w.clock.Now().Add(tier.Delay)
	headers := kafka.BuildRetryHeaders(newCount, nextAttemptAt)

	if perr := w.publisher.PublishWithHeaders(ctx, tier.Topic, value, headers); perr != nil {
		metrics.EnricherRetryPublishFailures.WithLabelValues(tierName, stage).Inc()
		jobLog.Warn("publish retry failed",
			logger.Err(perr),
			logger.Str(logger.FieldRetryTier, tierName),
			logger.Str("stage", stage),
		)
		return false
	}
	metrics.EnricherRetryPublishesTotal.WithLabelValues(tierName, stage).Inc()

	if stage == "enrich" {
		entry := jobsvc.RetryLedgerEntry{
			JobID:         job.ID,
			RetryCount:    newCount,
			NextAttemptAt: nextAttemptAt,
		}
		if lerr := w.markLedgerRetryWithRetry(ctx, entry); lerr != nil {
			metrics.EnricherRetryLedgerFailuresTotal.Inc()
			jobLog.Error("mark ledger retry failed after retry", lerr,
				logger.Int(logger.FieldRetryCount, newCount),
			)
			// Non-fatal — Kafka is authoritative for retry scheduling; the
			// ledger is an observability surface.
		}
	}

	jobLog.Info("scheduled retry",
		logger.Str("topic", tier.Topic),
		logger.Str(logger.FieldRetryTier, tierName),
		logger.Str(logger.FieldNextAttemptAt, nextAttemptAt.UTC().Format(time.RFC3339)),
		logger.Str("stage", stage),
		logger.Dur("delay", tier.Delay),
	)
	return true
}

// markLedgerRetryWithRetry attempts UpdateRetryBatch with one inline retry on
// transient failure before giving up. Kafka publish is authoritative; a stale
// ledger row is a brief observability gap, not a correctness issue.
func (w *Worker) markLedgerRetryWithRetry(ctx context.Context, entry jobsvc.RetryLedgerEntry) error {
	err := w.ledger.UpdateRetryBatch(ctx, []jobsvc.RetryLedgerEntry{entry})
	if err == nil {
		return nil
	}
	// One short inline retry — the ledger is best-effort.
	if sleepErr := w.clock.Sleep(ctx, 100*time.Millisecond); sleepErr != nil {
		return err
	}
	return w.ledger.UpdateRetryBatch(ctx, []jobsvc.RetryLedgerEntry{entry})
}

// sendToDLQ publishes the original message to the DLQ topic and marks the
// ledger row as errored. The `cause` argument labels enricher_dlq_publishes_total
// (validation | exhausted | fetch_failed | llm_result_missing) — see callers.
// Returns true on successful publish.
func (w *Worker) sendToDLQ(ctx context.Context, jobLog logger.Logger, jobID string, value []byte, retryCount int, cause string, err error) bool {
	headers := kafka.BuildDLQHeaders(retryCount, err.Error())
	if perr := w.publisher.PublishWithHeaders(ctx, kafka.TopicJobListingsFilteredDLQ, value, headers); perr != nil {
		jobLog.Error("publish DLQ failed", perr, logger.Str("cause", cause))
		return false
	}
	metrics.EnricherDLQPublishesTotal.WithLabelValues(cause).Inc()
	w.markError(ctx, jobID)
	jobLog.Warn("moved to DLQ", logger.Err(err), logger.Str("cause", cause))
	metrics.EnricherJobsTotal.WithLabelValues("dlq").Inc()
	return true
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
