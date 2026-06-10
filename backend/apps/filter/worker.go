package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
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
	"github.com/trushilpatel/jobradar/service/job/location"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
	llmcore "github.com/trushilpatel/jobradar/service/llm/services"
)

// filterMaxRetries is the max NATS delivery count before a job goes to DLQ
// on the infra-error path. Must equal len(filterBackOff) so that every Nak'd
// delivery has a BackOff entry and the (filterMaxRetries+1)-th delivery is DLQ'd.
// Must stay in sync with MaxDeliver on the JOBS_RAW consumer in
// common/jetstream/streams.go (MaxDeliver:3 → 2 Nak tiers before Term).
const filterMaxRetries = 2

// filterBackOff mirrors the JOBS_RAW consumer BackOff config so the ledger's
// next_attempt_at field stays meaningful for dashboards on the infra-error path.
// Must match the BackOff slice in ConsumerFilterWorkers (common/jetstream/streams.go).
var filterBackOff = []time.Duration{30 * time.Second, 2 * time.Minute}

// Worker consumes JOBS_RAW via NATS pull consumer, splits each fetch into
// chunks of batchSize, and scores each chunk serially via ScoreBatch. An
// inter-chunk sleep paces successive LLM calls. Each message is individually
// Ack'd, Nak'd (retry), or Term'd (DLQ) — no batch commit.
type Worker struct {
	consumer        MessageFetcher
	publisher       js.Publisher
	scorer          llmcore.JobsRelevanceFilter
	ledger          FilterLedger
	batchSize       int
	flushTimeout    time.Duration
	llmCallInterval time.Duration
	clock           clock
	log             logger.Logger
}

func NewWorker(
	consumer MessageFetcher,
	publisher js.Publisher,
	scorer llmcore.JobsRelevanceFilter,
	ledger FilterLedger,
	batchSize int,
	flushTimeout time.Duration,
	llmCallInterval time.Duration,
	log logger.Logger,
) *Worker {
	return &Worker{
		consumer:        consumer,
		publisher:       publisher,
		scorer:          scorer,
		ledger:          ledger,
		batchSize:       batchSize,
		flushTimeout:    flushTimeout,
		llmCallInterval: llmCallInterval,
		clock:           realClock{},
		log:             log,
	}
}

// jobWithCtx pairs a raw job with its retry delivery count.
type jobWithCtx struct {
	ctx        context.Context
	job        ats.RawJob
	retryCount int
}

// chunkItem pairs a jobWithCtx with its NATS message for per-message ack control.
type chunkItem struct {
	job jobWithCtx
	msg *js.Msg
}

// routeOutcome is what routeFailedChunk reports so processChunk can update the ledger.
type routeOutcome struct {
	DLQed              []string
	Retried            []jobsvc.RetryLedgerEntry
	RetriedWithPayload []jobsvc.RetryLedgerEntryWithPayload
}

// Run starts consuming JOBS_RAW and blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker started",
		logger.Int(logger.FieldBatch, w.batchSize),
		logger.Dur(logger.FieldLLMCallInterval, w.llmCallInterval),
	)

	// Fetch batchSize*4 per cycle so we get several chunks worth of work per poll,
	// preserving the inter-chunk sleep behaviour between ScoreBatch calls.
	fetchN := w.batchSize * 4

	for {
		if ctx.Err() != nil {
			return nil
		}

		msgs, err := w.consumer.Fetch(fetchN, w.flushTimeout)
		if err != nil {
			w.log.Error("fetch failed", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
			continue
		}
		if len(msgs) == 0 {
			continue
		}

		if err := w.processFetch(ctx, msgs); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			w.log.Error("process fetch error", err)
		}
	}
}

