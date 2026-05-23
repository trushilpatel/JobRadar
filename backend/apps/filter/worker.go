package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"runtime/debug"
	"strings"
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

// filterMaxRetries is the max number of delivery attempts before a job goes to DLQ.
// Must match the BackOff length on JOBS_RAW consumer in common/jetstream/streams.go.
const filterMaxRetries = 3

// filterBackOff mirrors the JOBS_RAW consumer BackOff config so the ledger's
// next_attempt_at field stays meaningful for dashboards.
var filterBackOff = []time.Duration{time.Minute, 10 * time.Minute, time.Hour}

// clock abstracts wall-clock reads and ctx-aware sleeps so tests can control
// both the inter-chunk pacing and the NextAttemptAt value stamped into the
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
	DLQed   []string
	Retried []jobsvc.RetryLedgerEntry
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
		ids := make([]string, len(nonUSAItems))
		for i, it := range nonUSAItems {
			ids[i] = it.job.job.ID
		}
		if err := w.ledger.UpdateSeenReasonBatch(ctx, ids, jobcommon.SeenReasonNonUSA); err != nil {
			w.log.Error("mark non_usa_location batch failed", err,
				logger.Str(logger.FieldFetchID, fetchID),
				logger.Int(logger.FieldChunkIndex, chunkIndex),
			)
		}
		metrics.FilterJobsTotal.WithLabelValues("non_usa").Add(float64(len(nonUSAItems)))
		for _, it := range nonUSAItems {
			if aerr := it.msg.Ack(); aerr != nil {
				w.log.Warn("msg ack failed", logger.Err(aerr))
			}
		}
		w.log.Info("chunk non-USA rejected",
			logger.Str(logger.FieldFetchID, fetchID),
			logger.Int(logger.FieldChunkIndex, chunkIndex),
			logger.Int(logger.FieldNonUSACount, len(nonUSAItems)),
		)
	}

	if len(passThrough) > 0 {
		// Phase 1: publish all pass-through jobs and collect IDs — no Ack yet.
		passThroughIDs := make([]string, len(passThrough))
		for i, it := range passThrough {
			w.publishDetail(ctx, it.job.job)
			passThroughIDs[i] = it.job.job.ID
		}
		// Phase 2: record ledger state before any Ack so a crash between here
		// and the Ack loop redelivers the messages; UpdateSeenReasonBatch is
		// idempotent, so the second write on redeliver is a no-op.
		if err := w.ledger.UpdateSeenReasonBatch(ctx, passThroughIDs, jobcommon.SeenReasonFiltered); err != nil {
			w.log.Error("mark filtered (pass-through) failed", err,
				logger.Str(logger.FieldFetchID, fetchID),
				logger.Int(logger.FieldChunkIndex, chunkIndex),
			)
		}
		// Phase 3: Ack only after the ledger write has been attempted.
		for _, it := range passThrough {
			if aerr := it.msg.Ack(); aerr != nil {
				w.log.Warn("msg ack failed", logger.Err(aerr))
			}
		}
		w.log.Info("chunk pass-through (no title)",
			logger.Str(logger.FieldFetchID, fetchID),
			logger.Int(logger.FieldChunkIndex, chunkIndex),
			logger.Int(logger.FieldPassThroughCount, len(passThrough)),
		)
		metrics.FilterJobsTotal.WithLabelValues("relevant").Add(float64(len(passThrough)))
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
			}
		}

		publishFailed := len(scorable) - len(outcome.DLQed) - len(outcome.Retried)
		w.log.Info("chunk routed",
			logger.Str(logger.FieldFetchID, fetchID),
			logger.Int(logger.FieldChunkIndex, chunkIndex),
			logger.Int(logger.FieldChunkCount, chunkCount),
			logger.Int(logger.FieldRetriedCount, len(outcome.Retried)),
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

	// Phase 1: classify and publish relevant jobs — collect IDs, no Ack yet.
	var irrelevantIDs []string
	for _, item := range scorable {
		if relevant[item.job.job.ID] {
			w.publishDetail(ctx, item.job.job)
		} else {
			irrelevantIDs = append(irrelevantIDs, item.job.job.ID)
		}
	}

	// Phase 2: record ledger state for both buckets before any Ack.
	// UpdateSeenReasonBatch is idempotent — safe to call again on NATS redeliver.
	if len(irrelevantIDs) > 0 {
		if err := w.ledger.UpdateSeenReasonBatch(ctx, irrelevantIDs, jobcommon.SeenReasonLLMIrrelevant); err != nil {
			w.log.Error("mark irrelevant failed", err,
				logger.Str(logger.FieldFetchID, fetchID),
				logger.Int(logger.FieldChunkIndex, chunkIndex),
			)
		}
	}

	if len(relevantIDs) > 0 {
		if err := w.ledger.UpdateSeenReasonBatch(ctx, relevantIDs, jobcommon.SeenReasonFiltered); err != nil {
			w.log.Error("mark filtered (relevant) failed", err,
				logger.Str(logger.FieldFetchID, fetchID),
				logger.Int(logger.FieldChunkIndex, chunkIndex),
			)
		}
	}

	// Phase 3: Ack all scorable messages only after both ledger writes have been attempted.
	for _, item := range scorable {
		if aerr := item.msg.Ack(); aerr != nil {
			w.log.Warn("msg ack failed", logger.Err(aerr))
		}
	}

	metrics.FilterJobsTotal.WithLabelValues("relevant").Add(float64(len(relevantIDs)))
	metrics.FilterJobsTotal.WithLabelValues("irrelevant").Add(float64(len(irrelevantIDs)))

	outcome := "mixed"
	switch {
	case len(relevantIDs) == 0:
		outcome = "irrelevant"
	case len(irrelevantIDs) == 0:
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
		logger.Int(logger.FieldRelevant, len(relevantIDs)),
		logger.Int(logger.FieldIrrelevant, len(irrelevantIDs)),
	)
	return nil
}

