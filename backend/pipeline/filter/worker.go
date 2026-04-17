package filter

import (
	"context"
	"encoding/json"
	"time"

	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/pipeline/ats"
	"github.com/trushilpatel/jobradar/pipeline/llm"
)

// Worker consumes jobs.raw, scores them in batches via LLM,
// routes relevant jobs to jobs.raw_detail, and marks irrelevant ones in seen_ledger.
type Worker struct {
	consumer     *kafka.Consumer
	publisher    kafka.Publisher
	scorer       llm.Scorer
	ledger       FilterLedger
	batchSize    int
	flushTimeout time.Duration
	log          logger.Logger
}

func NewWorker(
	consumer *kafka.Consumer,
	publisher kafka.Publisher,
	scorer llm.Scorer,
	ledger FilterLedger,
	batchSize int,
	flushTimeoutSeconds int,
	log logger.Logger,
) *Worker {
	return &Worker{
		consumer:     consumer,
		publisher:    publisher,
		scorer:       scorer,
		ledger:       ledger,
		batchSize:    batchSize,
		flushTimeout: time.Duration(flushTimeoutSeconds) * time.Second,
		log:          log,
	}
}

// Run starts consuming jobs.raw and blocks until ctx is cancelled.
// Jobs are scored in batches of batchSize. If the batch doesn't fill within
// flushTimeout, whatever is buffered gets flushed — so jobs never sit forever.
func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker started",
		logger.Int("batch_size", w.batchSize),
		logger.Dur("flush_timeout", w.flushTimeout),
	)

	var batch []ats.RawJob
	timer := time.NewTimer(w.flushTimeout)
	defer timer.Stop()

	// jobsCh bridges the Kafka poll goroutine to the batch loop below.
	jobsCh := make(chan ats.RawJob, w.batchSize*2)
	errCh := make(chan error, 1)

	go func() {
		err := w.consumer.Poll(ctx, func(_ string, value []byte) error {
			var job ats.RawJob
			if err := json.Unmarshal(value, &job); err != nil {
				w.log.Warn("malformed message, skipping", logger.Err(err))
				return nil
			}
			select {
			case jobsCh <- job:
			case <-ctx.Done():
			}
			return nil
		})
		errCh <- err
	}()

	for {
		select {
		case job := <-jobsCh:
			batch = append(batch, job)
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

// flushBatch scores a batch of jobs and routes them.
func (w *Worker) flushBatch(ctx context.Context, batch []ats.RawJob) {
	inputs := make([]llm.ScorerInput, len(batch))
	for i, job := range batch {
		inputs[i] = llm.ScorerInput{ID: job.ID, Title: job.Title, Location: job.Location}
	}

	start := time.Now()
	relevantIDs, err := w.scorer.ScoreBatch(ctx, inputs)
	metrics.FilterLLMDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		w.log.Error("ScoreBatch failed, marking batch as error", err, logger.Int(logger.FieldBatch, len(batch)))
		ids := make([]string, len(batch))
		for i, job := range batch {
			ids[i] = job.ID
		}
		if merr := w.ledger.UpdateSeenReasonBatch(ids, model.SeenReasonError); merr != nil {
			w.log.Error("mark error batch failed", merr)
		}
		metrics.FilterJobsTotal.WithLabelValues("error").Add(float64(len(batch)))
		return
	}

	relevant := make(map[string]bool, len(relevantIDs))
	for _, id := range relevantIDs {
		relevant[id] = true
	}

	var irrelevantIDs []string
	for _, job := range batch {
		if relevant[job.ID] {
			w.publishDetail(ctx, job)
		} else {
			irrelevantIDs = append(irrelevantIDs, job.ID)
		}
	}

	if len(irrelevantIDs) > 0 {
		if err := w.ledger.UpdateSeenReasonBatch(irrelevantIDs, model.SeenReasonLLMIrrelevant); err != nil {
			w.log.Error("mark irrelevant failed", err)
		}
	}

	metrics.FilterJobsTotal.WithLabelValues("relevant").Add(float64(len(relevantIDs)))
	metrics.FilterJobsTotal.WithLabelValues("irrelevant").Add(float64(len(irrelevantIDs)))

	w.log.Info("batch scored",
		logger.Int(logger.FieldBatch, len(batch)),
		logger.Int(logger.FieldRelevant, len(relevantIDs)),
		logger.Int("irrelevant", len(batch)-len(relevantIDs)),
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
