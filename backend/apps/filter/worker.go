package main

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/infra/tracing"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
	llmcore "github.com/trushilpatel/jobradar/service/llm/services"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Worker consumes jobs.raw, scores them in batches via LLM,
// routes relevant jobs to jobs.raw_detail, and marks irrelevant ones in seen_ledger.
type Worker struct {
	consumer     *kafka.Consumer
	publisher    kafka.Publisher
	scorer       llmcore.JobsRelevanceFilter
	ledger       FilterLedger
	batchSize    int
	flushTimeout time.Duration
	parallelism  int
	log          logger.Logger
}

func NewWorker(
	consumer *kafka.Consumer,
	publisher kafka.Publisher,
	scorer llmcore.JobsRelevanceFilter,
	ledger FilterLedger,
	batchSize int,
	flushTimeoutSeconds int,
	parallelism int,
	log logger.Logger,
) *Worker {
	return &Worker{
		consumer:     consumer,
		publisher:    publisher,
		scorer:       scorer,
		ledger:       ledger,
		batchSize:    batchSize,
		flushTimeout: time.Duration(flushTimeoutSeconds) * time.Second,
		parallelism:  parallelism,
		log:          log,
	}
}

// jobWithCtx pairs a raw job with the trace context extracted from its Kafka headers.
type jobWithCtx struct {
	ctx context.Context
	job ats.RawJob
}

// Run starts consuming jobs.raw and blocks until ctx is cancelled.
// Jobs are scored in batches of batchSize. If the batch doesn't fill within
// flushTimeout, whatever is buffered gets flushed — so jobs never sit forever.
func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker started",
		logger.Int("batch_size", w.batchSize),
		logger.Dur("flush_timeout", w.flushTimeout),
		logger.Int("llm_parallelism", w.parallelism),
	)

	var batch []jobWithCtx
	timer := time.NewTimer(w.flushTimeout)
	defer timer.Stop()

	// jobsCh bridges the Kafka poll goroutine to the batch loop below.
	jobsCh := make(chan jobWithCtx, w.batchSize*2)
	errCh := make(chan error, 1)

	go func() {
		err := w.consumer.Poll(ctx, func(_ string, value []byte, headers []kgo.RecordHeader) error {
			msgCtx := tracing.ExtractKafka(ctx, headers) // continue trace from crawler
			var job ats.RawJob
			if err := json.Unmarshal(value, &job); err != nil {
				w.log.Warn("malformed message, skipping", logger.Err(err))
				return nil
			}
			select {
			case jobsCh <- jobWithCtx{ctx: msgCtx, job: job}:
			case <-ctx.Done():
			}
			return nil
		})
		errCh <- err
	}()

	for {
		select {
		case item := <-jobsCh:
			batch = append(batch, item)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.flushTimeout)

			if len(batch) >= w.batchSize {
				w.flushBatch(ctx, batch)
				batch = batch[:0]
				timer.Reset(w.flushTimeout)
			}

		case <-timer.C:
			if len(batch) > 0 {
				w.log.Debug("flush timeout, flushing batch", logger.Int(logger.FieldBatch, len(batch)))
				w.flushBatch(ctx, batch)
				batch = batch[:0]
			}
			timer.Reset(w.flushTimeout)

		case err := <-errCh:
			// Flush any remaining jobs before exiting
			if len(batch) > 0 {
				w.flushBatch(ctx, batch)
			}
			return err

		case <-ctx.Done():
			// Flush remaining jobs on graceful shutdown
			if len(batch) > 0 {
				w.log.Info("shutdown, flushing remaining jobs", logger.Int(logger.FieldBatch, len(batch)))
				w.flushBatch(ctx, batch)
			}
			return nil
		}
	}
}

// subBatchResult holds the outcome of one parallel ScoreBatch call.
type subBatchResult struct {
	relevantIDs []string
	errorIDs    []string
}

