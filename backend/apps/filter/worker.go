package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
	llmcore "github.com/trushilpatel/jobradar/service/llm/services"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Worker consumes job_listings.raw via PollBatch, splits each fetch into
// chunks of batchSize, and scores each chunk concurrently via ScoreBatch.
// Up to parallelism chunks run in parallel; the rest wait for a slot.
// Offsets are committed only after all chunks complete.
type Worker struct {
	consumer    *kafka.Consumer
	publisher   kafka.Publisher
	scorer      llmcore.JobsRelevanceFilter
	ledger      FilterLedger
	batchSize   int
	parallelism int
	log         logger.Logger
}

func NewWorker(
	consumer *kafka.Consumer,
	publisher kafka.Publisher,
	scorer llmcore.JobsRelevanceFilter,
	ledger FilterLedger,
	batchSize int,
	parallelism int,
	log logger.Logger,
) *Worker {
	return &Worker{
		consumer:    consumer,
		publisher:   publisher,
		scorer:      scorer,
		ledger:      ledger,
		batchSize:   batchSize,
		parallelism: parallelism,
		log:         log,
	}
}

// jobWithCtx pairs a raw job with the trace context extracted from its Kafka
// headers and the retry_count value carried on its incoming Kafka record.
type jobWithCtx struct {
	ctx        context.Context
	job        ats.RawJob
	retryCount int
}

// Run starts consuming job_listings.raw and blocks until ctx is cancelled.
// Each Kafka fetch is split into chunks of batchSize. All chunks are scored
// concurrently (up to parallelism at a time) before offsets are committed.
func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker started",
		logger.Int("batch_size", w.batchSize),
		logger.Int("llm_parallelism", w.parallelism),
	)

	return w.consumer.PollBatch(ctx, func(records []*kgo.Record) error {
		return w.processFetch(ctx, records)
	})
}

// processFetch parses all records from one Kafka fetch, splits them into
// chunks of batchSize, and scores each chunk in a separate goroutine.
// A semaphore caps concurrent LLM calls at w.parallelism.
func (w *Worker) processFetch(ctx context.Context, records []*kgo.Record) error {
	var jobs []jobWithCtx
	for _, r := range records {
		msgCtx := tracing.ExtractKafka(ctx, r.Headers)
		var job ats.RawJob
		if err := json.Unmarshal(r.Value, &job); err != nil {
			w.log.Warn("malformed message, skipping", logger.Err(err))
			continue
		}
		jobs = append(jobs, jobWithCtx{
			ctx:        msgCtx,
			job:        job,
			retryCount: kafka.RetryCountFromHeaders(r.Headers),
		})
	}

	if len(jobs) == 0 {
		return nil
	}

	fetchCtx, span := tracing.Start(ctx, "filter.worker.processFetch")
	defer tracing.End(span, nil)

	chunks := splitBySize(jobs, w.batchSize)
	sem := make(chan struct{}, w.parallelism)
	var wg sync.WaitGroup
	start := time.Now()

	for _, chunk := range chunks {
		wg.Add(1)
		sem <- struct{}{}
		go func(sub []jobWithCtx) {
			defer wg.Done()
			defer func() { <-sem }()
			w.processChunk(fetchCtx, sub)
		}(chunk)
	}

	wg.Wait()
	metrics.FilterLLMDuration.Observe(time.Since(start).Seconds())

	w.log.Info("fetch processed",
		logger.Int("jobs", len(jobs)),
		logger.Int("chunks", len(chunks)),
	)

	return nil
}