// routeFailedChunk routes each job in a failed chunk to retry (Nak) or DLQ (Term).
// NATS BackOff handles redelivery timing — we don't need to publish to retry topics.
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

	for _, item := range chunk {
		retryCount := item.job.retryCount
		if retryCount >= filterMaxRetries {
			if w.publishToDLQ(ctx, item, scoreErr) {
				outcome.DLQed = append(outcome.DLQed, item.job.job.ID)
			}
			continue
		}

		// Within retry budget: Nak so NATS redelivers via BackOff schedule.
		if nerr := item.msg.Nak(); nerr != nil {
			w.log.Warn("nak failed",
				logger.Err(nerr),
				logger.Str(logger.FieldJobID, item.job.job.ID),
			)
			metrics.FilterRetryPublishFailures.WithLabelValues("nak").Inc()
			continue
		}

		newCount := retryCount + 1
		nextAt := w.clock.Now().Add(filterBackOff[retryCount])
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

func (w *Worker) publishDetail(ctx context.Context, job ats.RawJob) {
	payload, err := json.Marshal(job)
	if err != nil {
		w.log.Error("marshal failed", err, logger.Str(logger.FieldJobID, job.ID))
		return
	}
	subject := fmt.Sprintf(js.SubjectJobsFilteredFmt, job.Adapter)
	if err := w.publisher.Publish(ctx, subject, payload); err != nil {
		w.log.Error("publish failed", err, logger.Str(logger.FieldJobID, job.ID))
	}
}

// formatSlotStatuses renders per-slot exhaustion detail for log output.
func formatSlotStatuses(ss []domainerr.LLMSlotStatus) string {
	if len(ss) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ss))
	for _, s := range ss {
		cd := "-"
		if !s.CooldownUntil.IsZero() {
			cd = s.CooldownUntil.UTC().Format(time.RFC3339)
		}
		parts = append(parts, fmt.Sprintf("%s=%s(fail=%d,cd=%s)", s.SlotID, s.LastDenial, s.FailureCount, cd))
	}
	return strings.Join(parts, "; ")
}

// mergedLocations returns the slice to pass to location.AnyNonUSA.
// If AllLocations is populated it is used directly; otherwise the single
// Location string is wrapped in a one-element slice for backward compatibility
// with thin-listing adapters that pre-date the AllLocations field.
func mergedLocations(job ats.RawJob) []string {
	if len(job.AllLocations) > 0 {
		return job.AllLocations
	}
	if job.Location != "" {
		return []string{job.Location}
	}
	return nil
}

// splitBySize divides s into chunks of at most size elements each.
func splitBySize(s []chunkItem, size int) [][]chunkItem {
	if size < 1 {
		size = 1
	}
	chunks := make([][]chunkItem, 0, (len(s)+size-1)/size)
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[i:end])
	}
	return chunks
}
