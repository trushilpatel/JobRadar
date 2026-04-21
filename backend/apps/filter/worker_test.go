package main

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
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
	f.calls = append(f.calls, ledgerCall{ids: append([]string(nil), ids...), reason: reason})
	return nil
}

// publishedRecord captures one publish call.
type publishedRecord struct {
	topic   string
	value   []byte
	headers []kgo.RecordHeader
}

type fakePublisher struct {
	payloads [][]byte
	records  []publishedRecord
}

func (f *fakePublisher) Publish(_ context.Context, topic string, value []byte) error {
	f.payloads = append(f.payloads, value)
	f.records = append(f.records, publishedRecord{topic: topic, value: value})
	return nil
}

func (f *fakePublisher) PublishWithHeaders(_ context.Context, topic string, value []byte, headers []kgo.RecordHeader) error {
	f.payloads = append(f.payloads, value)
	dup := make([]kgo.RecordHeader, len(headers))
	copy(dup, headers)
	f.records = append(f.records, publishedRecord{topic: topic, value: value, headers: dup})
	return nil
}

func (f *fakePublisher) recordsOnTopic(topic string) []publishedRecord {
	var out []publishedRecord
	for _, r := range f.records {
		if r.topic == topic {
			out = append(out, r)
		}
	}
	return out
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

func makeBatchWithRetryCount(retryCount int, jobs ...ats.RawJob) []jobWithCtx {
	batch := make([]jobWithCtx, len(jobs))
	for i, j := range jobs {
		batch[i] = jobWithCtx{ctx: context.Background(), job: j, retryCount: retryCount}
	}
	return batch
}

func headerValue(h []kgo.RecordHeader, key string) (string, bool) {
	for _, rh := range h {
		if rh.Key == key {
			return string(rh.Value), true
		}
	}
	return "", false
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

// On a generic (transient) scorer error with incoming retry_count=0, jobs go to
// the 1m retry tier with retry_count=1 and next_attempt_at ≈ now+1m. No ledger
// call is made — the ledger row stays as-is.
func TestFlushBatch_ScorerError(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	start := time.Now()
	w.flushBatch(context.Background(), makeBatch(
		makeJob("job-1"), makeJob("job-2"),
	))

	retryRecords := pub.recordsOnTopic(kafka.TopicJobListingsRawRetry1m)
	if len(retryRecords) != 2 {
		t.Fatalf("expected 2 records on retry.1m, got %d (all records: %+v)", len(retryRecords), pub.records)
	}
	if len(pub.recordsOnTopic(kafka.TopicJobListingsRawDLQ)) != 0 {
		t.Errorf("expected 0 records on DLQ, got %d", len(pub.recordsOnTopic(kafka.TopicJobListingsRawDLQ)))
	}
	if len(pub.recordsOnTopic(kafka.TopicJobListingsFiltered)) != 0 {
		t.Errorf("expected 0 records on filtered, got %d", len(pub.recordsOnTopic(kafka.TopicJobListingsFiltered)))
	}

	for _, rec := range retryRecords {
		rc, ok := headerValue(rec.headers, kafka.HeaderRetryCount)
		if !ok || rc != "1" {
			t.Errorf("expected retry_count header '1', got %q (present=%v)", rc, ok)
		}
		naStr, ok := headerValue(rec.headers, kafka.HeaderNextAttemptAt)
		if !ok {
			t.Fatalf("missing next_attempt_at header")
		}
		naMs, err := strconv.ParseInt(naStr, 10, 64)
		if err != nil {
			t.Fatalf("unparseable next_attempt_at: %v", err)
		}
		na := time.UnixMilli(naMs)
		want := start.Add(1 * time.Minute)
		if na.Before(want.Add(-5*time.Second)) || na.After(want.Add(30*time.Second)) {
			t.Errorf("next_attempt_at %v outside expected window around %v", na, want)
		}
	}

	if len(ledger.calls) != 0 {
		t.Errorf("expected no ledger calls on retry (ledger row unchanged), got %d: %+v", len(ledger.calls), ledger.calls)
	}
}

// With incoming retry_count=1, failure should publish to retry.10m with retry_count=2.
func TestFlushBatch_ScorerError_RetryCountAdvances(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.flushBatch(context.Background(), makeBatchWithRetryCount(1,
		makeJob("job-1"), makeJob("job-2"),
	))

	retry10m := pub.recordsOnTopic(kafka.TopicJobListingsRawRetry10m)
	if len(retry10m) != 2 {
		t.Fatalf("expected 2 records on retry.10m, got %d", len(retry10m))
	}
	if len(pub.recordsOnTopic(kafka.TopicJobListingsRawRetry1m)) != 0 {
		t.Errorf("expected 0 records on retry.1m")
	}
	for _, rec := range retry10m {
		rc, _ := headerValue(rec.headers, kafka.HeaderRetryCount)
		if rc != "2" {
			t.Errorf("expected retry_count '2', got %q", rc)
		}
	}
	if len(ledger.calls) != 0 {
		t.Errorf("expected no ledger calls on retry, got %d", len(ledger.calls))
	}
}

// With incoming retry_count=3 (tiers exhausted), failure should publish to DLQ
// and mark ledger rows as error.
func TestFlushBatch_ScorerError_ExhaustedGoesToDLQ(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.flushBatch(context.Background(), makeBatchWithRetryCount(3,
		makeJob("job-1"), makeJob("job-2"),
	))

	dlq := pub.recordsOnTopic(kafka.TopicJobListingsRawDLQ)
	if len(dlq) != 2 {
		t.Fatalf("expected 2 records on DLQ, got %d", len(dlq))
	}
	for _, rec := range dlq {
		rc, _ := headerValue(rec.headers, kafka.HeaderRetryCount)
		if rc != "3" {
			t.Errorf("expected retry_count '3' on DLQ, got %q", rc)
		}
		if le, ok := headerValue(rec.headers, kafka.HeaderLastError); !ok || le == "" {
			t.Errorf("expected non-empty last_error header, got %q (present=%v)", le, ok)
		}
	}

	if len(ledger.calls) != 1 {
		t.Fatalf("expected 1 ledger call marking error, got %d", len(ledger.calls))
	}
	call := ledger.calls[0]
	if call.reason != jobcommon.SeenReasonError {
		t.Errorf("expected reason %q, got %q", jobcommon.SeenReasonError, call.reason)
	}
	if len(call.ids) != 2 {
		t.Errorf("expected 2 error IDs, got %v", call.ids)
	}
}

// ErrLLMValidation (our bug) should skip retries entirely: all jobs go to DLQ
// regardless of incoming retry_count, and ledger rows get marked error.
func TestFlushBatch_ValidationErrorGoesStraightToDLQ(t *testing.T) {
	scorer := &fakeScorer{err: domainerr.ErrLLMValidation}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	// Use retry_count=0 to prove we bypass the retry ladder entirely.
	w.flushBatch(context.Background(), makeBatchWithRetryCount(0,
		makeJob("job-1"), makeJob("job-2"),
	))

	if len(pub.recordsOnTopic(kafka.TopicJobListingsRawRetry1m)) != 0 {
		t.Errorf("expected 0 records on retry.1m, got %d", len(pub.recordsOnTopic(kafka.TopicJobListingsRawRetry1m)))
	}
	dlq := pub.recordsOnTopic(kafka.TopicJobListingsRawDLQ)
	if len(dlq) != 2 {
		t.Fatalf("expected 2 records on DLQ, got %d", len(dlq))
	}
	for _, rec := range dlq {
		if le, ok := headerValue(rec.headers, kafka.HeaderLastError); !ok || le == "" {
			t.Errorf("expected non-empty last_error header, got %q (present=%v)", le, ok)
		}
	}

	if len(ledger.calls) != 1 {
		t.Fatalf("expected 1 ledger call marking error, got %d", len(ledger.calls))
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