// processFetch parses all messages from one fetch, splits them into chunks of
// batchSize, and processes each chunk serially with a clock-gated inter-chunk
// sleep. Each message is Ack'd, Nak'd, or Term'd inside processChunk so a crash
// only replays messages whose disposition was not yet set.
func (w *Worker) processFetch(ctx context.Context, msgs []*js.Msg) (err error) {
	fetchID := uuid.NewString()
	fetchLog := w.log.With(logger.Str(logger.FieldFetchID, fetchID))

	items := make([]chunkItem, 0, len(msgs))
	// TODO: extract W3C trace context from msg headers via tracing.ExtractNATS once available
	for _, msg := range msgs {
		var job ats.RawJob
		if jerr := json.Unmarshal(msg.Data(), &job); jerr != nil {
			fetchLog.Warn("malformed message, routing to DLQ", logger.Err(jerr))
			if perr := w.publisher.Publish(ctx, js.SubjectJobsDLQ, msg.Data()); perr != nil {
				fetchLog.Error("publish malformed to DLQ failed", perr)
			}
			if terr := msg.Term(); terr != nil {
				fetchLog.Warn("msg term failed", logger.Err(terr))
			}
			continue
		}
		items = append(items, chunkItem{
			job: jobWithCtx{
				ctx:        ctx,
				job:        job,
				retryCount: msg.DeliveryCount() - 1,
			},
			msg: msg,
		})
	}

	if len(items) == 0 {
		return nil
	}

	chunks := splitBySize(items, w.batchSize)

	fetchCtx, span := tracing.Start(ctx, "filter.worker.processFetch")
	span.SetAttributes(
		attribute.String("fetch.id", fetchID),
		attribute.Int("fetch.records", len(msgs)),
		attribute.Int("fetch.chunks", len(chunks)),
	)
	defer tracing.End(span, &err)

	metrics.FilterChunksPerFetch.Observe(float64(len(chunks)))

	fetchLog.Info("fetch received",
		logger.Int(logger.FieldRecords, len(msgs)),
		logger.Int(logger.FieldChunkCount, len(chunks)),
	)

	for i, chunk := range chunks {
		if i > 0 {
			fetchLog.Debug("inter-chunk sleep",
				logger.Dur(logger.FieldSleep, w.llmCallInterval),
				logger.Int(logger.FieldChunkIndex, i),
			)
			if serr := w.clock.Sleep(fetchCtx, w.llmCallInterval); serr != nil {
				w.nakRemainingChunks(chunks[i:], fetchID, "shutdown during inter-chunk sleep")
				return serr
			}
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					ids := make([]string, 0, len(chunk))
					for _, it := range chunk {
						ids = append(ids, it.job.job.ID)
					}
					w.log.Error("panic in processChunk, recovering",
						fmt.Errorf("panic: %v", r),
						logger.Str(logger.FieldFetchID, fetchID),
						logger.Int(logger.FieldChunkIndex, i),
						logger.Str(logger.FieldStack, string(debug.Stack())),
					)
					if merr := w.ledger.UpdateSeenReasonBatch(fetchCtx, ids, jobcommon.SeenReasonError); merr != nil {
						w.log.Error("mark error batch failed after panic", merr,
							logger.Str(logger.FieldFetchID, fetchID),
							logger.Int(logger.FieldChunkIndex, i),
						)
					}
					// Nak all messages so NATS redelivers.
					for _, it := range chunk {
						if nerr := it.msg.Nak(); nerr != nil {
							w.log.Warn("msg nak failed", logger.Err(nerr))
						}
					}
				}
			}()
			// processChunk handles all errors internally (routing to retry/DLQ);
			// a non-nil return means an unexpected internal failure worth logging.
			if cerr := w.processChunk(fetchCtx, chunk, fetchID, i, len(chunks)); cerr != nil {
				fetchLog.Error("processChunk returned unexpected error", cerr,
					logger.Int(logger.FieldChunkIndex, i),
				)
			}
		}()

		if fetchCtx.Err() != nil {
			w.nakRemainingChunks(chunks[i+1:], fetchID, "shutdown between chunks")
			return nil
		}
	}

	return nil
}

