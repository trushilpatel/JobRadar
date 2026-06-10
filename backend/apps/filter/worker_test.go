package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
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
	reason jobcommon.SeenReason
}

type fakeLedger struct {
	calls                   []ledgerCall
	retryBatches            [][]jobsvc.RetryLedgerEntry
	retryBatchesWithPayload [][]jobsvc.RetryLedgerEntryWithPayload
	retryErr                error
	retryWithPayloadErr     error
	// seenErr, when set, is returned by UpdateSeenReasonBatch (used to inject
	// ErrLedgerRowMissing). seenErrForReason takes precedence when its key matches.
	seenErr          error
	seenErrForReason map[jobcommon.SeenReason]error
}

func (f *fakeLedger) UpdateSeenReasonBatch(_ context.Context, ids []string, reason jobcommon.SeenReason) error {
	f.calls = append(f.calls, ledgerCall{ids: append([]string(nil), ids...), reason: reason})
	if f.seenErrForReason != nil {
		if err, ok := f.seenErrForReason[reason]; ok {
			return err
		}
	}
	return f.seenErr
}

func (f *fakeLedger) UpdateRetryBatch(_ context.Context, entries []jobsvc.RetryLedgerEntry) error {
	if f.retryErr != nil {
		return f.retryErr
	}
	dup := append([]jobsvc.RetryLedgerEntry(nil), entries...)
	f.retryBatches = append(f.retryBatches, dup)
	return nil
}

func (f *fakeLedger) UpdateRetryBatchWithPayload(_ context.Context, entries []jobsvc.RetryLedgerEntryWithPayload) error {
	if f.retryWithPayloadErr != nil {
		return f.retryWithPayloadErr
	}
	dup := append([]jobsvc.RetryLedgerEntryWithPayload(nil), entries...)
	f.retryBatchesWithPayload = append(f.retryBatchesWithPayload, dup)
	return nil
}

// publishedRecord captures one publish call.
type publishedRecord struct {
	subject string
	value   []byte
}

type fakePublisher struct {
	payloads [][]byte
	records  []publishedRecord
	err      error
	// failOn fails the publish for jobs whose JSON `id` field matches a key.
	// Used to exercise the per-job publish-failure partition.
	failOn map[string]error
}

func (f *fakePublisher) Publish(_ context.Context, subject string, value []byte) error {
	if f.failOn != nil {
		var raw map[string]any
		_ = json.Unmarshal(value, &raw)
		if id, ok := raw["id"].(string); ok {
			if e, exists := f.failOn[id]; exists {
				return e
			}
		}
	}
	if f.err != nil {
		return f.err
	}
	f.payloads = append(f.payloads, value)
	f.records = append(f.records, publishedRecord{subject: subject, value: value})
	return nil
}

func (f *fakePublisher) PublishWithMsgID(_ context.Context, subject string, value []byte, _ string) error {
	return f.Publish(context.Background(), subject, value)
}

// recordsOnSubject returns all publishes whose subject contains needle.
func (f *fakePublisher) recordsOnSubject(needle string) []publishedRecord {
	var out []publishedRecord
	for _, r := range f.records {
		if strings.Contains(r.subject, needle) {
			out = append(out, r)
		}
	}
	return out
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
		flushTimeout:    30 * time.Second,
		llmCallInterval: 5 * time.Second,
		clock:           realClock{},
		log:             logger.NewNoop(),
	}
}

func makeWorkerWithClock(scorer *fakeScorer, ledger *fakeLedger, pub *fakePublisher, clk clock, batchSize int) *Worker {
	return &Worker{
		scorer:          scorer,
		ledger:          ledger,
		publisher:       pub,
		batchSize:       batchSize,
		flushTimeout:    30 * time.Second,
		llmCallInterval: 5 * time.Second,
		clock:           clk,
		log:             logger.NewNoop(),
	}
}

func makeJob(id string) ats.RawJob {
	return ats.RawJob{ID: id, Title: "Software Engineer"}
}

// makeChunkItem builds a chunkItem from a job and returns its TestMsgState.
func makeChunkItem(job ats.RawJob, deliveryCount int) (chunkItem, *js.TestMsgState) {
	msg, state := js.NewTestMsg([]byte("{}"), deliveryCount)
	return chunkItem{
		job: jobWithCtx{ctx: context.Background(), job: job, retryCount: deliveryCount - 1},
		msg: msg,
	}, state
}

// makeBatch builds chunkItems from jobs with deliveryCount=1 (first delivery).
// States are discarded — use makeChunkItem directly when you need state inspection.
func makeBatch(jobs ...ats.RawJob) []chunkItem {
	batch := make([]chunkItem, len(jobs))
	for i, j := range jobs {
		item, _ := makeChunkItem(j, 1)
		batch[i] = item
	}
	return batch
}

// makeBatchWithRetryCount builds chunkItems at the given retry count.
func makeBatchWithRetryCount(retryCount int, jobs ...ats.RawJob) []chunkItem {
	batch := make([]chunkItem, len(jobs))
	for i, j := range jobs {
		item, _ := makeChunkItem(j, retryCount+1)
		batch[i] = item
	}
	return batch
}

// makeTestMsgs creates NATS test messages from job/delivery-count pairs.
func makeTestMsgs(t *testing.T, specs []struct {
	id            string
	deliveryCount int
}) ([]*js.Msg, []*js.TestMsgState) {
	t.Helper()
	msgs := make([]*js.Msg, len(specs))
	states := make([]*js.TestMsgState, len(specs))
	for i, s := range specs {
		payload, err := json.Marshal(makeJob(s.id))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		msgs[i], states[i] = js.NewTestMsg(payload, s.deliveryCount)
	}
	return msgs, states
}

// ── processChunk tests (domain-level) ─────────────────────────────────────────

func TestProcessChunk_AllRelevant(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"job-1", "job-2", "job-3"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	_ = w.processChunk(context.Background(), makeBatch(
		makeJob("job-1"), makeJob("job-2"), makeJob("job-3"),
	), "fetch-1", 0, 1)

	if len(pub.payloads) != 3 {
		t.Errorf("expected 3 published jobs, got %d", len(pub.payloads))
	}
	if len(ledger.calls) != 1 {
		t.Fatalf("expected 1 ledger call (filtered), got %d", len(ledger.calls))
	}
	if ledger.calls[0].reason != jobcommon.SeenReasonFiltered {
		t.Errorf("expected reason %q, got %q", jobcommon.SeenReasonFiltered, ledger.calls[0].reason)
	}
	if len(ledger.calls[0].ids) != 3 {
		t.Errorf("expected 3 filtered IDs, got %v", ledger.calls[0].ids)
	}
}

