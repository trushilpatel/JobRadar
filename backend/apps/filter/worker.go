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
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
	llmcore "github.com/trushilpatel/jobradar/service/llm/services"
)

// clock abstracts wall-clock reads and ctx-aware sleeps so tests can control
// both the inter-chunk pacing and the NextAttemptAt value stamped into the
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
// for per-chunk commits. Kept as a package-local interface so tests can
// substitute a fake without depending on the full Consumer type.
type recordCommitter interface {
	CommitRecords(ctx context.Context, records ...*kgo.Record) error
}

// Worker consumes job_listings.raw via PollBatch, splits each fetch into
// chunks of batchSize, and scores each chunk serially via ScoreBatch. An
// inter-chunk sleep paces successive LLM calls. Offsets are committed after
// each chunk so a crash mid-fetch only replays the uncommitted tail.
type Worker struct {
	consumer        *kafka.Consumer
	committer       recordCommitter
	publisher       kafka.Publisher
	scorer          llmcore.JobsRelevanceFilter
	ledger          FilterLedger
	batchSize       int
	llmCallInterval time.Duration
	clock           clock
	log             logger.Logger
}

func NewWorker(
	consumer *kafka.Consumer,
	publisher kafka.Publisher,
	scorer llmcore.JobsRelevanceFilter,
	ledger FilterLedger,
	batchSize int,
	llmCallInterval time.Duration,
	log logger.Logger,
) *Worker {
	return &Worker{
		consumer:        consumer,
		committer:       consumer,
		publisher:       publisher,
		scorer:          scorer,
		ledger:          ledger,
		batchSize:       batchSize,
		llmCallInterval: llmCallInterval,
		clock:           realClock{},
		log:             log,
	}
}

// jobWithCtx pairs a raw job with the trace context extracted from its Kafka
// headers and the retry_count value carried on its incoming Kafka record.
// This type stays domain-only — the Kafka record lives on chunkItem.
type jobWithCtx struct {
	ctx        context.Context
	job        ats.RawJob
	retryCount int
}

// chunkItem pairs a jobWithCtx with its source Kafka record so the worker
// can commit offsets per-chunk without polluting jobWithCtx with Kafka types.
type chunkItem struct {
	job    jobWithCtx
	record *kgo.Record
}

// routeOutcome is what routeFailedChunk reports back to processChunk so the
// ledger can be updated correctly. Publish failures are counter-incremented
// and logged; they do not appear in either slice.
type routeOutcome struct {
	DLQed   []string
	Retried []jobsvc.RetryLedgerEntry
}

// Run starts consuming job_listings.raw and blocks until ctx is cancelled.
// Each Kafka fetch is split into chunks of batchSize and processed serially.
func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker started",
		logger.Int(logger.FieldBatch, w.batchSize),
		logger.Dur("llm_call_interval", w.llmCallInterval),
	)

	return w.consumer.PollBatch(ctx, func(records []*kgo.Record) error {
		return w.processFetch(ctx, records)
	})
}

