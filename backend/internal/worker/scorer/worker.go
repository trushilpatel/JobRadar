package scorer

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/trushilpatel/jobradar/internal/adapter"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/llm"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
)

// Worker consumes jobs.raw, scores them in batches via LLM,
// routes relevant jobs to jobs.raw_detail, and marks irrelevant ones in seen_ledger.
type Worker struct {
	consumer     *kafka.Consumer
	publisher    kafka.Publisher
	scorer       llm.Scorer
	ledger       store.SeenLedgerWriter
	batchSize    int
	flushTimeout time.Duration
}

func NewWorker(
	consumer *kafka.Consumer,
	publisher kafka.Publisher,
	scorer llm.Scorer,
	ledger store.SeenLedgerWriter,
	batchSize int,
	flushTimeoutSeconds int,
) *Worker {
	return &Worker{
		consumer:     consumer,
		publisher:    publisher,
		scorer:       scorer,
		ledger:       ledger,
		batchSize:    batchSize,
		flushTimeout: time.Duration(flushTimeoutSeconds) * time.Second,
	}
}

// Run starts consuming jobs.raw and blocks until ctx is cancelled.
// Jobs are scored in batches of batchSize. If the batch doesn't fill within
// flushTimeout, whatever is buffered gets flushed — so jobs never sit forever.
func (w *Worker) Run(ctx context.Context) error {
	log.Printf("[scorer] worker started (batchSize=%d flushTimeout=%s)", w.batchSize, w.flushTimeout)

	var batch []adapter.RawJob
	timer := time.NewTimer(w.flushTimeout)
	defer timer.Stop()

	// jobsCh bridges the Kafka poll goroutine to the batch loop below.
	jobsCh := make(chan adapter.RawJob, w.batchSize*2)
	errCh := make(chan error, 1)

	go func() {
		err := w.consumer.Poll(ctx, func(_ string, value []byte) error {
			var job adapter.RawJob
			if err := json.Unmarshal(value, &job); err != nil {
				log.Printf("[scorer] malformed message: %v — skipping", err)
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
				log.Printf("[scorer] flush timeout — flushing %d buffered jobs", len(batch))
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
				log.Printf("[scorer] shutdown — flushing %d remaining jobs", len(batch))
				w.flushBatch(ctx, batch)
			}
			return nil
		}
	}
}

// flushBatch scores a batch of jobs and routes them.
func (w *Worker) flushBatch(ctx context.Context, batch []adapter.RawJob) {
	inputs := make([]llm.ScorerInput, len(batch))
	for i, job := range batch {
		inputs[i] = llm.ScorerInput{ID: job.ID, Title: job.Title}
	}

	relevantIDs, err := w.scorer.ScoreBatch(ctx, inputs)
	if err != nil {
		log.Printf("[scorer] ScoreBatch failed: %v — re-queuing batch of %d", err, len(batch))
		for _, job := range batch {
			payload, merr := json.Marshal(job)
			if merr != nil {
				log.Printf("[scorer] re-queue marshal failed for %s: %v", job.ID, merr)
				continue
			}
			if perr := w.publisher.Publish(ctx, kafka.TopicJobsRaw, payload); perr != nil {
				log.Printf("[scorer] re-queue publish failed for %s: %v", job.ID, perr)
			}
		}
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
			log.Printf("[scorer] batch mark irrelevant failed: %v", err)
		}
	}

	log.Printf("[scorer] batch=%d relevant=%d irrelevant=%d", len(batch), len(relevantIDs), len(batch)-len(relevantIDs))
}

func (w *Worker) publishDetail(ctx context.Context, job adapter.RawJob) {
	payload, err := json.Marshal(job)
	if err != nil {
		log.Printf("[scorer] marshal failed for %s: %v", job.ID, err)
		return
	}
	if err := w.publisher.Publish(ctx, kafka.TopicJobsRawDetail, payload); err != nil {
		log.Printf("[scorer] publish failed for %s: %v", job.ID, err)
	}
}