// flushBatch splits the batch into w.parallelism sub-batches, scores them in
// parallel via ScoreBatch, then routes relevant jobs and marks irrelevant ones.
func (w *Worker) flushBatch(ctx context.Context, batch []jobWithCtx) {
	if len(batch) == 0 {
		return
	}

	// Use the first job's trace context as the parent for the batch span.
	batchCtx, span := tracing.Start(batch[0].ctx, "filter.worker.flushBatch")
	defer tracing.End(span, nil)

	// Only use as many parallel calls as needed to keep each sub-batch at the
	// target size. Avoids firing w.parallelism LLM calls for a tiny partial batch.
	maxParallelism := w.parallelism
	if maxParallelism < 1 {
		maxParallelism = 1
	}
	targetSubBatchSize := w.batchSize / maxParallelism
	if targetSubBatchSize < 1 {
		targetSubBatchSize = 1
	}
	parallelism := len(batch) / targetSubBatchSize
	if parallelism < 1 {
		parallelism = 1
	} else if parallelism > maxParallelism {
		parallelism = maxParallelism
	}

	subBatches := splitIntoN(batch, parallelism)
	resultCh := make(chan subBatchResult, len(subBatches))

	var wg sync.WaitGroup
	start := time.Now()

	for _, sub := range subBatches {
		sub := sub
		wg.Add(1)
		go func() {
			defer wg.Done()
			inputs := make([]llmdto.JobsRelevanceFilterInput, len(sub))
			ids := make([]string, len(sub))
			for i, item := range sub {
				inputs[i] = llmdto.JobsRelevanceFilterInput{ID: item.job.ID, Title: item.job.Title, Location: item.job.Location}
				ids[i] = item.job.ID
			}
			relevantIDs, err := w.scorer.ScoreBatch(batchCtx, inputs)
			if err != nil {
				w.log.Error("ScoreBatch failed, marking sub-batch as error", err, logger.Int(logger.FieldBatch, len(sub)))
				resultCh <- subBatchResult{errorIDs: ids}
				return
			}
			resultCh <- subBatchResult{relevantIDs: relevantIDs}
		}()
	}

	wg.Wait()
	close(resultCh)
	metrics.FilterLLMDuration.Observe(time.Since(start).Seconds())

	// Collect results from all sub-batches
	var allRelevantIDs []string
	var allErrorIDs []string
	for r := range resultCh {
		allRelevantIDs = append(allRelevantIDs, r.relevantIDs...)
		allErrorIDs = append(allErrorIDs, r.errorIDs...)
	}

	if len(allErrorIDs) > 0 {
		if merr := w.ledger.UpdateSeenReasonBatch(batchCtx, allErrorIDs, jobcommon.SeenReasonError); merr != nil {
			w.log.Error("mark error batch failed", merr)
		}
		metrics.FilterJobsTotal.WithLabelValues("error").Add(float64(len(allErrorIDs)))
	}

	relevant := make(map[string]bool, len(allRelevantIDs))
	for _, id := range allRelevantIDs {
		relevant[id] = true
	}
	errorSet := make(map[string]bool, len(allErrorIDs))
	for _, id := range allErrorIDs {
		errorSet[id] = true
	}

	var irrelevantIDs []string
	for _, item := range batch {
		if errorSet[item.job.ID] {
			continue
		}
		if relevant[item.job.ID] {
			w.publishDetail(batchCtx, item.job)
		} else {
			irrelevantIDs = append(irrelevantIDs, item.job.ID)
		}
	}

	if len(irrelevantIDs) > 0 {
		if err := w.ledger.UpdateSeenReasonBatch(batchCtx, irrelevantIDs, jobcommon.SeenReasonLLMIrrelevant); err != nil {
			w.log.Error("mark irrelevant failed", err)
		}
	}

	metrics.FilterJobsTotal.WithLabelValues("relevant").Add(float64(len(allRelevantIDs)))
	metrics.FilterJobsTotal.WithLabelValues("irrelevant").Add(float64(len(irrelevantIDs)))

	w.log.Info("batch scored",
		logger.Int(logger.FieldBatch, len(batch)),
		logger.Int(logger.FieldRelevant, len(allRelevantIDs)),
		logger.Int("irrelevant", len(irrelevantIDs)),
		logger.Int("errors", len(allErrorIDs)),
		logger.Int("sub_batches", len(subBatches)),
	)
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

// splitIntoN divides s into at most n roughly equal sub-slices.
func splitIntoN(s []jobWithCtx, n int) [][]jobWithCtx {
	if n <= 0 || n > len(s) {
		n = len(s)
	}
	size := (len(s) + n - 1) / n // ceil division
	chunks := make([][]jobWithCtx, 0, n)
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[i:end])
	}
	return chunks
}