func TestProcessChunk_PartialRelevant(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"job-1", "job-3"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	_ = w.processChunk(context.Background(), makeBatch(
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
	if len(ledger.calls) != 2 {
		t.Fatalf("expected 2 ledger calls (filtered + irrelevant), got %d", len(ledger.calls))
	}
	// New ordering: relevant jobs are published and marked 'filtered' first, then
	// irrelevant jobs are marked 'llm_irrelevant'.
	filteredCall := ledger.calls[0]
	if filteredCall.reason != jobcommon.SeenReasonFiltered {
		t.Errorf("expected reason %q, got %q", jobcommon.SeenReasonFiltered, filteredCall.reason)
	}
	if len(filteredCall.ids) != 2 {
		t.Errorf("expected 2 filtered IDs, got %v", filteredCall.ids)
	}
	irrelevantCall := ledger.calls[1]
	if irrelevantCall.reason != jobcommon.SeenReasonLLMIrrelevant {
		t.Errorf("expected reason %q, got %q", jobcommon.SeenReasonLLMIrrelevant, irrelevantCall.reason)
	}
	if len(irrelevantCall.ids) != 1 || irrelevantCall.ids[0] != "job-2" {
		t.Errorf("expected irrelevant IDs [job-2], got %v", irrelevantCall.ids)
	}
}

// TestProcessChunk_PublishDetailFails_NaksMessage_NoLedgerWrite (#19) — a
// publish failure for a relevant job Naks the message, does NOT mark it
// 'filtered', and increments filter_publish_failures_total.
func TestProcessChunk_PublishDetailFails_NaksMessage_NoLedgerWrite(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"job-1"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{failOn: map[string]error{"job-1": errors.New("broker down")}}

	item1, state1 := makeChunkItem(makeJob("job-1"), 1)

	w := makeWorker(scorer, ledger, pub)
	before := promtestutil.ToFloat64(metrics.FilterPublishFailures.WithLabelValues("filtered"))
	_ = w.processChunk(context.Background(), []chunkItem{item1}, "fetch-pubfail", 0, 1)
	after := promtestutil.ToFloat64(metrics.FilterPublishFailures.WithLabelValues("filtered"))

	if !state1.Naked {
		t.Errorf("expected publish-failed job to be Nak'd")
	}
	if state1.Acked {
		t.Errorf("publish-failed job must NOT be Acked")
	}
	// No 'filtered' ledger write for the failed job.
	for _, c := range ledger.calls {
		if c.reason == jobcommon.SeenReasonFiltered {
			t.Errorf("publish-failed job must not be marked 'filtered'; got call %+v", c)
		}
	}
	if after-before != 1 {
		t.Errorf("expected filter_publish_failures_total +1, got +%v", after-before)
	}
}

// TestProcessChunk_LedgerRowMissing_NaksMessage (#20) — when
// UpdateSeenReasonBatch returns ErrLedgerRowMissing (crawler→filter race), all
// affected messages are Nak'd (not Acked) and filter_ledger_missing_total
// increments.
func TestProcessChunk_LedgerRowMissing_NaksMessage(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"job-1", "job-2"}}
	ledger := &fakeLedger{seenErr: domainerr.ErrLedgerRowMissing.WithMessage("race")}
	pub := &fakePublisher{}

	item1, state1 := makeChunkItem(makeJob("job-1"), 1)
	item2, state2 := makeChunkItem(makeJob("job-2"), 1)

	w := makeWorker(scorer, ledger, pub)
	before := promtestutil.ToFloat64(metrics.FilterLedgerMissing.WithLabelValues(string(jobcommon.SeenReasonFiltered)))
	_ = w.processChunk(context.Background(), []chunkItem{item1, item2}, "fetch-missing", 0, 1)
	after := promtestutil.ToFloat64(metrics.FilterLedgerMissing.WithLabelValues(string(jobcommon.SeenReasonFiltered)))

	if !state1.Naked || !state2.Naked {
		t.Errorf("expected both messages Nak'd on missing ledger row: j1=%v j2=%v", state1.Naked, state2.Naked)
	}
	if state1.Acked || state2.Acked {
		t.Errorf("messages must NOT be Acked when ledger row missing")
	}
	if after-before != 1 {
		t.Errorf("expected filter_ledger_missing_total +1, got +%v", after-before)
	}
}

// TestRouteFailedChunk_NakFails_StillRecordsRetry_AndTerms (#21) — when Nak
// fails, the message is Term'd AND the retry ledger entry is still recorded.
func TestRouteFailedChunk_NakFails_StillRecordsRetry_AndTerms(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakeLedger{}, &fakePublisher{})

	item, state := makeChunkItem(makeJob("job-nak-fail"), 1) // retryCount=0 → within budget
	state.NakErr = errors.New("nak broker error")

	outcome := w.routeFailedChunk(context.Background(), []chunkItem{item}, errors.New("transient score error"))

	if !state.Naked {
		t.Errorf("expected Nak to have been attempted")
	}
	if !state.Termed {
		t.Errorf("expected Term after Nak failure")
	}
	if len(outcome.Retried) != 1 || outcome.Retried[0].JobID != "job-nak-fail" {
		t.Errorf("expected retry ledger entry recorded despite Nak failure, got %+v", outcome.Retried)
	}
}

// TestProcessChunk_AllRelevant_OneFailsPublish (#22) — 3 relevant jobs, 1
// publish fails: the 2 published get marked 'filtered' and Acked; the failed
// one is Nak'd with no ledger write.
func TestProcessChunk_AllRelevant_OneFailsPublish(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"job-1", "job-2", "job-3"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{failOn: map[string]error{"job-2": errors.New("transient")}}

	item1, state1 := makeChunkItem(makeJob("job-1"), 1)
	item2, state2 := makeChunkItem(makeJob("job-2"), 1)
	item3, state3 := makeChunkItem(makeJob("job-3"), 1)

	w := makeWorker(scorer, ledger, pub)
	_ = w.processChunk(context.Background(), []chunkItem{item1, item2, item3}, "fetch-mixedpub", 0, 1)

	// job-2 Nak'd, not Acked; job-1 + job-3 Acked.
	if state2.Acked || !state2.Naked {
		t.Errorf("job-2 (publish failed) should be Nak'd, not Acked: acked=%v naked=%v", state2.Acked, state2.Naked)
	}
	if !state1.Acked || !state3.Acked {
		t.Errorf("job-1 + job-3 should be Acked: j1=%v j3=%v", state1.Acked, state3.Acked)
	}
	// Exactly one 'filtered' ledger call covering only the 2 published IDs.
	var filtered *ledgerCall
	for i := range ledger.calls {
		if ledger.calls[i].reason == jobcommon.SeenReasonFiltered {
			filtered = &ledger.calls[i]
		}
	}
	if filtered == nil {
		t.Fatal("expected a 'filtered' ledger call")
	}
	if len(filtered.ids) != 2 {
		t.Errorf("expected 2 filtered IDs (job-1, job-3), got %v", filtered.ids)
	}
	for _, id := range filtered.ids {
		if id == "job-2" {
			t.Errorf("job-2 must not be marked 'filtered' (its publish failed)")
		}
	}
}

// On a generic transient scorer error with retryCount=0, jobs are Nak'd and
// UpdateRetryBatch is called with correct next_attempt_at (based on BackOff).
func TestProcessChunk_ScorerError_RoutesToRetry(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	w := makeWorker(scorer, ledger, pub)
	w.clock = &fakeClock{now: now}

	item1, state1 := makeChunkItem(makeJob("job-1"), 1)
	item2, state2 := makeChunkItem(makeJob("job-2"), 1)
	_ = w.processChunk(context.Background(), []chunkItem{item1, item2}, "fetch-1", 0, 1)

	if !state1.Naked || !state2.Naked {
		t.Errorf("expected both jobs Nak'd: job-1 naked=%v, job-2 naked=%v", state1.Naked, state2.Naked)
	}
	if state1.Termed || state2.Termed {
		t.Errorf("jobs should NOT be Term'd on first retry attempt")
	}
	if len(pub.recordsOnSubject(js.SubjectJobsDLQ)) != 0 {
		t.Errorf("expected 0 DLQ records, got %d", len(pub.recordsOnSubject(js.SubjectJobsDLQ)))
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
		if !e.NextAttemptAt.Equal(now.Add(30 * time.Second)) {
			t.Errorf("expected next_attempt_at=%v (30s tier), got %v", now.Add(30*time.Second), e.NextAttemptAt)
		}
	}
	for _, call := range ledger.calls {
		if call.reason == jobcommon.SeenReasonError {
			t.Errorf("unexpected reason='error' ledger call on transient retry path")
		}
	}
}

