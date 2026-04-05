package scorer

import (
	"context"
	"encoding/json"
	"log"

	"github.com/trushilpatel/jobradar/internal/adapter"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/llm"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
)

const batchSize = 50

// Worker consumes jobs.raw, scores them in batches via LLM,
// routes relevant jobs to jobs.raw_detail, and marks irrelevant ones in seen_ledger.
type Worker struct {
	consumer  *kafka.Consumer
	publisher kafka.Publisher
	scorer    llm.Scorer
	ledger    store.SeenLedgerWriter
}

func NewWorker(
	consumer *kafka.Consumer,
	publisher kafka.Publisher,
	scorer llm.Scorer,
	ledger store.SeenLedgerWriter,
) *Worker {
	return &Worker{
		consumer:  consumer,
		publisher: publisher,
		scorer:    scorer,
		ledger:    ledger,
	}
}

// Run starts consuming jobs.raw and blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	log.Println("[scorer] worker started")

	var batch []adapter.RawJob

	return w.consumer.Poll(ctx, func(topic string, value []byte) error {
		var job adapter.RawJob
		if err := json.Unmarshal(value, &job); err != nil {
			log.Printf("[scorer] failed to unmarshal job: %v", err)
			return nil // skip malformed messages, don't stop the consumer
		}

		batch = append(batch, job)

		if len(batch) >= batchSize {
			w.flushBatch(ctx, batch)
			batch = batch[:0] // reset without reallocating
		}

		return nil
	})
}

// flushBatch scores a batch of jobs and routes them.
func (w *Worker) flushBatch(ctx context.Context, batch []adapter.RawJob) {
	// Build scorer inputs — title + id only, no full text
	inputs := make([]llm.ScorerInput, len(batch))
	for i, job := range batch {
		inputs[i] = llm.ScorerInput{ID: job.ID, Title: job.Title}
	}

	relevantIDs, err := w.scorer.ScoreBatch(ctx, inputs)
	if err != nil {
		log.Printf("[scorer] ScoreBatch failed: %v — skipping batch", err)
		return
	}

	// Build a set for O(1) lookup
	relevant := make(map[string]bool, len(relevantIDs))
	for _, id := range relevantIDs {
		relevant[id] = true
	}

	for _, job := range batch {
		if relevant[job.ID] {
			w.publishDetail(ctx, job)
		} else {
			w.markIrrelevant(ctx, job.ID)
		}
	}

	log.Printf("[scorer] batch=%d relevant=%d irrelevant=%d", len(batch), len(relevantIDs), len(batch)-len(relevantIDs))
}

// publishDetail sends a relevant job ID to jobs.raw_detail.
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

// markIrrelevant updates the seen_ledger so this job is never re-processed.
func (w *Worker) markIrrelevant(ctx context.Context, jobID string) {
	if err := w.ledger.UpdateSeenReason(jobID, model.SeenReasonLLMIrrelevant); err != nil {
		log.Printf("[scorer] failed to mark irrelevant for %s: %v", jobID, err)
	}
}