// processFetch parses all records from one Kafka fetch, splits them into
// chunks of batchSize, and processes each chunk serially with a clock-gated
// inter-chunk sleep. After each chunk, the chunk's records are committed so
// a crash only replays the uncommitted tail.
func (w *Worker) processFetch(ctx context.Context, records []*kgo.Record) (err error) {
	fetchID := uuid.NewString()
	fetchLog := w.log.With(logger.Str(logger.FieldFetchID, fetchID))

	items := make([]chunkItem, 0, len(records))
	var malformed []*kgo.Record
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
		lastOffset = r.Offset

		msgCtx := tracing.ExtractKafka(ctx, r.Headers)
		var job ats.RawJob
		if jerr := json.Unmarshal(r.Value, &job); jerr != nil {
			fetchLog.Warn("malformed message, routing to DLQ", logger.Err(jerr))
			headers := []kgo.RecordHeader{
				{Key: kafka.HeaderReason, Value: []byte("malformed")},
				{Key: kafka.HeaderLastError, Value: []byte(jerr.Error())},
			}
			if perr := w.publisher.PublishWithHeaders(ctx, kafka.TopicJobListingsRawDLQ, r.Value, headers); perr != nil {
				fetchLog.Error("publish malformed message to DLQ failed", perr)
			}
			malformed = append(malformed, r)
			continue
		}
		items = append(items, chunkItem{
			job: jobWithCtx{
				ctx:        msgCtx,
				job:        job,
				retryCount: kafka.RetryCountFromHeaders(r.Headers),
			},
			record: r,
		})
	}

	if len(items) == 0 {
		if len(malformed) > 0 {
			if cerr := w.committer.CommitRecords(ctx, malformed...); cerr != nil {
				fetchLog.Error("commit malformed records failed", cerr)
			}
		}
		return nil
	}

	chunks := splitBySize(items, w.batchSize)

	fetchCtx, span := tracing.Start(ctx, "filter.worker.processFetch")
	span.SetAttributes(
		attribute.String("fetch.id", fetchID),
		attribute.Int("fetch.records", len(records)),
		attribute.Int("fetch.chunks", len(chunks)),
		attribute.Int("kafka.partition", int(partition)),
	)
	defer tracing.End(span, &err)

	metrics.FilterChunksPerFetch.Observe(float64(len(chunks)))

	fetchLog.Info("fetch received",
		logger.Int("records", len(records)),
		logger.Int(logger.FieldChunkCount, len(chunks)),
		logger.Int(logger.FieldPartition, int(partition)),
		logger.Int("first_offset", int(firstOffset)),
		logger.Int("last_offset", int(lastOffset)),
	)

	// Serial chunk processing is load-bearing: chunks are processed in strict
	// fetch order (splitBySize preserves PollFetches ordering), and Kafka
	// offset commits are per-partition max-offset semantics. Committing chunk
	// N implicitly advances past chunks 1..N-1 on the same partition. A
	// future change that reintroduces parallel chunk processing here would
	// make this commit strategy unsafe — a fast later chunk could commit past
	// an unfinished earlier one and silently skip records.
	for i, chunk := range chunks {
		if i > 0 {
			fetchLog.Debug("inter-chunk sleep",
				logger.Dur("sleep", w.llmCallInterval),
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
						logger.Str("stack", string(debug.Stack())),
					)
					if merr := w.ledger.UpdateSeenReasonBatch(fetchCtx, ids, jobcommon.SeenReasonError); merr != nil {
						w.log.Error("mark error batch failed after panic", merr,
							logger.Str(logger.FieldFetchID, fetchID),
							logger.Int(logger.FieldChunkIndex, i),
						)
					}
				}
			}()
			w.processChunk(fetchCtx, chunk, fetchID, i, len(chunks))
		}()

		if fetchCtx.Err() != nil {
			return nil
		}

		recs := chunkRecords(chunk)
		if cerr := w.committer.CommitRecords(fetchCtx, recs...); cerr != nil {
			metrics.FilterCommitFailuresTotal.Inc()
			fetchLog.Error("commit failed", cerr,
				logger.Int(logger.FieldChunkIndex, i),
				logger.Int(logger.FieldPartition, int(partition)),
			)
			// Continue — next fetch will redeliver uncommitted records.
			continue
		}
		if len(recs) > 0 {
			last := recs[len(recs)-1]
			fetchLog.Debug("chunk committed",
				logger.Int(logger.FieldChunkIndex, i),
				logger.Int(logger.FieldChunkCount, len(chunks)),
				logger.Int(logger.FieldOffset, int(last.Offset)),
				logger.Int(logger.FieldPartition, int(last.Partition)),
			)
		}
	}

	if len(malformed) > 0 {
		if cerr := w.committer.CommitRecords(fetchCtx, malformed...); cerr != nil {
			fetchLog.Error("commit malformed records failed", cerr)
		}
	}

	return nil
}