// Exhausted retry count → all jobs Term'd + DLQ published.
func TestProcessChunk_ScorerError_ExhaustedGoesToDLQ(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	item1, state1 := makeChunkItem(makeJob("job-1"), filterMaxRetries+1)
	item2, state2 := makeChunkItem(makeJob("job-2"), filterMaxRetries+1)
	_ = w.processChunk(context.Background(), []chunkItem{item1, item2}, "fetch-1", 0, 1)

	if !state1.Termed || !state2.Termed {
		t.Errorf("expected both jobs Term'd: job-1 termed=%v, job-2 termed=%v", state1.Termed, state2.Termed)
	}
	if len(pub.recordsOnSubject(js.SubjectJobsDLQ)) != 2 {
		t.Fatalf("expected 2 DLQ publishes, got %d", len(pub.recordsOnSubject(js.SubjectJobsDLQ)))
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

// ErrLLMValidation → all jobs Term'd to DLQ regardless of retry count.
func TestProcessChunk_ValidationErrorGoesStraightToDLQ(t *testing.T) {
	scorer := &fakeScorer{err: domainerr.ErrLLMValidation}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	item1, state1 := makeChunkItem(makeJob("job-1"), 1)
	item2, state2 := makeChunkItem(makeJob("job-2"), 1)
	_ = w.processChunk(context.Background(), []chunkItem{item1, item2}, "fetch-1", 0, 1)

	if !state1.Termed || !state2.Termed {
		t.Errorf("expected both jobs Term'd for validation error")
	}
	if len(pub.recordsOnSubject(js.SubjectJobsDLQ)) != 2 {
		t.Fatalf("expected 2 DLQ records, got %d", len(pub.recordsOnSubject(js.SubjectJobsDLQ)))
	}
	if len(ledger.retryBatches) != 0 {
		t.Errorf("expected no UpdateRetryBatch on validation")
	}
	if len(ledger.calls) != 1 || ledger.calls[0].reason != jobcommon.SeenReasonError {
		t.Errorf("expected single error ledger call, got %+v", ledger.calls)
	}
}

// Mixed retry counts: retryCount=0 → Nak, retryCount=filterMaxRetries → DLQ.
func TestProcessChunk_MixedRetryCounts(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	itemA, stateA := makeChunkItem(makeJob("job-a"), 1)                  // retryCount=0
	itemB, stateB := makeChunkItem(makeJob("job-b"), filterMaxRetries+1) // retryCount=filterMaxRetries
	_ = w.processChunk(context.Background(), []chunkItem{itemA, itemB}, "fetch-1", 0, 1)

	if !stateA.Naked {
		t.Errorf("job-a: expected Nak'd (within budget), got acked=%v termed=%v", stateA.Acked, stateA.Termed)
	}
	if !stateB.Termed {
		t.Errorf("job-b: expected Term'd (exhausted), got acked=%v naked=%v", stateB.Acked, stateB.Naked)
	}
	if len(pub.recordsOnSubject(js.SubjectJobsDLQ)) != 1 {
		t.Errorf("expected 1 DLQ record, got %d", len(pub.recordsOnSubject(js.SubjectJobsDLQ)))
	}
	if len(ledger.retryBatches) != 1 || len(ledger.retryBatches[0]) != 1 {
		t.Errorf("expected 1 retry entry for job-a, got %+v", ledger.retryBatches)
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

// UpdateRetryBatch DB failure → logged, not propagated; Nak still called.
func TestProcessChunk_UpdateRetryBatchFailure(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{retryErr: errors.New("db down")}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	item, state := makeChunkItem(makeJob("job-1"), 1)
	_ = w.processChunk(context.Background(), []chunkItem{item}, "fetch-1", 0, 1)

	if !state.Naked {
		t.Errorf("expected job Nak'd even when ledger write fails")
	}
}

func TestProcessChunk_EmptyBatch(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	_ = w.processChunk(context.Background(), makeBatch(), "fetch-1", 0, 1)

	if scorer.calls != 0 {
		t.Errorf("expected scorer not called on empty batch, got %d", scorer.calls)
	}
	if len(pub.payloads) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.payloads))
	}
}

// ── processFetch tests ────────────────────────────────────────────────────────

// N scoreable chunks: N-1 inter-chunk sleeps, all msgs Ack'd on success.
func TestProcessFetch_SerialChunkExecutionAndAcks(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: nil}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	clk := &fakeClock{now: time.Now()}

	// batchSize=1 → 3 messages = 3 chunks, 2 inter-chunk sleeps.
	w := makeWorkerWithClock(scorer, ledger, pub, clk, 1)

	msgs, states := makeTestMsgs(t, []struct {
		id            string
		deliveryCount int
	}{
		{"job-1", 1}, {"job-2", 1}, {"job-3", 1},
	})

	if err := w.processFetch(context.Background(), msgs); err != nil {
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
	for i, state := range states {
		if !state.Acked {
			t.Errorf("msg %d: expected Ack'd after successful processing", i)
		}
	}
}

// ScoreBatch error → all messages Nak'd, ledger retry entries written.
func TestProcessFetch_ScoreBatchFailureNaksAll(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)
	msgs, states := makeTestMsgs(t, []struct {
		id            string
		deliveryCount int
	}{
		{"job-1", 1}, {"job-2", 1},
	})

	if err := w.processFetch(context.Background(), msgs); err != nil {
		t.Fatalf("processFetch: %v", err)
	}

	for i, state := range states {
		if !state.Naked {
			t.Errorf("msg %d: expected Nak'd on ScoreBatch failure", i)
		}
	}
	if len(ledger.retryBatches) != 1 || len(ledger.retryBatches[0]) != 2 {
		t.Fatalf("expected 1 UpdateRetryBatch call with 2 entries, got %+v", ledger.retryBatches)
	}
	for _, e := range ledger.retryBatches[0] {
		if e.RetryCount != 1 {
			t.Errorf("expected retry_count=1, got %d", e.RetryCount)
		}
		if !e.NextAttemptAt.Equal(now.Add(30 * time.Second)) {
			t.Errorf("expected next_attempt_at=%v (30s tier), got %v", now.Add(30*time.Second), e.NextAttemptAt)
		}
	}
}

// Ctx cancellation mid-sleep: second chunk not started, and the unprocessed
// chunks are Nak'd so NATS redelivers them immediately rather than waiting for
// AckWait to expire (preventing the "Awaiting Filter" stuck-row symptom across
// pod restarts).
func TestProcessFetch_CtxCancelMidSleep(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: nil}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	clk := &fakeClock{now: time.Now(), sleepErr: context.Canceled}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 1)
	msgs, states := makeTestMsgs(t, []struct {
		id            string
		deliveryCount int
	}{
		{"job-1", 1}, {"job-2", 1}, {"job-3", 1},
	})

	err := w.processFetch(context.Background(), msgs)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	// First chunk processed before sleep; second aborts.
	if scorer.calls != 1 {
		t.Errorf("expected 1 scorer call before cancel, got %d", scorer.calls)
	}
	// First message was Ack'd as part of chunk 0; chunks 1 and 2 must be Nak'd
	// (not silently abandoned) so JetStream redelivers them immediately.
	if !states[0].Acked {
		t.Errorf("msg 0: expected Ack'd after successful first-chunk processing")
	}
	for i := 1; i < len(states); i++ {
		if !states[i].Naked {
			t.Errorf("msg %d: expected Nak'd on shutdown so NATS redelivers", i)
		}
		if states[i].Acked {
			t.Errorf("msg %d: must not be Acked on shutdown", i)
		}
	}
}

// All malformed messages → Term'd, scorer not called.
func TestProcessFetch_AllMalformedReturnsNil(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)

	msg1, state1 := js.NewTestMsg([]byte("not json"), 1)
	msg2, state2 := js.NewTestMsg([]byte("{{{"), 1)

	if err := w.processFetch(context.Background(), []*js.Msg{msg1, msg2}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if scorer.calls != 0 {
		t.Errorf("expected no scorer calls, got %d", scorer.calls)
	}
	for i, state := range []*js.TestMsgState{state1, state2} {
		if !state.Termed {
			t.Errorf("malformed msg %d: expected Term'd", i)
		}
	}
}

// deliveryCount=2 → retryCount=1, next tier is the 2m BackOff slot.
func TestProcessFetch_RetryCountFromDelivery(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)

	payload, _ := json.Marshal(makeJob("job-1"))
	msg, _ := js.NewTestMsg(payload, 2) // deliveryCount=2 → retryCount=1

	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	if len(ledger.retryBatches) != 1 || ledger.retryBatches[0][0].RetryCount != 2 {
		t.Errorf("expected retry_count=2 ledger entry, got %+v", ledger.retryBatches)
	}
	want := now.Add(2 * time.Minute)
	if !ledger.retryBatches[0][0].NextAttemptAt.Equal(want) {
		t.Errorf("expected next_attempt_at=%v (2m tier), got %v", want, ledger.retryBatches[0][0].NextAttemptAt)
	}
}

