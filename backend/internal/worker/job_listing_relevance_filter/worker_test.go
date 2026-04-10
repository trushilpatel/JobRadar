package job_listing_relevance_filter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher"
	"github.com/trushilpatel/jobradar/internal/llm"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/model"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeScorer struct {
	relevantIDs []string
	err         error
	calls       int
}

func (f *fakeScorer) ScoreBatch(_ context.Context, jobs []llm.ScorerInput) ([]string, error) {
	if len(jobs) == 0 {
		return nil, nil
	}
	f.calls++
	return f.relevantIDs, f.err
}

func (f *fakeScorer) Enrich(_ context.Context, _ llm.EnrichInput) (*llm.EnrichResult, error) {
	return nil, nil
}

type ledgerCall struct {
	ids    []string
	reason string
}

type fakeLedger struct {
	calls []ledgerCall
}

func (f *fakeLedger) MarkSeen(_, _, _ string) error { return nil }
func (f *fakeLedger) UpdateSeenReason(_, _ string) error { return nil }
func (f *fakeLedger) UpdateSeenReasonBatch(ids []string, reason string) error {
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

func makeJob(id string) ats_fetcher.RawJob {
	return ats_fetcher.RawJob{ID: id, Title: "Software Engineer"}
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestFlushBatch_AllRelevant(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"job-1", "job-2", "job-3"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.flushBatch(context.Background(), []ats_fetcher.RawJob{
		makeJob("job-1"), makeJob("job-2"), makeJob("job-3"),
	})

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
	w.flushBatch(context.Background(), []ats_fetcher.RawJob{
		makeJob("job-1"), makeJob("job-2"), makeJob("job-3"),
	})

	if len(pub.payloads) != 2 {
		t.Errorf("expected 2 published jobs, got %d", len(pub.payloads))
	}

	// Verify published IDs are the relevant ones
	for _, payload := range pub.payloads {
		var job ats_fetcher.RawJob
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
	if call.reason != model.SeenReasonLLMIrrelevant {
		t.Errorf("expected reason %q, got %q", model.SeenReasonLLMIrrelevant, call.reason)
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
	w.flushBatch(context.Background(), []ats_fetcher.RawJob{
		makeJob("job-1"), makeJob("job-2"),
	})

	if len(pub.payloads) != 0 {
		t.Errorf("expected no published jobs on scorer error, got %d", len(pub.payloads))
	}
	if len(ledger.calls) != 1 {
		t.Fatalf("expected 1 ledger call, got %d", len(ledger.calls))
	}
	call := ledger.calls[0]
	if call.reason != model.SeenReasonError {
		t.Errorf("expected reason %q, got %q", model.SeenReasonError, call.reason)
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
	w.flushBatch(context.Background(), []ats_fetcher.RawJob{})

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