// processChunk owns one chunk end-to-end: ScoreBatch → publish relevant /
// mark irrelevant, or route errors through retry ladder / DLQ and update
// the ledger. It never returns an error signal; commit happens after it
// returns regardless of outcome.
func (w *Worker) processChunk(ctx context.Context, chunk []chunkItem, fetchID string, chunkIndex, chunkCount int) {
	if len(chunk) == 0 {
		return
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
	defer sp.End()

	start := w.clock.Now()

	// Partition: jobs without a title can't be scored — the LLM has no signal to
	// evaluate relevance and will default to "EXCLUDE". Pass them through directly
	// to the enricher which will fetch full content and do role_match there.
	var scorable, passThrough []chunkItem
	for _, item := range chunk {
		if item.job.job.Title == "" {
			passThrough = append(passThrough, item)
		} else {
			scorable = append(scorable, item)
		}
	}

	for _, item := range passThrough {
		w.publishDetail(ctx, item.job.job)
	}
	if len(passThrough) > 0 {
		w.log.Info("chunk pass-through (no title)",
			logger.Str(logger.FieldFetchID, fetchID),
			logger.Int(logger.FieldChunkIndex, chunkIndex),
			logger.Int("pass_through_count", len(passThrough)),
		)
		metrics.FilterJobsTotal.WithLabelValues("relevant").Add(float64(len(passThrough)))
	}

	if len(scorable) == 0 {
		metrics.FilterChunkDuration.WithLabelValues("relevant").Observe(w.clock.Now().Sub(start).Seconds())
		return
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
					logger.Str("retry_after", exhausted.RetryAfter.UTC().Format(time.RFC3339)),
					logger.Dur("retry_in", time.Until(exhausted.RetryAfter)),
				)
			}
			fields = append(fields, logger.Str("slots_detail", formatSlotStatuses(exhausted.Slots)))
		}
		w.log.Error("ScoreBatch failed, routing chunk", err, fields...)

		routeCtx, routeSpan := tracing.Start(ctx, "filter.worker.routeFailed")
		outcome := w.routeFailedChunk(routeCtx, scorable, err)
		routeSpan.SetAttributes(
			attribute.Int("dlqed", len(outcome.DLQed)),
			attribute.Int("retried", len(outcome.Retried)),
		)
		routeSpan.End()

		// DLQ ledger update (ledger reason=error).
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

		// Retry ledger update (ledger reason=retry, per-job retry_count + next_attempt_at).
		if len(outcome.Retried) > 0 {
			if rerr := w.ledger.UpdateRetryBatch(ctx, outcome.Retried); rerr != nil {
				w.log.Error("UpdateRetryBatch failed", rerr,
					logger.Str(logger.FieldFetchID, fetchID),
					logger.Int(logger.FieldChunkIndex, chunkIndex),
					logger.Int("entries_count", len(outcome.Retried)),
				)
			}
		}

		publishFailed := len(scorable) - len(outcome.DLQed) - len(outcome.Retried)
		w.log.Info("chunk routed",
			logger.Str(logger.FieldFetchID, fetchID),
			logger.Int(logger.FieldChunkIndex, chunkIndex),
			logger.Int(logger.FieldChunkCount, chunkCount),
			logger.Int("retried_count", len(outcome.Retried)),
			logger.Int("dlqed_count", len(outcome.DLQed)),
			logger.Int("publish_failed_count", publishFailed),
		)

		metrics.FilterChunkDuration.WithLabelValues("error").Observe(w.clock.Now().Sub(start).Seconds())
		return
	}
	scoreSpan.End()

	relevant := make(map[string]bool, len(relevantIDs))
	for _, id := range relevantIDs {
		relevant[id] = true
	}

	var irrelevantIDs []string
	for _, item := range scorable {
		if relevant[item.job.job.ID] {
			w.publishDetail(ctx, item.job.job)
		} else {
			irrelevantIDs = append(irrelevantIDs, item.job.job.ID)
		}
	}

	if len(irrelevantIDs) > 0 {
		if err := w.ledger.UpdateSeenReasonBatch(ctx, irrelevantIDs, jobcommon.SeenReasonLLMIrrelevant); err != nil {
			w.log.Error("mark irrelevant failed", err,
				logger.Str(logger.FieldFetchID, fetchID),
				logger.Int(logger.FieldChunkIndex, chunkIndex),
			)
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
		logger.Int("irrelevant", len(irrelevantIDs)),
	)
}