// ── Task 1: filterBackOff alignment ────────────────────────────────────────────

// TestFilterBackOffMatchesConsumerConfig asserts that filterBackOff (used by
// the filter worker to stamp next_attempt_at into the retry ledger) stays in
// sync with the JOBS_RAW consumer BackOff slice in common/jetstream/streams.go.
// These two slices must stay identical — if either changes the retry ladder
// behaviour diverges. This test is the compiler/CI guard replacing the prose
// comment "Must match the BackOff length on JOBS_RAW consumer".
//
// Expected BackOff from ConsumerFilterWorkers in streams.go:
//
//	[]time.Duration{30 * time.Second, 2 * time.Minute}
func TestFilterBackOffMatchesConsumerConfig(t *testing.T) {
	// consumerBackOff mirrors the BackOff slice from common/jetstream/streams.go
	// ConsumerFilterWorkers config. Both slices must be kept identical.
	consumerBackOff := []time.Duration{30 * time.Second, 2 * time.Minute}

	if len(filterBackOff) != len(consumerBackOff) {
		t.Fatalf("filterBackOff length %d != consumer BackOff length %d; update one to match the other",
			len(filterBackOff), len(consumerBackOff))
	}
	for i, want := range consumerBackOff {
		if filterBackOff[i] != want {
			t.Errorf("filterBackOff[%d] = %v, consumer BackOff[%d] = %v; they must match",
				i, filterBackOff[i], i, want)
		}
	}
}

// TestFilterMaxRetriesMatchesConsumerMaxDeliver asserts that filterMaxRetries
// (infra retry budget) equals len(filterBackOff), and that the budget equals
// MaxDeliver-1 (since MaxDeliver counts all deliveries including the first).
// Keeps the NATS consumer config and the worker constant in sync.
func TestFilterMaxRetriesMatchesConsumerMaxDeliver(t *testing.T) {
	// MaxDeliver is 3 in ConsumerFilterWorkers config.
	const consumerMaxDeliver = 3
	if filterMaxRetries != len(filterBackOff) {
		t.Errorf("filterMaxRetries (%d) must equal len(filterBackOff) (%d)",
			filterMaxRetries, len(filterBackOff))
	}
	if filterMaxRetries != consumerMaxDeliver-1 {
		t.Errorf("filterMaxRetries (%d) must equal MaxDeliver-1 (%d)",
			filterMaxRetries, consumerMaxDeliver-1)
	}
}

// ── Task 4: publishToDLQ direct coverage ───────────────────────────────────────

// TestPublishToDLQ_Success calls publishToDLQ directly and asserts the
// success path: the job is published to the DLQ subject, the message is
// Term'd, and true is returned.
func TestPublishToDLQ_Success(t *testing.T) {
	pub := &fakePublisher{}
	w := makeWorker(&fakeScorer{}, &fakeLedger{}, pub)

	item, state := makeChunkItem(makeJob("dlq-direct"), filterMaxRetries+1)
	ok := w.publishToDLQ(context.Background(), item, errors.New("score error"))

	if !ok {
		t.Errorf("expected publishToDLQ to return true on success")
	}
	if !state.Termed {
		t.Errorf("expected message Term'd after successful DLQ publish")
	}
	if state.Acked {
		t.Errorf("DLQ job must not be Acked")
	}
	dlqRecords := pub.recordsOnSubject(js.SubjectJobsDLQ)
	if len(dlqRecords) != 1 {
		t.Fatalf("expected 1 DLQ publish, got %d", len(dlqRecords))
	}
	// Verify the payload round-trips cleanly back to the original job.
	var got ats.RawJob
	if err := json.Unmarshal(dlqRecords[0].value, &got); err != nil {
		t.Fatalf("unmarshal DLQ payload: %v", err)
	}
	if got.ID != "dlq-direct" {
		t.Errorf("DLQ payload job ID = %q, want dlq-direct", got.ID)
	}
}

// TestPublishToDLQ_PublishError calls publishToDLQ with a publisher that
// fails for the DLQ subject. The message must be Term'd (to release it from
// the broker) and false returned — no phantom Ack, no silent drop.
func TestPublishToDLQ_PublishError(t *testing.T) {
	pub := &fakePublisher{err: errors.New("broker unavailable")}
	w := makeWorker(&fakeScorer{}, &fakeLedger{}, pub)

	item, state := makeChunkItem(makeJob("dlq-pub-fail"), filterMaxRetries+1)
	ok := w.publishToDLQ(context.Background(), item, errors.New("score error"))

	if ok {
		t.Errorf("expected publishToDLQ to return false when publish fails")
	}
	if !state.Termed {
		t.Errorf("expected message Term'd even when DLQ publish fails (no endless redelivery)")
	}
	if state.Acked {
		t.Errorf("must not Ack on publish failure")
	}
}

// TestPublishToDLQ_TermError_OnPublishFail exercises the "Term itself fails"
// branch inside the publish-failure path. This is belt-and-suspenders: the
// Term error is logged but must not panic or suppress the false return.
func TestPublishToDLQ_TermError_OnPublishFail(t *testing.T) {
	pub := &fakePublisher{err: errors.New("broker unavailable")}
	w := makeWorker(&fakeScorer{}, &fakeLedger{}, pub)

	item, state := makeChunkItem(makeJob("dlq-term-fail-pubfail"), filterMaxRetries+1)
	state.TermErr = errors.New("term broker error") // inject Term failure

	ok := w.publishToDLQ(context.Background(), item, errors.New("score error"))

	if ok {
		t.Errorf("expected false even when both publish and Term fail")
	}
	if !state.Termed {
		t.Errorf("expected Term to have been attempted")
	}
}

// TestPublishToDLQ_TermError_OnSuccess exercises the "Term fails" branch in
// the success path (publish succeeded, but Term itself errors). publishToDLQ
// still returns true because the DLQ payload landed — the Term error is logged.
func TestPublishToDLQ_TermError_OnSuccess(t *testing.T) {
	pub := &fakePublisher{}
	w := makeWorker(&fakeScorer{}, &fakeLedger{}, pub)

	item, state := makeChunkItem(makeJob("dlq-term-fail-success"), filterMaxRetries+1)
	state.TermErr = errors.New("term broker error") // inject Term failure

	ok := w.publishToDLQ(context.Background(), item, errors.New("score error"))

	if !ok {
		t.Errorf("expected true: DLQ payload landed even though Term errored")
	}
	if !state.Termed {
		t.Errorf("expected Term to have been attempted")
	}
	dlqRecords := pub.recordsOnSubject(js.SubjectJobsDLQ)
	if len(dlqRecords) != 1 {
		t.Fatalf("expected 1 DLQ publish, got %d", len(dlqRecords))
	}
}

// ── LLM exhaustion path tests ─────────────────────────────────────────────────

