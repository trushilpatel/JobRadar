package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	"github.com/trushilpatel/jobradar/infra/logger"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeScorer struct {
	relevantIDs []string
	err         error
	calls       int
}

func (f *fakeScorer) ScoreBatch(_ context.Context, jobs []llmdto.JobsRelevanceFilterInput) ([]string, error) {
	if len(jobs) == 0 {
		return nil, nil
	}
	f.calls++
	return f.relevantIDs, f.err
}

type ledgerCall struct {
	ids    []string
	reason string
}

type fakeLedger struct {
	calls []ledgerCall
}

func (f *fakeLedger) UpdateSeenReasonBatch(_ context.Context, ids []string, reason string) error {
	f.calls = append(f.calls, ledgerCall{ids: ids, reason: reason})
	return nil
}

type fakePublisher struct {
	payloads [][]byte
}

func (f *fakePublisher) Publish(_ context.Context, _ string, value []byte) error {
	f.payloads = append(f.payloads, value)
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func makeWorker(scorer *fakeScorer, ledger *fakeLedger, pub *fakePublisher) *Worker {
	return &Worker{
		scorer:    scorer,
		ledger:    ledger,
		publisher: pub,
		batchSize: 10,
		log:       logger.NewNoop(),
	}
}

func makeJob(id string) ats.RawJob {
	return ats.RawJob{ID: id, Title: "Software Engineer"}
}

func makeBatch(jobs ...ats.RawJob) []jobWithCtx {
	batch := make([]jobWithCtx, len(jobs))
	for i, j := range jobs {
		batch[i] = jobWithCtx{ctx: context.Background(), job: j}
	}
	return batch
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestFlushBatch_AllRelevant(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"job-1", "job-2", "job-3"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.flushBatch(context.Background(), makeBatch(
		makeJob("job-1"), makeJob("job-2"), makeJob("job-3"),
	))

	if len(pub.payloads) != 3 {
		t.Errorf("expected 3 published jobs, got %d", len(pub.payloads))
	}
	if len(ledger.calls) != 0 {
		t.Errorf("expected no ledger calls, got %d", len(ledger.calls))
	}
}

func TestFlushBatch_PartialRelevant(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"job-1", "job-3"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.flushBatch(context.Background(), makeBatch(
		makeJob("job-1"), makeJob("job-2"), makeJob("job-3"),
	))

	if len(pub.payloads) != 2 {
		t.Errorf("expected 2 published jobs, got %d", len(pub.payloads))
	}

	for _, payload := range pub.payloads {
		var job ats.RawJob
		if err := json.Unmarshal(payload, &job); err != nil {
			t.Fatalf("unmarshal published payload: %v", err)
		}
		if job.ID != "job-1" && job.ID != "job-3" {
			t.Errorf("unexpected published job ID: %s", job.ID)
		}
	}

	if len(ledger.calls) != 1 {
		t.Fatalf("expected 1 ledger call, got %d", len(ledger.calls))
	}
	call := ledger.calls[0]
	if call.reason != jobcommon.SeenReasonLLMIrrelevant {
		t.Errorf("expected reason %q, got %q", jobcommon.SeenReasonLLMIrrelevant, call.reason)
	}
	if len(call.ids) != 1 || call.ids[0] != "job-2" {
		t.Errorf("expected irrelevant IDs [job-2], got %v", call.ids)
	}
}

func TestFlushBatch_ScorerError(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.flushBatch(context.Background(), makeBatch(
		makeJob("job-1"), makeJob("job-2"),
	))

	if len(pub.payloads) != 0 {
		t.Errorf("expected no published jobs on scorer error, got %d", len(pub.payloads))
	}
	if len(ledger.calls) != 1 {
		t.Fatalf("expected 1 ledger call, got %d", len(ledger.calls))
	}
	call := ledger.calls[0]
	if call.reason != jobcommon.SeenReasonError {
		t.Errorf("expected reason %q, got %q", jobcommon.SeenReasonError, call.reason)
	}
	if len(call.ids) != 2 {
		t.Errorf("expected 2 error IDs, got %v", call.ids)
	}
}

func TestFlushBatch_EmptyBatch(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.flushBatch(context.Background(), makeBatch())

	if scorer.calls != 0 {
		t.Errorf("expected scorer not called on empty batch, got %d calls", scorer.calls)
	}
	if len(pub.payloads) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.payloads))
	}
	if len(ledger.calls) != 0 {
		t.Errorf("expected no ledger calls, got %d", len(ledger.calls))
	}
}