// processChunk owns one chunk end-to-end: ScoreBatch → publish relevant /
// mark irrelevant, or route errors through retry ladder / DLQ. Each message
// is Ack'd (done), Nak'd (retry), or Term'd (DLQ) before this returns.
func (w *Worker) processChunk(ctx context.Context, chunk []chunkItem, fetchID string, chunkIndex, chunkCount int) (err error) {
	if len(chunk) == 0 {
		return nil
	}

	retryMax := 0
	for _, it := range chunk {
		if it.job.retryCount > retryMax {
			retryMax = it.job.retryCount
		}
	}

	ctx, sp := tracing.Start(ctx, "filter.worker.processChunk")
	sp.SetAttributes(
		attribute.Int("chunk.index", chunkIndex),
		attribute.Int("chunk.size", len(chunk)),
		attribute.Int("chunk.retry_count_max", retryMax),
	)
	defer tracing.End(sp, &err)

	start := w.clock.Now()

	var scorable, passThrough []chunkItem
	var nonUSAItems []chunkItem
	for _, item := range chunk {
		// Non-USA gate: only applied when LocationKnown=true (adapter-supplied
		// structured location). When LocationKnown=false (Workday/Meta thin listing),
		// location is unknown at this stage and must not be rejected here.
		if item.job.job.LocationKnown && location.AnyNonUSA(mergedLocations(item.job.job)) {
			nonUSAItems = append(nonUSAItems, item)
			continue
		}
		if item.job.job.Title == "" {
			passThrough = append(passThrough, item)
		} else {
			scorable = append(scorable, item)
		}
	}

	if len(nonUSAItems) > 0 {
		// markSeenOrNak handles ledger write + Ack-on-success / Nak-on-error,
		// preventing the bug where a ledger failure still Acked the messages.
		w.markSeenOrNak(ctx, nonUSAItems, jobcommon.SeenReasonNonUSA, fetchID, chunkIndex)
		metrics.FilterJobsTotal.WithLabelValues("non_usa").Add(float64(len(nonUSAItems)))
		w.log.Info("chunk non-USA rejected",
			logger.Str(logger.FieldFetchID, fetchID),
			logger.Int(logger.FieldChunkIndex, chunkIndex),
			logger.Int(logger.FieldNonUSACount, len(nonUSAItems)),
		)
	}

	if len(passThrough) > 0 {
		// Partition into published-OK and publish-failed BEFORE any ledger write.
		// Publish-failed jobs are Nak'd (NATS redelivers) with no ledger write —
		// the row stays at its prior reason and flows through on redelivery.
		publishedItems, _ := w.publishPartition(ctx, passThrough)
		// Mark only the published bucket 'filtered'; Nak (don't Ack) any whose
		// ledger row is missing so NATS redelivers after the crawler INSERT lands.
		w.markSeenOrNak(ctx, publishedItems, jobcommon.SeenReasonFiltered, fetchID, chunkIndex)
		w.log.Info("chunk pass-through (no title)",
			logger.Str(logger.FieldFetchID, fetchID),
			logger.Int(logger.FieldChunkIndex, chunkIndex),
			logger.Int(logger.FieldPassThroughCount, len(passThrough)),
		)
		metrics.FilterJobsTotal.WithLabelValues("relevant").Add(float64(len(publishedItems)))
	}

	if len(scorable) == 0 {
		metrics.FilterChunkDuration.WithLabelValues("relevant").Observe(w.clock.Now().Sub(start).Seconds())
		return nil
	}

	inputs := make([]llmdto.JobsRelevanceFilterInput, len(scorable))
	for i, item := range scorable {
		inputs[i] = llmdto.JobsRelevanceFilterInput{
			ID:       item.job.job.ID,
			Title:    item.job.job.Title,
			Location: item.job.job.Location,
		}
	}

	scoreCtx, scoreSpan := tracing.Start(ctx, "filter.worker.scoreBatch")
	scoreSpan.SetAttributes(attribute.Int("batch.size", len(scorable)))
	relevantIDs, err := w.scorer.ScoreBatch(scoreCtx, inputs)
	if err != nil {
		scoreSpan.SetAttributes(attribute.String("outcome", "error"))
		scoreSpan.RecordError(err)
		scoreSpan.End()

		fields := []logger.Field{
			logger.Str(logger.FieldFetchID, fetchID),
			logger.Int(logger.FieldChunkIndex, chunkIndex),
			logger.Int(logger.FieldChunkCount, chunkCount),
			logger.Int(logger.FieldBatch, len(scorable)),
		}
		var exhausted *domainerr.LLMAllExhaustedError
		if stderrors.As(err, &exhausted) {
			if !exhausted.RetryAfter.IsZero() {
				fields = append(fields,
					logger.Str(logger.FieldRetryAfter, exhausted.RetryAfter.UTC().Format(time.RFC3339)),
					logger.Dur(logger.FieldRetryIn, time.Until(exhausted.RetryAfter)),
				)
			}
			fields = append(fields, logger.Str(logger.FieldSlotsDetail, formatSlotStatuses(exhausted.Slots)))
		}
		w.log.Error("ScoreBatch failed, routing chunk", err, fields...)

		routeCtx, routeSpan := tracing.Start(ctx, "filter.worker.routeFailed")
		outcome := w.routeFailedChunk(routeCtx, scorable, err)
		routeSpan.SetAttributes(
			attribute.Int("dlqed", len(outcome.DLQed)),
			attribute.Int("retried", len(outcome.Retried)),
		)
		routeSpan.End()

		if len(outcome.DLQed) > 0 {
			if merr := w.ledger.UpdateSeenReasonBatch(ctx, outcome.DLQed, jobcommon.SeenReasonError); merr != nil {
				w.log.Error("mark error batch failed", merr,
					logger.Str(logger.FieldFetchID, fetchID),
					logger.Int(logger.FieldChunkIndex, chunkIndex),
				)
				metrics.FilterCommitFailuresTotal.Inc()
			}
			metrics.FilterJobsTotal.WithLabelValues("error").Add(float64(len(outcome.DLQed)))

			cause := "exhausted"
			if stderrors.Is(err, domainerr.ErrLLMValidation) {
				cause = "validation"
			}
			metrics.FilterDLQPublishesTotal.WithLabelValues(cause).Add(float64(len(outcome.DLQed)))
		}

		if len(outcome.Retried) > 0 {
			if rerr := w.ledger.UpdateRetryBatch(ctx, outcome.Retried); rerr != nil {
				w.log.Error("UpdateRetryBatch failed", rerr,
					logger.Str(logger.FieldFetchID, fetchID),
					logger.Int(logger.FieldChunkIndex, chunkIndex),
					logger.Int(logger.FieldEntriesCount, len(outcome.Retried)),
				)
				metrics.FilterCommitFailuresTotal.Inc()
			}
		}

		publishFailed := len(scorable) - len(outcome.DLQed) - len(outcome.Retried) - len(outcome.RetriedWithPayload)
		w.log.Info("chunk routed",
			logger.Str(logger.FieldFetchID, fetchID),
			logger.Int(logger.FieldChunkIndex, chunkIndex),
			logger.Int(logger.FieldChunkCount, chunkCount),
			logger.Int(logger.FieldRetriedCount, len(outcome.Retried)),
			logger.Int(logger.FieldExhaustedCount, len(outcome.RetriedWithPayload)),
			logger.Int(logger.FieldDLQedCount, len(outcome.DLQed)),
			logger.Int(logger.FieldPublishFailedCount, publishFailed),
		)

		metrics.FilterChunkDuration.WithLabelValues("error").Observe(w.clock.Now().Sub(start).Seconds())
		// The ScoreBatch error was fully routed (retry/DLQ); clear err so the
		// processChunk span does not record it as an unhandled propagated error.
		err = nil
		return nil
	}
	scoreSpan.End()

	relevant := make(map[string]bool, len(relevantIDs))
	for _, id := range relevantIDs {
		relevant[id] = true
	}

	// Classify into relevant vs irrelevant.
	var relevantItems, irrelevantItems []chunkItem
	for _, item := range scorable {
		if relevant[item.job.job.ID] {
			relevantItems = append(relevantItems, item)
		} else {
			irrelevantItems = append(irrelevantItems, item)
		}
	}

	// Relevant: publish first, then mark only the published bucket 'filtered'.
	// Publish-failed jobs are Nak'd with no ledger write (NATS redelivers).
	publishedItems, _ := w.publishPartition(ctx, relevantItems)
	w.markSeenOrNak(ctx, publishedItems, jobcommon.SeenReasonFiltered, fetchID, chunkIndex)

	// Irrelevant: no publish; mark 'llm_irrelevant'. A missing ledger row Naks
	// (crawler→filter race) so NATS redelivers once the INSERT lands.
	w.markSeenOrNak(ctx, irrelevantItems, jobcommon.SeenReasonLLMIrrelevant, fetchID, chunkIndex)

	metrics.FilterJobsTotal.WithLabelValues("relevant").Add(float64(len(publishedItems)))
	metrics.FilterJobsTotal.WithLabelValues("irrelevant").Add(float64(len(irrelevantItems)))

	outcome := "mixed"
	switch {
	case len(relevantItems) == 0:
		outcome = "irrelevant"
	case len(irrelevantItems) == 0:
		outcome = "relevant"
	}
	scoreSpan.SetAttributes(attribute.String("outcome", outcome))
	metrics.FilterChunkDuration.WithLabelValues(outcome).Observe(w.clock.Now().Sub(start).Seconds())
	metrics.FilterLLMDuration.Observe(w.clock.Now().Sub(start).Seconds())

	w.log.Info("chunk scored",
		logger.Str(logger.FieldFetchID, fetchID),
		logger.Int(logger.FieldChunkIndex, chunkIndex),
		logger.Int(logger.FieldChunkCount, chunkCount),
		logger.Int(logger.FieldBatch, len(scorable)),
		logger.Int(logger.FieldRelevant, len(relevantItems)),
		logger.Int(logger.FieldIrrelevant, len(irrelevantItems)),
	)
	return nil
}