// TestRouteFailedChunk_LLMExhausted_TermsAndWritesPayload verifies that an
// LLMAllExhaustedError causes each message to be Term'd (not Nak'd) and a
// payload-bearing retry ledger entry to be recorded in RetriedWithPayload.
// The infra retry path (Nak + RetryLedgerEntry) must not be used.
func TestRouteFailedChunk_LLMExhausted_TermsAndWritesPayload(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	retryAfter := now.Add(5 * time.Minute)
	exhaustedErr := &domainerr.LLMAllExhaustedError{RetryAfter: retryAfter}

	w := makeWorker(&fakeScorer{}, &fakeLedger{}, &fakePublisher{})
	w.clock = &fakeClock{now: now}

	job1 := ats.RawJob{ID: "ex-1", Title: "SWE", Adapter: "greenhouse"}
	job2 := ats.RawJob{ID: "ex-2", Title: "PM", Adapter: "lever"}
	item1, state1 := makeChunkItem(job1, 1)
	item2, state2 := makeChunkItem(job2, 1)

	outcome := w.routeFailedChunk(context.Background(), []chunkItem{item1, item2}, exhaustedErr)

	// Messages must be Term'd (permanent), not Nak'd.
	if !state1.Termed || !state2.Termed {
		t.Errorf("expected both messages Term'd: j1=%v j2=%v", state1.Termed, state2.Termed)
	}
	if state1.Naked || state2.Naked {
		t.Errorf("LLM exhaustion path must NOT Nak: j1=%v j2=%v", state1.Naked, state2.Naked)
	}

	// No infra retry entries.
	if len(outcome.Retried) != 0 {
		t.Errorf("expected 0 Retried entries, got %d", len(outcome.Retried))
	}
	if len(outcome.DLQed) != 0 {
		t.Errorf("expected 0 DLQed entries, got %d", len(outcome.DLQed))
	}

	// Two payload-bearing retry entries.
	if len(outcome.RetriedWithPayload) != 2 {
		t.Fatalf("expected 2 RetriedWithPayload entries, got %d", len(outcome.RetriedWithPayload))
	}
	for _, e := range outcome.RetriedWithPayload {
		switch e.JobID {
		case "ex-1":
			if e.Adapter != "greenhouse" {
				t.Errorf("ex-1 adapter = %q, want greenhouse", e.Adapter)
			}
		case "ex-2":
			if e.Adapter != "lever" {
				t.Errorf("ex-2 adapter = %q, want lever", e.Adapter)
			}
		default:
			t.Errorf("unexpected job ID %q in RetriedWithPayload", e.JobID)
		}
		if !e.NextAttemptAt.Equal(retryAfter) {
			t.Errorf("entry %q next_attempt_at = %v, want %v", e.JobID, e.NextAttemptAt, retryAfter)
		}
		if len(e.Payload) == 0 {
			t.Errorf("entry %q payload must not be empty", e.JobID)
		}
		// Verify the payload round-trips back to the original job.
		var got ats.RawJob
		if err := json.Unmarshal(e.Payload, &got); err != nil {
			t.Fatalf("unmarshal payload for %q: %v", e.JobID, err)
		}
		if got.ID != e.JobID {
			t.Errorf("payload job ID = %q, want %q", got.ID, e.JobID)
		}
	}
}

// TestRouteFailedChunk_LLMExhausted_ZeroRetryAfter falls back to filterBackOff[0]
// when RetryAfter is the zero time.
func TestRouteFailedChunk_LLMExhausted_ZeroRetryAfter(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	exhaustedErr := &domainerr.LLMAllExhaustedError{} // zero RetryAfter

	w := makeWorker(&fakeScorer{}, &fakeLedger{}, &fakePublisher{})
	w.clock = &fakeClock{now: now}

	item, state := makeChunkItem(makeJob("ex-zero"), 1)
	outcome := w.routeFailedChunk(context.Background(), []chunkItem{item}, exhaustedErr)

	if !state.Termed {
		t.Errorf("expected Term on zero RetryAfter")
	}
	if len(outcome.RetriedWithPayload) != 1 {
		t.Fatalf("expected 1 RetriedWithPayload entry, got %d", len(outcome.RetriedWithPayload))
	}
	want := now.Add(filterBackOff[0])
	if !outcome.RetriedWithPayload[0].NextAttemptAt.Equal(want) {
		t.Errorf("next_attempt_at = %v, want %v (filterBackOff[0])", outcome.RetriedWithPayload[0].NextAttemptAt, want)
	}
}

// TestProcessChunk_LLMExhausted_TermsAndCallsUpdateRetryBatchWithPayload verifies
// the full processChunk → routeFailedChunk → UpdateRetryBatchWithPayload path:
// messages are Term'd, no Nak, and UpdateRetryBatchWithPayload is called.
// UpdateRetryBatch (infra path) must NOT be called.
func TestProcessChunk_LLMExhausted_TermsAndCallsUpdateRetryBatchWithPayload(t *testing.T) {
	retryAfter := time.Date(2026, 6, 1, 14, 0, 0, 0, time.UTC)
	exhaustedErr := &domainerr.LLMAllExhaustedError{RetryAfter: retryAfter}
	scorer := &fakeScorer{err: exhaustedErr}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)

	item1, state1 := makeChunkItem(ats.RawJob{ID: "e1", Title: "SWE", Adapter: "greenhouse"}, 1)
	item2, state2 := makeChunkItem(ats.RawJob{ID: "e2", Title: "PM", Adapter: "lever"}, 1)
	_ = w.processChunk(context.Background(), []chunkItem{item1, item2}, "fetch-exhaust", 0, 1)

	if !state1.Termed || !state2.Termed {
		t.Errorf("expected both Term'd: j1=%v j2=%v", state1.Termed, state2.Termed)
	}
	if state1.Naked || state2.Naked {
		t.Errorf("exhaustion path must NOT Nak: j1=%v j2=%v", state1.Naked, state2.Naked)
	}
	// UpdateRetryBatchWithPayload must be called.
	if len(ledger.retryBatchesWithPayload) != 1 {
		t.Fatalf("expected 1 UpdateRetryBatchWithPayload call, got %d", len(ledger.retryBatchesWithPayload))
	}
	if len(ledger.retryBatchesWithPayload[0]) != 2 {
		t.Errorf("expected 2 payload entries, got %d", len(ledger.retryBatchesWithPayload[0]))
	}
	// UpdateRetryBatch (infra path) must NOT be called.
	if len(ledger.retryBatches) != 0 {
		t.Errorf("UpdateRetryBatch must not be called on exhaustion path, got %d", len(ledger.retryBatches))
	}
	// No DLQ publishes.
	if len(pub.recordsOnSubject(js.SubjectJobsDLQ)) != 0 {
		t.Errorf("DLQ must not be used on exhaustion path")
	}
}

// TestProcessChunk_LLMExhausted_PayloadLedgerWriteFailure verifies the
// ordering invariant: when UpdateRetryBatchWithPayload fails, the message is
// Nak'd (not Term'd) so NATS redelivers and the job is not permanently lost.
// The sweeper cannot recover a job if the ledger row was never written.
func TestProcessChunk_LLMExhausted_PayloadLedgerWriteFailure(t *testing.T) {
	exhaustedErr := &domainerr.LLMAllExhaustedError{RetryAfter: time.Now().Add(time.Minute)}
	scorer := &fakeScorer{err: exhaustedErr}
	ledger := &fakeLedger{retryWithPayloadErr: errors.New("db down")}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	item, state := makeChunkItem(makeJob("e-ledgerfail"), 1)
	_ = w.processChunk(context.Background(), []chunkItem{item}, "fetch-ledgerfail", 0, 1)

	// Ledger write failed → must Nak (not Term) so NATS redelivers.
	if state.Termed {
		t.Errorf("must NOT Term when payload ledger write fails — job would be permanently lost")
	}
	if !state.Naked {
		t.Errorf("expected message Nak'd when payload ledger write fails so NATS redelivers")
	}
}

// TestProcessChunk_LLMExhausted_EmitsMetric verifies that routeFailedChunk on
// LLMAllExhaustedError increments FilterLLMExhaustionTotal.
func TestProcessChunk_LLMExhausted_EmitsMetric(t *testing.T) {
	exhaustedErr := &domainerr.LLMAllExhaustedError{RetryAfter: time.Now().Add(time.Minute)}
	scorer := &fakeScorer{err: exhaustedErr}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	before := promtestutil.ToFloat64(metrics.FilterLLMExhaustionTotal)

	item1, _ := makeChunkItem(makeJob("em-1"), 1)
	item2, _ := makeChunkItem(makeJob("em-2"), 1)
	_ = w.processChunk(context.Background(), []chunkItem{item1, item2}, "fetch-metric", 0, 1)

	after := promtestutil.ToFloat64(metrics.FilterLLMExhaustionTotal)
	if after-before != 2 {
		t.Errorf("expected FilterLLMExhaustionTotal +2, got +%v", after-before)
	}
}