// processChunk owns one chunk end-to-end: ScoreBatch → publish relevant →
// mark irrelevant in ledger → route errors to retry/DLQ.
func (w *Worker) processChunk(ctx context.Context, chunk []jobWithCtx) {
	if len(chunk) == 0 {
		return
	}
	inputs := make([]llmdto.JobsRelevanceFilterInput, len(chunk))
	for i, item := range chunk {
		inputs[i] = llmdto.JobsRelevanceFilterInput{
			ID:       item.job.ID,
			Title:    item.job.Title,
			Location: item.job.Location,
		}
	}

	relevantIDs, err := w.scorer.ScoreBatch(ctx, inputs)
	if err != nil {
		fields := []logger.Field{logger.Int(logger.FieldBatch, len(chunk))}
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
		dlqIDs := w.routeFailedChunk(ctx, chunk, err)
		if len(dlqIDs) > 0 {
			if merr := w.ledger.UpdateSeenReasonBatch(ctx, dlqIDs, jobcommon.SeenReasonError); merr != nil {
				w.log.Error("mark error batch failed", merr)
			}
			metrics.FilterJobsTotal.WithLabelValues("error").Add(float64(len(dlqIDs)))
		}
		return
	}

	relevant := make(map[string]bool, len(relevantIDs))
	for _, id := range relevantIDs {
		relevant[id] = true
	}

	var irrelevantIDs []string
	for _, item := range chunk {
		if relevant[item.job.ID] {
			w.publishDetail(ctx, item.job)
		} else {
			irrelevantIDs = append(irrelevantIDs, item.job.ID)
		}
	}

	if len(irrelevantIDs) > 0 {
		if err := w.ledger.UpdateSeenReasonBatch(ctx, irrelevantIDs, jobcommon.SeenReasonLLMIrrelevant); err != nil {
			w.log.Error("mark irrelevant failed", err)
		}
	}

	metrics.FilterJobsTotal.WithLabelValues("relevant").Add(float64(len(relevantIDs)))
	metrics.FilterJobsTotal.WithLabelValues("irrelevant").Add(float64(len(irrelevantIDs)))

	w.log.Info("chunk scored",
		logger.Int(logger.FieldBatch, len(chunk)),
		logger.Int(logger.FieldRelevant, len(relevantIDs)),
		logger.Int("irrelevant", len(irrelevantIDs)),
	)
}

// routeFailedChunk ships each job in a failed chunk to the correct retry tier
// (or DLQ if exhausted / validation error). Returns DLQ job IDs.
func (w *Worker) routeFailedChunk(ctx context.Context, chunk []jobWithCtx, scoreErr error) []string {
	tiers := kafka.FilterRetryTiers()
	now := time.Now()

	if stderrors.Is(scoreErr, domainerr.ErrLLMValidation) {
		var dlqIDs []string
		for _, item := range chunk {
			if w.publishToDLQ(ctx, item, scoreErr) {
				dlqIDs = append(dlqIDs, item.job.ID)
			}
		}
		return dlqIDs
	}

	var dlqIDs []string
	for _, item := range chunk {
		if item.retryCount >= len(tiers) {
			if w.publishToDLQ(ctx, item, scoreErr) {
				dlqIDs = append(dlqIDs, item.job.ID)
			}
			continue
		}
		tier := tiers[item.retryCount]
		newCount := item.retryCount + 1
		nextAt := now.Add(tier.Delay)
		payload, err := json.Marshal(item.job)
		if err != nil {
			w.log.Error("marshal for retry failed", err, logger.Str(logger.FieldJobID, item.job.ID))
			continue
		}
		headers := kafka.BuildRetryHeaders(newCount, nextAt)
		if perr := w.publisher.PublishWithHeaders(ctx, tier.Topic, payload, headers); perr != nil {
			w.log.Error("publish to retry tier failed", perr,
				logger.Str(logger.FieldJobID, item.job.ID),
				logger.Int("retry_count", newCount),
			)
		}
	}
	return dlqIDs
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
		w.log.Error("publish to DLQ failed", perr, logger.Str(logger.FieldJobID, item.job.ID))
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
func splitBySize(s []jobWithCtx, size int) [][]jobWithCtx {
	if size < 1 {
		size = 1
	}
	chunks := make([][]jobWithCtx, 0, (len(s)+size-1)/size)
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[i:end])
	}
	return chunks
}
