package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
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
	calls        []ledgerCall
	retryBatches [][]jobsvc.RetryLedgerEntry
	retryErr     error
}

func (f *fakeLedger) UpdateSeenReasonBatch(_ context.Context, ids []string, reason string) error {
	f.calls = append(f.calls, ledgerCall{ids: append([]string(nil), ids...), reason: reason})
	return nil
}

func (f *fakeLedger) UpdateRetryBatch(_ context.Context, entries []jobsvc.RetryLedgerEntry) error {
	if f.retryErr != nil {
		return f.retryErr
	}
	dup := append([]jobsvc.RetryLedgerEntry(nil), entries...)
	f.retryBatches = append(f.retryBatches, dup)
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

type committedBatch struct {
	records []*kgo.Record
}

type fakeCommitter struct {
	mu       sync.Mutex
	batches  []committedBatch
	errQueue []error
}

func (f *fakeCommitter) CommitRecords(_ context.Context, records ...*kgo.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	dup := make([]*kgo.Record, len(records))
	copy(dup, records)
	f.batches = append(f.batches, committedBatch{records: dup})
	if len(f.errQueue) > 0 {
		e := f.errQueue[0]
		f.errQueue = f.errQueue[1:]
		return e
	}
	return nil
}

type fakeClock struct {
	now      time.Time
	sleeps   []time.Duration
	sleepErr error
}

func (f *fakeClock) Now() time.Time { return f.now }

func (f *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.sleepErr != nil {
		return f.sleepErr
	}
	f.sleeps = append(f.sleeps, d)
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func makeWorker(scorer *fakeScorer, ledger *fakeLedger, pub *fakePublisher) *Worker {
	return &Worker{
		scorer:          scorer,
		ledger:          ledger,
		publisher:       pub,
		batchSize:       5,
		llmCallInterval: 5 * time.Second,
		clock:           realClock{},
		log:             logger.NewNoop(),
	}
}

// makeWorkerFull wires a worker with a fake committer + fake clock for
// processFetch-level tests.
func makeWorkerFull(scorer *fakeScorer, ledger *fakeLedger, pub *fakePublisher, committer recordCommitter, clk clock, batchSize int) *Worker {
	return &Worker{
		scorer:          scorer,
		ledger:          ledger,
		publisher:       pub,
		committer:       committer,
		batchSize:       batchSize,
		llmCallInterval: 5 * time.Second,
		clock:           clk,
		log:             logger.NewNoop(),
	}
}

func makeJob(id string) ats.RawJob {
	return ats.RawJob{ID: id, Title: "Software Engineer"}
}

func makeBatch(jobs ...ats.RawJob) []chunkItem {
	batch := make([]chunkItem, len(jobs))
	for i, j := range jobs {
		batch[i] = chunkItem{
			job:    jobWithCtx{ctx: context.Background(), job: j},
			record: &kgo.Record{Partition: 0, Offset: int64(i)},
		}
	}
	return batch
}

func makeBatchWithRetryCount(retryCount int, jobs ...ats.RawJob) []chunkItem {
	batch := make([]chunkItem, len(jobs))
	for i, j := range jobs {
		batch[i] = chunkItem{
			job:    jobWithCtx{ctx: context.Background(), job: j, retryCount: retryCount},
			record: &kgo.Record{Partition: 0, Offset: int64(i)},
		}
	}
	return batch
}

// makeRawRecord builds a kgo.Record with a JSON-encoded job and the retry_count
// header. Used to drive processFetch end-to-end.
func makeRawRecord(t *testing.T, id string, retryCount int, partition int32, offset int64) *kgo.Record {
	t.Helper()
	payload, err := json.Marshal(makeJob(id))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	headers := []kgo.RecordHeader{
		{Key: kafka.HeaderRetryCount, Value: []byte(strconv.Itoa(retryCount))},
	}
	return &kgo.Record{
		Value:     payload,
		Headers:   headers,
		Partition: partition,
		Offset:    offset,
	}
}

func headerValue(h []kgo.RecordHeader, key string) (string, bool) {
	for _, rh := range h {
		if rh.Key == key {
			return string(rh.Value), true
		}
	}
	return "", false
}

// ── processChunk tests (domain-level) ─────────────────────────────────────────

func TestProcessChunk_AllRelevant(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"job-1", "job-2", "job-3"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.processChunk(context.Background(), makeBatch(
		makeJob("job-1"), makeJob("job-2"), makeJob("job-3"),
	), "fetch-1", 0, 1)

	if len(pub.payloads) != 3 {
		t.Errorf("expected 3 published jobs, got %d", len(pub.payloads))
	}
	if len(ledger.calls) != 0 {
		t.Errorf("expected no ledger calls, got %d", len(ledger.calls))
	}
}

func TestProcessChunk_PartialRelevant(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"job-1", "job-3"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.processChunk(context.Background(), makeBatch(
		makeJob("job-1"), makeJob("job-2"), makeJob("job-3"),
	), "fetch-1", 0, 1)

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
// the 1m retry tier with retry_count=1 and next_attempt_at = clock.Now()+1m.
// UpdateRetryBatch receives matching entries; no reason='error' ledger write.
func TestProcessChunk_ScorerError_RoutesToRetry(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	w := makeWorker(scorer, ledger, pub)
	w.clock = &fakeClock{now: now}

	w.processChunk(context.Background(), makeBatch(
		makeJob("job-1"), makeJob("job-2"),
	), "fetch-1", 0, 1)

	retryRecords := pub.recordsOnTopic(kafka.TopicJobListingsRawRetry1m)
	if len(retryRecords) != 2 {
		t.Fatalf("expected 2 records on retry.1m, got %d (records: %+v)", len(retryRecords), pub.records)
	}
	if len(pub.recordsOnTopic(kafka.TopicJobListingsRawDLQ)) != 0 {
		t.Errorf("expected 0 records on DLQ")
	}

	want := now.Add(1 * time.Minute).UnixMilli()
	for _, rec := range retryRecords {
		rc, ok := headerValue(rec.headers, kafka.HeaderRetryCount)
		if !ok || rc != "1" {
			t.Errorf("expected retry_count '1', got %q (present=%v)", rc, ok)
		}
		naStr, ok := headerValue(rec.headers, kafka.HeaderNextAttemptAt)
		if !ok {
			t.Fatalf("missing next_attempt_at header")
		}
		if naStr != strconv.FormatInt(want, 10) {
			t.Errorf("expected next_attempt_at %d, got %s", want, naStr)
		}
	}

	if len(ledger.retryBatches) != 1 {
		t.Fatalf("expected 1 UpdateRetryBatch call, got %d", len(ledger.retryBatches))
	}
	entries := ledger.retryBatches[0]
	if len(entries) != 2 {
		t.Fatalf("expected 2 retry entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.RetryCount != 1 {
			t.Errorf("expected retry_count=1, got %d for %s", e.RetryCount, e.JobID)
		}
		if !e.NextAttemptAt.Equal(now.Add(1 * time.Minute)) {
			t.Errorf("expected next_attempt_at=%v, got %v", now.Add(1*time.Minute), e.NextAttemptAt)
		}
	}
	for _, call := range ledger.calls {
		if call.reason == jobcommon.SeenReasonError {
			t.Errorf("unexpected reason='error' ledger call on transient retry path")
		}
	}
}

// Exhausted retry count → all DLQ'd, UpdateSeenReasonBatch(error) called.
func TestProcessChunk_ScorerError_ExhaustedGoesToDLQ(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.processChunk(context.Background(), makeBatchWithRetryCount(3,
		makeJob("job-1"), makeJob("job-2"),
	), "fetch-1", 0, 1)

	dlq := pub.recordsOnTopic(kafka.TopicJobListingsRawDLQ)
	if len(dlq) != 2 {
		t.Fatalf("expected 2 records on DLQ, got %d", len(dlq))
	}
	if len(ledger.retryBatches) != 0 {
		t.Errorf("expected no UpdateRetryBatch call on exhausted path, got %d", len(ledger.retryBatches))
	}
	if len(ledger.calls) != 1 {
		t.Fatalf("expected 1 ledger call marking error, got %d", len(ledger.calls))
	}
	if ledger.calls[0].reason != jobcommon.SeenReasonError {
		t.Errorf("expected reason=error, got %q", ledger.calls[0].reason)
	}
}

// ErrLLMValidation → all jobs DLQ'd regardless of retry_count.
func TestProcessChunk_ValidationErrorGoesStraightToDLQ(t *testing.T) {
	scorer := &fakeScorer{err: domainerr.ErrLLMValidation}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.processChunk(context.Background(), makeBatchWithRetryCount(0,
		makeJob("job-1"), makeJob("job-2"),
	), "fetch-1", 0, 1)

	if len(pub.recordsOnTopic(kafka.TopicJobListingsRawRetry1m)) != 0 {
		t.Errorf("expected no retry.1m records for validation error")
	}
	dlq := pub.recordsOnTopic(kafka.TopicJobListingsRawDLQ)
	if len(dlq) != 2 {
		t.Fatalf("expected 2 DLQ records, got %d", len(dlq))
	}
	if len(ledger.retryBatches) != 0 {
		t.Errorf("expected no UpdateRetryBatch on validation")
	}
	if len(ledger.calls) != 1 || ledger.calls[0].reason != jobcommon.SeenReasonError {
		t.Errorf("expected single error ledger call, got %+v", ledger.calls)
	}
}

// Mixed retry counts under a generic error: some retried, some DLQ'd in same chunk.
func TestProcessChunk_MixedRetryCounts(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	chunk := []chunkItem{
		{job: jobWithCtx{ctx: context.Background(), job: makeJob("job-a"), retryCount: 0}, record: &kgo.Record{Offset: 1}},
		{job: jobWithCtx{ctx: context.Background(), job: makeJob("job-b"), retryCount: 3}, record: &kgo.Record{Offset: 2}},
	}
	w.processChunk(context.Background(), chunk, "fetch-1", 0, 1)

	if len(pub.recordsOnTopic(kafka.TopicJobListingsRawRetry1m)) != 1 {
		t.Errorf("expected 1 record on retry.1m, got %d", len(pub.recordsOnTopic(kafka.TopicJobListingsRawRetry1m)))
	}
	if len(pub.recordsOnTopic(kafka.TopicJobListingsRawDLQ)) != 1 {
		t.Errorf("expected 1 record on DLQ, got %d", len(pub.recordsOnTopic(kafka.TopicJobListingsRawDLQ)))
	}
	if len(ledger.retryBatches) != 1 || len(ledger.retryBatches[0]) != 1 {
		t.Errorf("expected 1 retry entry, got %+v", ledger.retryBatches)
	}
	foundErr := false
	for _, call := range ledger.calls {
		if call.reason == jobcommon.SeenReasonError && len(call.ids) == 1 && call.ids[0] == "job-b" {
			foundErr = true
		}
	}
	if !foundErr {
		t.Errorf("expected error ledger call for job-b, got %+v", ledger.calls)
	}
}

// UpdateRetryBatch DB failure → logged, not propagated; chunk processing completes.
func TestProcessChunk_UpdateRetryBatchFailure(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{retryErr: errors.New("db down")}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.processChunk(context.Background(), makeBatch(makeJob("job-1")), "fetch-1", 0, 1)

	// Retry publish still happened.
	if len(pub.recordsOnTopic(kafka.TopicJobListingsRawRetry1m)) != 1 {
		t.Errorf("expected retry publish to succeed despite ledger failure")
	}
}

func TestProcessChunk_EmptyBatch(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.processChunk(context.Background(), makeBatch(), "fetch-1", 0, 1)

	if scorer.calls != 0 {
		t.Errorf("expected scorer not called on empty batch, got %d", scorer.calls)
	}
	if len(pub.payloads) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.payloads))
	}
}

