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
	reason string
}

type fakeLedger struct {
	calls        []ledgerCall
	retryBatches [][]jobsvc.RetryLedgerEntry
	retryErr     error
	// seenErr, when set, is returned by UpdateSeenReasonBatch (used to inject
	// ErrLedgerRowMissing). seenErrForReason takes precedence when its key matches.
	seenErr          error
	seenErrForReason map[string]error
}

func (f *fakeLedger) UpdateSeenReasonBatch(_ context.Context, ids []string, reason string) error {
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
	before := promtestutil.ToFloat64(metrics.FilterLedgerMissing.WithLabelValues(jobcommon.SeenReasonFiltered))
	_ = w.processChunk(context.Background(), []chunkItem{item1, item2}, "fetch-missing", 0, 1)
	after := promtestutil.ToFloat64(metrics.FilterLedgerMissing.WithLabelValues(jobcommon.SeenReasonFiltered))

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
		if !e.NextAttemptAt.Equal(now.Add(time.Minute)) {
			t.Errorf("expected next_attempt_at=%v, got %v", now.Add(time.Minute), e.NextAttemptAt)
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
		if !e.NextAttemptAt.Equal(now.Add(time.Minute)) {
			t.Errorf("expected next_attempt_at=%v, got %v", now.Add(time.Minute), e.NextAttemptAt)
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

// deliveryCount=2 → retryCount=1, next tier is the 10m BackOff slot.
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
	want := now.Add(10 * time.Minute)
	if !ledger.retryBatches[0][0].NextAttemptAt.Equal(want) {
		t.Errorf("expected next_attempt_at=%v (10m tier), got %v", want, ledger.retryBatches[0][0].NextAttemptAt)
	}
}