// routeFailedChunk ships each job in a failed chunk to the correct retry tier
// (or DLQ if exhausted / validation error). Returns the routing outcome so
// processChunk can update the ledger accordingly. NextAttemptAt is computed
// once per job and used for both the Kafka header and the ledger row.
func (w *Worker) routeFailedChunk(ctx context.Context, chunk []chunkItem, scoreErr error) routeOutcome {
	tiers := kafka.FilterRetryTiers()
	var outcome routeOutcome

	if stderrors.Is(scoreErr, domainerr.ErrLLMValidation) {
		for _, item := range chunk {
			if w.publishToDLQ(ctx, item.job, scoreErr) {
				outcome.DLQed = append(outcome.DLQed, item.job.job.ID)
			}
		}
		return outcome
	}

	for _, item := range chunk {
		if item.job.retryCount >= len(tiers) {
			if w.publishToDLQ(ctx, item.job, scoreErr) {
				outcome.DLQed = append(outcome.DLQed, item.job.job.ID)
			}
			continue
		}
		tier := tiers[item.job.retryCount]
		tierName := tier.Name()
		newCount := item.job.retryCount + 1
		nextAt := w.clock.Now().Add(tier.Delay)

		payload, err := json.Marshal(item.job.job)
		if err != nil {
			w.log.Error("marshal for retry failed", err,
				logger.Str(logger.FieldJobID, item.job.job.ID),
			)
			continue
		}
		headers := kafka.BuildRetryHeaders(newCount, nextAt)
		if perr := w.publisher.PublishWithHeaders(ctx, tier.Topic, payload, headers); perr != nil {
			w.log.Warn("publish to retry tier failed",
				logger.Err(perr),
				logger.Str(logger.FieldJobID, item.job.job.ID),
				logger.Int(logger.FieldRetryCount, newCount),
				logger.Str(logger.FieldRetryTier, tierName),
			)
			metrics.FilterRetryPublishFailures.WithLabelValues(tierName).Inc()
			continue
		}
		outcome.Retried = append(outcome.Retried, jobsvc.RetryLedgerEntry{
			JobID:         item.job.job.ID,
			RetryCount:    newCount,
			NextAttemptAt: nextAt,
		})
		metrics.FilterRetryPublishesTotal.WithLabelValues(tierName).Inc()
	}
	return outcome
}

// publishToDLQ marshals and publishes a single job to the raw DLQ. Returns true on success.
func (w *Worker) publishToDLQ(ctx context.Context, item jobWithCtx, scoreErr error) bool {
	payload, err := json.Marshal(item.job)
	if err != nil {
		w.log.Error("marshal for DLQ failed", err, logger.Str(logger.FieldJobID, item.job.ID))
		return false
	}
	headers := kafka.BuildDLQHeaders(item.retryCount, scoreErr.Error())
	if perr := w.publisher.PublishWithHeaders(ctx, kafka.TopicJobListingsRawDLQ, payload, headers); perr != nil {
		w.log.Warn("publish to DLQ failed",
			logger.Err(perr),
			logger.Str(logger.FieldJobID, item.job.ID),
		)
		return false
	}
	return true
}

func (w *Worker) publishDetail(ctx context.Context, job ats.RawJob) {
	payload, err := json.Marshal(job)
	if err != nil {
		w.log.Error("marshal failed", err, logger.Str(logger.FieldJobID, job.ID))
		return
	}
	if err := w.publisher.Publish(ctx, kafka.TopicJobListingsFiltered, payload); err != nil {
		w.log.Error("publish failed", err, logger.Str(logger.FieldJobID, job.ID))
	}
}

// formatSlotStatuses renders per-slot exhaustion detail for log output:
// "slot_id=reason(fail=N,cd=RFC3339)" joined by "; ".
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

// chunkRecords returns the *kgo.Record slice for a chunk, used to commit
// offsets after the chunk completes processing.
func chunkRecords(chunk []chunkItem) []*kgo.Record {
	out := make([]*kgo.Record, 0, len(chunk))
	for _, it := range chunk {
		if it.record != nil {
			out = append(out, it.record)
		}
	}
	return out
}