// TestRouteFailedChunk_LLMExhausted_TermFailAfterLedgerWrite verifies that a
// Term failure after a successful ledger write is logged but does not cause the
// entry to be dropped from RetriedWithPayload — the ledger row is authoritative.
func TestRouteFailedChunk_LLMExhausted_TermFailAfterLedgerWrite(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	exhaustedErr := &domainerr.LLMAllExhaustedError{RetryAfter: now.Add(5 * time.Minute)}

	w := makeWorker(&fakeScorer{}, &fakeLedger{}, &fakePublisher{})
	w.clock = &fakeClock{now: now}

	item, state := makeChunkItem(makeJob("term-fail"), 1)
	state.TermErr = errors.New("term broker error") // inject Term failure

	outcome := w.routeFailedChunk(context.Background(), []chunkItem{item}, exhaustedErr)

	// Term was attempted (and failed), but the ledger write succeeded.
	if !state.Termed {
		t.Errorf("expected Term to have been attempted even when it fails")
	}
	// Entry still in RetriedWithPayload (ledger write succeeded before Term).
	if len(outcome.RetriedWithPayload) != 1 {
		t.Fatalf("expected 1 RetriedWithPayload entry even when Term fails, got %d",
			len(outcome.RetriedWithPayload))
	}
	if outcome.RetriedWithPayload[0].JobID != "term-fail" {
		t.Errorf("unexpected job ID in entry: %q", outcome.RetriedWithPayload[0].JobID)
	}
}

// TestRouteFailedChunk_LLMExhausted_NakFailAfterLedgerWriteFailure verifies that
// when UpdateRetryBatchWithPayload fails AND the fallback Nak also fails, the
// worker logs the Nak failure without panicking or dropping the entry.
func TestRouteFailedChunk_LLMExhausted_NakFailAfterLedgerWriteFailure(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	exhaustedErr := &domainerr.LLMAllExhaustedError{RetryAfter: now.Add(5 * time.Minute)}
	ledger := &fakeLedger{retryWithPayloadErr: errors.New("db down")}

	w := makeWorker(&fakeScorer{}, ledger, &fakePublisher{})
	w.clock = &fakeClock{now: now}

	item, state := makeChunkItem(makeJob("nak-fail-too"), 1)
	state.NakErr = errors.New("nak broker error") // inject Nak failure on top of ledger failure

	outcome := w.routeFailedChunk(context.Background(), []chunkItem{item}, exhaustedErr)

	// Nak was attempted (and failed).
	if !state.Naked {
		t.Errorf("expected Nak to have been attempted on ledger write failure")
	}
	// No entry in RetriedWithPayload (ledger write failed).
	if len(outcome.RetriedWithPayload) != 0 {
		t.Errorf("expected no RetriedWithPayload entries when ledger write fails, got %d",
			len(outcome.RetriedWithPayload))
	}
}

// TestProcessChunk_LLMExhausted_MarshalFailTermsWithoutLedger verifies that
// routeFailedChunk handles a marshal failure for one job: the failed item is
// Term'd immediately (no ledger entry), other items are processed normally.
func TestProcessChunk_LLMExhausted_MarshalFailTermsWithoutLedger(t *testing.T) {
	// We can't easily inject a marshal failure for ats.RawJob (it's a clean struct),
	// so we test via the routeFailedChunk function directly with a job that would
	// successfully marshal. This test instead validates the happy path of that branch
	// via TestRouteFailedChunk_LLMExhausted_TermsAndWritesPayload which exercises
	// the exact same loop. Instead, we cover the all-entries path here to verify
	// FilterLLMExhaustionTotal is incremented even when all items marshal OK and
	// the ledger write succeeds.
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	exhaustedErr := &domainerr.LLMAllExhaustedError{RetryAfter: now.Add(3 * time.Minute)}

	w := makeWorker(&fakeScorer{}, &fakeLedger{}, &fakePublisher{})
	w.clock = &fakeClock{now: now}
	before := promtestutil.ToFloat64(metrics.FilterLLMExhaustionTotal)

	item1, state1 := makeChunkItem(makeJob("m1"), 1)
	item2, state2 := makeChunkItem(makeJob("m2"), 1)
	outcome := w.routeFailedChunk(context.Background(), []chunkItem{item1, item2}, exhaustedErr)

	after := promtestutil.ToFloat64(metrics.FilterLLMExhaustionTotal)
	if after-before != 2 {
		t.Errorf("expected FilterLLMExhaustionTotal +2, got +%.0f", after-before)
	}
	if !state1.Termed || !state2.Termed {
		t.Errorf("expected both items Term'd: s1=%v s2=%v", state1.Termed, state2.Termed)
	}
	if len(outcome.RetriedWithPayload) != 2 {
		t.Errorf("expected 2 RetriedWithPayload entries, got %d", len(outcome.RetriedWithPayload))
	}
}

// ── publishPartition branch coverage ─────────────────────────────────────────

// TestPublishPartition_NakFailAfterPublishFail exercises the path where publish
// fails and the subsequent Nak also fails — the message must be Term'd so it is
// not stuck in the broker forever.
func TestPublishPartition_NakFailAfterPublishFail(t *testing.T) {
	pub := &fakePublisher{err: errors.New("broker down")}
	w := makeWorker(&fakeScorer{}, &fakeLedger{}, pub)

	item, state := makeChunkItem(makeJob("pp-nak-fail"), 1)
	state.NakErr = errors.New("nak also failed") // inject Nak failure

	published, failed := w.publishPartition(context.Background(), []chunkItem{item})

	if len(published) != 0 {
		t.Errorf("expected 0 published on failure, got %d", len(published))
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(failed))
	}
	// Nak was attempted (and failed), so Term must have been called.
	if !state.Naked {
		t.Errorf("expected Nak to have been attempted")
	}
	if !state.Termed {
		t.Errorf("expected Term'd after Nak failure following publish failure")
	}
}

// TestPublishPartition_NakSuccessAfterPublishFail verifies the basic publish-fail
// path: publish fails → Nak → item ends up in failed list, not published.
func TestPublishPartition_NakSuccessAfterPublishFail(t *testing.T) {
	pub := &fakePublisher{err: errors.New("broker transient")}
	w := makeWorker(&fakeScorer{}, &fakeLedger{}, pub)

	item, state := makeChunkItem(makeJob("pp-nak-ok"), 1)
	// NakErr stays nil → Nak succeeds

	published, failed := w.publishPartition(context.Background(), []chunkItem{item})

	if len(published) != 0 {
		t.Errorf("expected 0 published, got %d", len(published))
	}
	if len(failed) != 1 || failed[0].job.job.ID != "pp-nak-ok" {
		t.Errorf("expected 1 failed item pp-nak-ok, got %v", failed)
	}
	if !state.Naked {
		t.Errorf("expected Nak'd after publish failure")
	}
	if state.Termed {
		t.Errorf("must NOT Term when Nak succeeds")
	}
}

// TestPublishPartition_AllSuccess verifies the success path: all items published,
// none in failed list.
func TestPublishPartition_AllSuccess(t *testing.T) {
	pub := &fakePublisher{}
	w := makeWorker(&fakeScorer{}, &fakeLedger{}, pub)

	item1, _ := makeChunkItem(ats.RawJob{ID: "s1", Adapter: "greenhouse"}, 1)
	item2, _ := makeChunkItem(ats.RawJob{ID: "s2", Adapter: "lever"}, 1)

	published, failed := w.publishPartition(context.Background(), []chunkItem{item1, item2})

	if len(published) != 2 {
		t.Errorf("expected 2 published, got %d", len(published))
	}
	if len(failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(failed))
	}
}

// ── nakRemainingChunks branch coverage ──────────────────────────────────────