// ── processFetch tests ────────────────────────────────────────────────────────

// N scoreable chunks: N-1 sleeps of llmCallInterval each, CommitRecords called N times.
func TestProcessFetch_SerialChunkExecutionAndCommits(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: nil} // all irrelevant → no publish failures
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now()}

	// batchSize=2 → 3 records produce 2 chunks? actually 3 records / 2 = 2 chunks
	// Use batchSize=1 with 3 records → 3 chunks, 2 inter-chunk sleeps.
	w := makeWorkerFull(scorer, ledger, pub, commit, clk, 1)

	records := []*kgo.Record{
		makeRawRecord(t, "job-1", 0, 0, 10),
		makeRawRecord(t, "job-2", 0, 0, 11),
		makeRawRecord(t, "job-3", 0, 0, 12),
	}

	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("processFetch: %v", err)
	}

	if len(clk.sleeps) != 2 {
		t.Errorf("expected 2 inter-chunk sleeps, got %d", len(clk.sleeps))
	}
	for _, d := range clk.sleeps {
		if d != w.llmCallInterval {
			t.Errorf("expected sleep=%v, got %v", w.llmCallInterval, d)
		}
	}

	if len(commit.batches) != 3 {
		t.Fatalf("expected 3 CommitRecords calls, got %d", len(commit.batches))
	}
	// Offsets committed in order 10, 11, 12.
	for i, want := range []int64{10, 11, 12} {
		got := commit.batches[i].records
		if len(got) != 1 || got[0].Offset != want {
			t.Errorf("commit batch %d: expected offset %d, got %+v", i, want, got)
		}
	}
}