// publishPartition publishes each item's job to its filtered subject and splits
// the items into published-OK and publish-failed buckets. Publish-failed jobs
// are Nak'd immediately (NATS redelivers after AckWait) and are NOT given a
// ledger write — the row stays at its prior reason. A new metric tracks the
// failure for alerting.
func (w *Worker) publishPartition(ctx context.Context, items []chunkItem) (published, failed []chunkItem) {
	for _, it := range items {
		if err := w.publishDetail(ctx, it.job.job); err != nil {
			metrics.FilterPublishFailures.WithLabelValues("filtered").Inc()
			if nerr := it.msg.Nak(); nerr != nil {
				// Broker state diverged — Term so the message is not stuck. The
				// next scan re-discovers the job (the ledger row stayed put).
				w.log.Warn("nak after publish failure failed; terming", logger.Err(nerr),
					logger.Str(logger.FieldJobID, it.job.job.ID))
				if terr := it.msg.Term(); terr != nil {
					w.log.Warn("msg term failed", logger.Err(terr))
				}
			}
			failed = append(failed, it)
			continue
		}
		published = append(published, it)
	}
	return published, failed
}

// markSeenOrNak records reason for every item's job ID via UpdateSeenReasonBatch.
// On success the chunk's messages are Acked. On ANY ledger error the messages
// are Nak'd so NATS redelivers and the UPDATE is re-attempted on the next
// delivery — Acking on a failed UPDATE would leave the row stuck at the prior
// reason (the phantom 'fetch' / "Awaiting Filter" bug). ErrLedgerRowMissing is
// reported separately (crawler→filter race) for metrics, but takes the same
// Nak path as any other failure.
func (w *Worker) markSeenOrNak(ctx context.Context, items []chunkItem, reason jobcommon.SeenReason, fetchID string, chunkIndex int) {
	if len(items) == 0 {
		return
	}
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.job.job.ID
	}
	err := w.ledger.UpdateSeenReasonBatch(ctx, ids, reason)
	if err != nil {
		if stderrors.Is(err, domainerr.ErrLedgerRowMissing) {
			metrics.FilterLedgerMissing.WithLabelValues(string(reason)).Inc()
			w.log.Warn("ledger row missing — naking chunk for redelivery (crawler race)",
				logger.Str(logger.FieldFetchID, fetchID),
				logger.Int(logger.FieldChunkIndex, chunkIndex),
				logger.Str(logger.FieldReason, string(reason)),
			)
		} else {
			w.log.Error("mark seen batch failed — naking chunk for redelivery", err,
				logger.Str(logger.FieldFetchID, fetchID),
				logger.Int(logger.FieldChunkIndex, chunkIndex),
				logger.Str(logger.FieldReason, string(reason)),
			)
		}
		for _, it := range items {
			if nerr := it.msg.Nak(); nerr != nil {
				w.log.Warn("msg nak failed", logger.Err(nerr), logger.Str(logger.FieldJobID, it.job.job.ID))
			}
		}
		return
	}
	for _, it := range items {
		if aerr := it.msg.Ack(); aerr != nil {
			w.log.Warn("msg ack failed", logger.Err(aerr))
		}
	}
}