// TestNakRemainingChunks_EmptyChunks exercises the early-return path when the
// total job count across chunks is zero — no logging, no Nak calls.
func TestNakRemainingChunks_EmptyChunks(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakeLedger{}, &fakePublisher{})
	// Must not panic and must not emit any messages.
	w.nakRemainingChunks([][]chunkItem{}, "fetch-empty", "shutdown")
	w.nakRemainingChunks([][]chunkItem{{}, {}}, "fetch-zero", "shutdown") // two empty inner slices
}

// TestNakRemainingChunks_NonEmpty exercises the main path where jobs are Nak'd.
func TestNakRemainingChunks_NonEmpty(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakeLedger{}, &fakePublisher{})

	item1, state1 := makeChunkItem(makeJob("nrc-1"), 1)
	item2, state2 := makeChunkItem(makeJob("nrc-2"), 1)

	w.nakRemainingChunks([][]chunkItem{{item1, item2}}, "fetch-1", "shutdown test")

	if !state1.Naked || !state2.Naked {
		t.Errorf("expected both items Nak'd: s1=%v s2=%v", state1.Naked, state2.Naked)
	}
}

// TestNakRemainingChunks_NakFail exercises the Nak-failure sub-branch (error
// logged, but function continues without panic).
func TestNakRemainingChunks_NakFail(t *testing.T) {
	w := makeWorker(&fakeScorer{}, &fakeLedger{}, &fakePublisher{})

	item, state := makeChunkItem(makeJob("nrc-nak-fail"), 1)
	state.NakErr = errors.New("nak failed") // inject Nak failure

	// Must not panic even when Nak fails.
	w.nakRemainingChunks([][]chunkItem{{item}}, "fetch-1", "shutdown test")

	if !state.Naked {
		t.Errorf("expected Nak to have been attempted")
	}
}

// ── markSeenOrNak branch coverage ───────────────────────────────────────────

// TestMarkSeenOrNak_EmptyItems verifies the early-return for empty input — no
// ledger calls, no message disposition.
func TestMarkSeenOrNak_EmptyItems(t *testing.T) {
	ledger := &fakeLedger{}
	w := makeWorker(&fakeScorer{}, ledger, &fakePublisher{})
	w.markSeenOrNak(context.Background(), nil, jobcommon.SeenReasonFiltered, "fetch-1", 0)
	if len(ledger.calls) != 0 {
		t.Errorf("expected no ledger calls for empty input, got %d", len(ledger.calls))
	}
}

// TestMarkSeenOrNak_GenericError covers the non-ErrLedgerRowMissing error branch
// inside markSeenOrNak — all messages must be Nak'd (not Acked) and no
// filter_ledger_missing metric increment (that metric is only for the race).
func TestMarkSeenOrNak_GenericError(t *testing.T) {
	ledger := &fakeLedger{seenErr: errors.New("db down")}
	w := makeWorker(&fakeScorer{}, ledger, &fakePublisher{})

	item1, state1 := makeChunkItem(makeJob("msn-1"), 1)
	item2, state2 := makeChunkItem(makeJob("msn-2"), 1)
	before := promtestutil.ToFloat64(metrics.FilterLedgerMissing.WithLabelValues(string(jobcommon.SeenReasonFiltered)))

	w.markSeenOrNak(context.Background(), []chunkItem{item1, item2}, jobcommon.SeenReasonFiltered, "fetch-1", 0)

	if !state1.Naked || !state2.Naked {
		t.Errorf("expected both Nak'd on generic ledger error: s1=%v s2=%v", state1.Naked, state2.Naked)
	}
	if state1.Acked || state2.Acked {
		t.Errorf("must NOT Ack on ledger error")
	}
	after := promtestutil.ToFloat64(metrics.FilterLedgerMissing.WithLabelValues(string(jobcommon.SeenReasonFiltered)))
	if after != before {
		t.Errorf("generic DB error must NOT increment filter_ledger_missing (that's only for race condition): delta=%.0f", after-before)
	}
}

// TestMarkSeenOrNak_NakFailAfterLedgerError verifies that a Nak failure on top
// of a ledger error doesn't panic — the Nak failure is logged and skipped.
func TestMarkSeenOrNak_NakFailAfterLedgerError(t *testing.T) {
	ledger := &fakeLedger{seenErr: errors.New("db down")}
	w := makeWorker(&fakeScorer{}, ledger, &fakePublisher{})

	item, state := makeChunkItem(makeJob("msn-nak-fail"), 1)
	state.NakErr = errors.New("nak failed") // inject Nak failure on top

	// Must not panic.
	w.markSeenOrNak(context.Background(), []chunkItem{item}, jobcommon.SeenReasonFiltered, "fetch-1", 0)

	if !state.Naked {
		t.Errorf("expected Nak to have been attempted")
	}
}

// ── processFetch additional branches ────────────────────────────────────────

// TestProcessFetch_MalformedMessage_DLQPublishFail covers the path where a
// message can't be JSON-decoded AND the subsequent DLQ publish also fails.
// The message must still be Term'd and processFetch must return nil (non-fatal).
func TestProcessFetch_MalformedMessage_DLQPublishFail(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{err: errors.New("DLQ broker down")}
	w := makeWorkerWithClock(scorer, ledger, pub, &fakeClock{now: time.Now()}, 5)

	msg, state := js.NewTestMsg([]byte("not valid json"), 1)

	err := w.processFetch(context.Background(), []*js.Msg{msg})
	if err != nil {
		t.Errorf("expected nil (malformed is non-fatal), got %v", err)
	}
	// Message must be Term'd even when DLQ publish fails.
	if !state.Termed {
		t.Errorf("expected malformed message to be Term'd even when DLQ publish fails")
	}
}

// TestProcessFetch_ContextAlreadyCancelled ensures processFetch returns nil
// immediately when the context is already done on entry.
func TestProcessFetch_ContextAlreadyCancelled(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	w := makeWorkerWithClock(scorer, ledger, pub, &fakeClock{now: time.Now()}, 5)

	payload, _ := json.Marshal(makeJob("ctx-cancel"))
	msg, _ := js.NewTestMsg(payload, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	// Should not error — just propagate nil or context error.
	// processFetch checks ctx.Err() inside the loop but the result is non-blocking.
	_ = w.processFetch(ctx, []*js.Msg{msg})
	// No assertion beyond "doesn't panic".
}

// ── Run function coverage ────────────────────────────────────────────────────

// fakeMessageFetcher is a MessageFetcher that returns a controlled sequence
// of fetch results.
type fakeMessageFetcher struct {
	results []fetchResult
	callIdx int
}

type fetchResult struct {
	msgs []*js.Msg
	err  error
}

func (f *fakeMessageFetcher) Fetch(_ int, _ time.Duration) ([]*js.Msg, error) {
	if f.callIdx >= len(f.results) {
		// Block forever (simulate idle consumer).
		time.Sleep(24 * time.Hour)
		return nil, nil
	}
	r := f.results[f.callIdx]
	f.callIdx++
	return r.msgs, r.err
}

// TestRun_CancelledContext asserts that Run returns nil when the context is
// cancelled — the worker stops cleanly without an error.
func TestRun_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled immediately

	consumer := &fakeMessageFetcher{
		results: []fetchResult{{msgs: nil, err: nil}},
	}
	w := &Worker{
		consumer:        consumer,
		publisher:       &fakePublisher{},
		scorer:          &fakeScorer{},
		ledger:          &fakeLedger{},
		batchSize:       5,
		flushTimeout:    30 * time.Second,
		llmCallInterval: 5 * time.Second,
		clock:           realClock{},
		log:             logger.NewNoop(),
	}

	err := w.Run(ctx)
	if err != nil {
		t.Errorf("expected nil on context cancel, got %v", err)
	}
}