// processFetch with ScoreBatch error: UpdateRetryBatch invoked with correct entries,
// chunk committed, next chunk proceeds.
func TestProcessFetch_ScoreBatchFailureRetriesAndCommits(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	commit := &fakeCommitter{}
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}

	w := makeWorkerFull(scorer, ledger, pub, commit, clk, 2)
	records := []*kgo.Record{
		makeRawRecord(t, "job-1", 0, 0, 10),
		makeRawRecord(t, "job-2", 0, 0, 11),
	}

	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("processFetch: %v", err)
	}

	if len(ledger.retryBatches) != 1 || len(ledger.retryBatches[0]) != 2 {
		t.Fatalf("expected 1 UpdateRetryBatch call with 2 entries, got %+v", ledger.retryBatches)
	}
	for _, e := range ledger.retryBatches[0] {
		if e.RetryCount != 1 {
			t.Errorf("expected retry_count=1, got %d", e.RetryCount)
		}
		if !e.NextAttemptAt.Equal(now.Add(1 * time.Minute)) {
			t.Errorf("expected next_attempt_at=%v, got %v", now.Add(1*time.Minute), e.NextAttemptAt)
		}
	}
	if len(commit.batches) != 1 {
		t.Errorf("expected 1 commit, got %d", len(commit.batches))
	}
}