// routeFailedChunk routes each job in a failed chunk based on the error type:
//
//   - ErrLLMValidation: DLQ (Term) for every job regardless of retry count.
//   - LLMAllExhaustedError: Term each message and record a payload-bearing retry
//     ledger entry so the sweeper can re-publish without re-fetching the job.
//     Does NOT Nak — the sweeper owns re-injection, not NATS BackOff.
//   - Other (infra) errors: Nak within retry budget (NATS BackOff), DLQ when exhausted.
func (w *Worker) routeFailedChunk(ctx context.Context, chunk []chunkItem, scoreErr error) routeOutcome {
	var outcome routeOutcome

	if stderrors.Is(scoreErr, domainerr.ErrLLMValidation) {
		for _, item := range chunk {
			if w.publishToDLQ(ctx, item, scoreErr) {
				outcome.DLQed = append(outcome.DLQed, item.job.job.ID)
			}
		}
		return outcome
	}

	// LLM exhaustion path: write payload to the ledger FIRST, then Term.
	// The sweeper picks these up via FetchDueRetryJobs and re-publishes once
	// LLM quota recovers. Do NOT Nak — NATS BackOff redelivery is not the
	// right recovery mechanism here (quota is shared; re-delivering sooner
	// just burns quota again).
	//
	// Ordering invariant: ledger write → Term. If the ledger write fails,
	// Nak each message (NATS redelivers) so the job is not permanently lost.
	var exhausted *domainerr.LLMAllExhaustedError
	if stderrors.As(scoreErr, &exhausted) {
		retryAfter := exhausted.RetryAfter
		if retryAfter.IsZero() {
			// No specific retry time: fall back to the first BackOff tier.
			retryAfter = w.clock.Now().Add(filterBackOff[0])
		}

		// Phase 1: marshal each job's payload. Items that fail marshal are
		// Term'd immediately — they can never be recovered via the sweeper.
		type marshaledItem struct {
			item  chunkItem
			entry jobsvc.RetryLedgerEntryWithPayload
			hasPL bool
		}
		marshaledItems := make([]marshaledItem, 0, len(chunk))
		for _, item := range chunk {
			payload, merr := json.Marshal(item.job.job)
			if merr != nil {
				w.log.Error("marshal for exhaustion retry failed; terming without ledger write", merr,
					logger.Str(logger.FieldJobID, item.job.job.ID),
				)
				if terr := item.msg.Term(); terr != nil {
					w.log.Warn("msg term failed", logger.Err(terr))
				}
				marshaledItems = append(marshaledItems, marshaledItem{item: item, hasPL: false})
				continue
			}
			marshaledItems = append(marshaledItems, marshaledItem{
				item: item,
				entry: jobsvc.RetryLedgerEntryWithPayload{
					JobID:         item.job.job.ID,
					Adapter:       item.job.job.Adapter,
					Payload:       json.RawMessage(payload),
					RetryCount:    item.job.retryCount + 1,
					NextAttemptAt: retryAfter,
				},
				hasPL: true,
			})
		}

		// Collect entries for items that marshaled successfully.
		var entries []jobsvc.RetryLedgerEntryWithPayload
		for _, mi := range marshaledItems {
			if mi.hasPL {
				entries = append(entries, mi.entry)
			}
		}

		if len(entries) > 0 {
			// Phase 2: ledger write BEFORE any Term. If the write fails, Nak all
			// entry-bearing items so NATS redelivers and the jobs are not lost.
			if lerr := w.ledger.UpdateRetryBatchWithPayload(ctx, entries); lerr != nil {
				w.log.Error("UpdateRetryBatchWithPayload failed on exhaustion path; naking for redelivery", lerr)
				metrics.FilterCommitFailuresTotal.Inc()
				for _, mi := range marshaledItems {
					if mi.hasPL {
						if nerr := mi.item.msg.Nak(); nerr != nil {
							w.log.Warn("msg nak failed after ledger write failure", logger.Err(nerr),
								logger.Str(logger.FieldJobID, mi.item.job.job.ID),
							)
						}
					}
				}
				metrics.FilterLLMExhaustionTotal.Add(float64(len(chunk)))
				return outcome
			}

			// Phase 3: ledger write succeeded — Term each successfully-written message.
			for _, mi := range marshaledItems {
				if mi.hasPL {
					if terr := mi.item.msg.Term(); terr != nil {
						w.log.Warn("msg term failed on exhaustion path", logger.Err(terr),
							logger.Str(logger.FieldJobID, mi.item.job.job.ID),
						)
					}
				}
			}
			outcome.RetriedWithPayload = entries
		}
		metrics.FilterLLMExhaustionTotal.Add(float64(len(chunk)))
		return outcome
	}

	for _, item := range chunk {
		retryCount := item.job.retryCount
		if retryCount >= filterMaxRetries {
			if w.publishToDLQ(ctx, item, scoreErr) {
				outcome.DLQed = append(outcome.DLQed, item.job.job.ID)
			}
			continue
		}

		newCount := retryCount + 1
		nextAt := w.clock.Now().Add(filterBackOff[retryCount])

		// Within retry budget: Nak so NATS redelivers via BackOff schedule.
		if nerr := item.msg.Nak(); nerr != nil {
			// Nak failed — the broker's view of this message has diverged, so we
			// can't rely on redelivery. Term it explicitly to release the message
			// AND still record the retry ledger row so the next pipeline pass
			// (a future scan re-discovering the job) is the recovery mechanism,
			// rather than silently dropping the job.
			w.log.Warn("nak failed; terming and recording retry ledger row",
				logger.Err(nerr),
				logger.Str(logger.FieldJobID, item.job.job.ID),
			)
			metrics.FilterRetryPublishFailures.WithLabelValues("nak").Inc()
			if terr := item.msg.Term(); terr != nil {
				w.log.Warn("msg term failed", logger.Err(terr))
			}
			outcome.Retried = append(outcome.Retried, jobsvc.RetryLedgerEntry{
				JobID:         item.job.job.ID,
				RetryCount:    newCount,
				NextAttemptAt: nextAt,
			})
			continue
		}

		outcome.Retried = append(outcome.Retried, jobsvc.RetryLedgerEntry{
			JobID:         item.job.job.ID,
			RetryCount:    newCount,
			NextAttemptAt: nextAt,
		})
		metrics.FilterRetryPublishesTotal.WithLabelValues(fmt.Sprintf("retry_%d", newCount)).Inc()
	}
	return outcome
}