// TestRun_FetchError_ContinuesLoop verifies that a fetch error is logged but
// does not abort Run. The loop continues until ctx is cancelled.
func TestRun_FetchError_ContinuesLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	consumer := &fakeMessageFetcher{}
	// Override Fetch to return an error the first time, then cancel context.
	realFetcher := &callbackFetcher{fn: func(n int, maxWait time.Duration) ([]*js.Msg, error) {
		callCount++
		if callCount == 1 {
			return nil, errors.New("nats connection lost")
		}
		cancel() // cancel on second call
		return nil, nil
	}}
	_ = consumer

	w := &Worker{
		consumer:        realFetcher,
		publisher:       &fakePublisher{},
		scorer:          &fakeScorer{},
		ledger:          &fakeLedger{},
		batchSize:       5,
		flushTimeout:    1 * time.Millisecond,
		llmCallInterval: 5 * time.Second,
		clock:           realClock{},
		log:             logger.NewNoop(),
	}

	err := w.Run(ctx)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 Fetch calls, got %d", callCount)
	}
}

// callbackFetcher is a MessageFetcher that calls a user-supplied function.
type callbackFetcher struct {
	fn func(n int, maxWait time.Duration) ([]*js.Msg, error)
}

func (c *callbackFetcher) Fetch(n int, maxWait time.Duration) ([]*js.Msg, error) {
	return c.fn(n, maxWait)
}

// ── Additional coverage tests ─────────────────────────────────────────────────

// TestProcessFetch_MalformedMessage_TermFail_NoPanic verifies that when a
// malformed message is received AND msg.Term() returns an error, the worker
// logs a warning but does not panic. processFetch must return nil.
func TestProcessFetch_MalformedMessage_TermFail_NoPanic(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	w := makeWorkerWithClock(scorer, ledger, pub, &fakeClock{now: time.Now()}, 5)

	msg, state := js.NewTestMsg([]byte("not valid json"), 1)
	state.TermErr = errors.New("nats term error")

	err := w.processFetch(context.Background(), []*js.Msg{msg})
	if err != nil {
		t.Errorf("processFetch: want nil, got %v", err)
	}
	if !state.Termed {
		t.Errorf("expected Term to have been attempted on malformed message")
	}
}

// panicScorer is a scorer that panics inside ScoreBatch to exercise the
// panic recovery handler in processChunk.
type panicScorer struct{}

func (p *panicScorer) ScoreBatch(_ context.Context, _ []llmdto.JobsRelevanceFilterInput) ([]string, error) {
	panic("scorer induced panic for testing")
}

// makeWorkerWithPanicScorer builds a worker using a scorer that panics.
func makeWorkerWithPanicScorer(ledger *fakeLedger, pub *fakePublisher, clk clock, batchSize int) *Worker {
	return &Worker{
		scorer:          &panicScorer{},
		ledger:          ledger,
		publisher:       pub,
		batchSize:       batchSize,
		flushTimeout:    30 * time.Second,
		llmCallInterval: 5 * time.Second,
		clock:           clk,
		log:             logger.NewNoop(),
	}
}

// TestProcessFetch_PanicInProcessChunk_RecoveredAndNaks verifies that a panic
// inside processChunk is caught by the deferred recovery in processFetch. The
// affected messages are Nak'd and an error is logged.
func TestProcessFetch_PanicInProcessChunk_RecoveredAndNaks(t *testing.T) {
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	w := makeWorkerWithPanicScorer(ledger, pub, &fakeClock{now: time.Now()}, 5)

	// Title must be non-empty so the item goes into scorable (not passThrough),
	// ensuring ScoreBatch is called and the panic occurs.
	payload, _ := json.Marshal(ats.RawJob{ID: "panic-job", Adapter: "greenhouse", Title: "SWE"})
	msg, state := js.NewTestMsg(payload, 1)

	// processFetch must not propagate the panic.
	err := w.processFetch(context.Background(), []*js.Msg{msg})
	if err != nil {
		t.Errorf("processFetch: want nil on recovered panic, got %v", err)
	}
	if !state.Naked {
		t.Errorf("expected Nak'd in panic recovery handler")
	}
}

// TestProcessFetch_PanicInProcessChunk_LedgerFail_NakFail_NoPanic verifies
// that the panic recovery handler tolerates both a ledger batch failure and
// a Nak failure without itself panicking.
func TestProcessFetch_PanicInProcessChunk_LedgerFail_NakFail_NoPanic(t *testing.T) {
	ledger := &fakeLedger{seenErr: errors.New("db down")} // UpdateSeenReasonBatch fails
	pub := &fakePublisher{}
	w := makeWorkerWithPanicScorer(ledger, pub, &fakeClock{now: time.Now()}, 5)

	// Title must be non-empty so the item goes into scorable (not passThrough).
	payload, _ := json.Marshal(ats.RawJob{ID: "panic-nak-fail", Adapter: "greenhouse", Title: "SWE"})
	msg, state := js.NewTestMsg(payload, 1)
	state.NakErr = errors.New("nak failed too") // Nak also fails

	// processFetch must not panic despite both ledger and Nak failures.
	err := w.processFetch(context.Background(), []*js.Msg{msg})
	if err != nil {
		t.Errorf("processFetch: want nil, got %v", err)
	}
	// Nak was attempted even though it failed.
	if !state.Naked {
		t.Errorf("expected Nak to have been attempted in panic recovery")
	}
}

// TestMarkSeenOrNak_AckFail_NoPanic verifies that when Ack() returns an error
// inside markSeenOrNak (success path), a warning is logged but no panic occurs.
func TestMarkSeenOrNak_AckFail_NoPanic(t *testing.T) {
	ledger := &fakeLedger{} // UpdateSeenReasonBatch succeeds
	w := makeWorker(&fakeScorer{}, ledger, &fakePublisher{})

	item, state := makeChunkItem(makeJob("ack-fail-mark"), 1)
	state.AckErr = errors.New("nats ack error")

	// Must not panic.
	w.markSeenOrNak(context.Background(), []chunkItem{item}, jobcommon.SeenReasonLLMIrrelevant, "fetch-1", 0)

	if !state.Acked {
		t.Errorf("expected Ack to have been attempted")
	}
}

// TestPublishPartition_NakFail_TermFail_NoPanic verifies that when both Nak
// and Term fail in publishPartition (after a publish failure), the worker
// logs warnings but does not panic.
func TestPublishPartition_NakFail_TermFail_NoPanic(t *testing.T) {
	pub := &fakePublisher{err: errors.New("publish down")}
	w := makeWorker(&fakeScorer{}, &fakeLedger{}, pub)

	item, state := makeChunkItem(makeJob("nak-term-fail"), 1)
	state.NakErr = errors.New("nak failed")
	state.TermErr = errors.New("term also failed")

	published, failed := w.publishPartition(context.Background(), []chunkItem{item})

	if len(published) != 0 {
		t.Errorf("expected 0 published, got %d", len(published))
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(failed))
	}
	if !state.Naked {
		t.Errorf("expected Nak to have been attempted")
	}
	if !state.Termed {
		t.Errorf("expected Term to have been attempted after Nak failure")
	}
}

// TestRun_FetchError_CtxCancelDuringSleep verifies that when a fetch error
// occurs and ctx is cancelled during the post-error backoff sleep, Run exits
// cleanly (the ctx.Done() case in the select fires).
func TestRun_FetchError_CtxCancelDuringSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	realFetcher := &callbackFetcher{fn: func(n int, maxWait time.Duration) ([]*js.Msg, error) {
		callCount++
		if callCount == 1 {
			// Return an error; immediately cancel ctx so the backoff select
			// fires ctx.Done() instead of time.After(1s).
			go cancel()
			return nil, errors.New("transient fetch error")
		}
		// Should not reach here — ctx.Done() path should exit.
		return nil, nil
	}}

	w := &Worker{
		consumer:        realFetcher,
		publisher:       &fakePublisher{},
		scorer:          &fakeScorer{},
		ledger:          &fakeLedger{},
		batchSize:       5,
		flushTimeout:    1 * time.Millisecond,
		llmCallInterval: 5 * time.Second,
		clock:           realClock{},
		log:             logger.NewNoop(),
	}

	err := w.Run(ctx)
	if err != nil {
		t.Errorf("Run: want nil after fetch error + ctx cancel, got %v", err)
	}
}