// Ctx cancellation mid-sleep: fake clock returns ctx.Err; next chunk not started.
func TestProcessFetch_CtxCancelMidSleep(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: nil}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now(), sleepErr: context.Canceled}

	w := makeWorkerFull(scorer, ledger, pub, commit, clk, 1)
	records := []*kgo.Record{
		makeRawRecord(t, "job-1", 0, 0, 10),
		makeRawRecord(t, "job-2", 0, 0, 11),
		makeRawRecord(t, "job-3", 0, 0, 12),
	}

	err := w.processFetch(context.Background(), records)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// First chunk processed + committed before the sleep. Second chunk aborts.
	if len(commit.batches) != 1 {
		t.Errorf("expected exactly 1 commit before ctx cancel, got %d", len(commit.batches))
	}
	if scorer.calls != 1 {
		t.Errorf("expected 1 scorer call before cancel, got %d", scorer.calls)
	}
}

// CommitRecords error: counter incremented, next chunk still runs.
func TestProcessFetch_CommitRecordsErrorContinues(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: nil}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	commit := &fakeCommitter{errQueue: []error{errors.New("commit boom")}}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerFull(scorer, ledger, pub, commit, clk, 1)
	records := []*kgo.Record{
		makeRawRecord(t, "job-1", 0, 0, 10),
		makeRawRecord(t, "job-2", 0, 0, 11),
	}

	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	if scorer.calls != 2 {
		t.Errorf("expected 2 scorer calls, got %d", scorer.calls)
	}
	// Both chunks attempted commit (first errored, second OK).
	if len(commit.batches) != 2 {
		t.Errorf("expected 2 commit attempts, got %d", len(commit.batches))
	}
}

// Empty items slice (all malformed): returns nil, no panic, no commit attempts.
func TestProcessFetch_AllMalformedReturnsNil(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerFull(scorer, ledger, pub, commit, clk, 5)
	records := []*kgo.Record{
		{Value: []byte("not json"), Partition: 0, Offset: 1},
		{Value: []byte("{{{"), Partition: 0, Offset: 2},
	}

	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	// Malformed records are DLQ'd then committed so they don't re-deliver.
	// All 2 malformed records land in one batch commit.
	if len(commit.batches) != 1 {
		t.Errorf("expected 1 commit (malformed batch), got %d", len(commit.batches))
	}
	if scorer.calls != 0 {
		t.Errorf("expected no scorer calls, got %d", scorer.calls)
	}
}

// Partial-partition chunk: one chunk spans two partitions; CommitRecords called
// with both records (consumer commits max offset per partition).
func TestProcessFetch_PartialPartitionChunk(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: nil}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	commit := &fakeCommitter{}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerFull(scorer, ledger, pub, commit, clk, 5)
	records := []*kgo.Record{
		makeRawRecord(t, "job-a", 0, 0, 10), // partition 0
		makeRawRecord(t, "job-b", 0, 1, 20), // partition 1
	}

	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	if len(commit.batches) != 1 {
		t.Fatalf("expected 1 commit batch, got %d", len(commit.batches))
	}
	got := commit.batches[0].records
	if len(got) != 2 {
		t.Fatalf("expected commit with 2 records, got %d", len(got))
	}
	partitions := map[int32]int64{}
	for _, r := range got {
		partitions[r.Partition] = r.Offset
	}
	if partitions[0] != 10 || partitions[1] != 20 {
		t.Errorf("expected per-partition offsets {0:10, 1:20}, got %+v", partitions)
	}
}

// Ensure chunk retry_count increment propagation works over processFetch path too.
func TestProcessFetch_RetryCountAdvances(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	commit := &fakeCommitter{}
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}

	w := makeWorkerFull(scorer, ledger, pub, commit, clk, 5)
	records := []*kgo.Record{
		makeRawRecord(t, "job-1", 1, 0, 10), // incoming retry_count=1 → goes to 10m tier
	}

	if err := w.processFetch(context.Background(), records); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	if len(pub.recordsOnTopic(kafka.TopicJobListingsRawRetry10m)) != 1 {
		t.Errorf("expected 1 record on retry.10m")
	}
	if len(ledger.retryBatches) != 1 || ledger.retryBatches[0][0].RetryCount != 2 {
		t.Errorf("expected retry_count=2 ledger entry, got %+v", ledger.retryBatches)
	}
}