// publishToDLQ publishes the job payload to the DLQ subject and Terms the message.
// Returns true on success. On failure the message is also Term'd to prevent
// endless redelivery of an exhausted job.
func (w *Worker) publishToDLQ(ctx context.Context, item chunkItem, scoreErr error) bool {
	payload, err := json.Marshal(item.job.job)
	if err != nil {
		w.log.Error("marshal for DLQ failed", err, logger.Str(logger.FieldJobID, item.job.job.ID))
		if terr := item.msg.Term(); terr != nil {
			w.log.Warn("msg term failed", logger.Err(terr))
		}
		return false
	}
	if perr := w.publisher.Publish(ctx, js.SubjectJobsDLQ, payload); perr != nil {
		w.log.Warn("publish to DLQ failed",
			logger.Err(perr),
			logger.Str(logger.FieldJobID, item.job.job.ID),
		)
		if terr := item.msg.Term(); terr != nil {
			w.log.Warn("msg term failed", logger.Err(terr))
		}
		return false
	}
	if terr := item.msg.Term(); terr != nil {
		w.log.Warn("msg term failed", logger.Err(terr))
	}
	return true
}

// publishDetail publishes the job to its filtered subject. It returns an error
// on marshal or publish failure so the caller can keep ledger + NATS state
// consistent: a publish-failed job is Nak'd (NATS redelivers) and is NOT marked
// 'filtered' in the ledger.
func (w *Worker) publishDetail(ctx context.Context, job ats.RawJob) error {
	payload, err := json.Marshal(job)
	if err != nil {
		w.log.Error("marshal failed", err, logger.Str(logger.FieldJobID, job.ID))
		return fmt.Errorf("marshal job %s: %w", job.ID, err)
	}
	subject := fmt.Sprintf(js.SubjectJobsFilteredFmt, job.Adapter)
	if err := w.publisher.Publish(ctx, subject, payload); err != nil {
		w.log.Error("publish failed", err, logger.Str(logger.FieldJobID, job.ID))
		return fmt.Errorf("publish job %s: %w", job.ID, err)
	}
	return nil
}
