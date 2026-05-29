package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
)

// ── pure-helper tests ───────────────────────────────────────────────────────

func TestFormatSlotStatuses_Empty(t *testing.T) {
	if got := formatSlotStatuses(nil); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if got := formatSlotStatuses([]domainerr.LLMSlotStatus{}); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestFormatSlotStatuses_WithCooldown(t *testing.T) {
	cd := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	statuses := []domainerr.LLMSlotStatus{
		{SlotID: "slotA", LastDenial: "rpm", FailureCount: 2, CooldownUntil: cd},
		{SlotID: "slotB", LastDenial: "tpm", FailureCount: 0}, // no cooldown
	}
	got := formatSlotStatuses(statuses)
	wantA := "slotA=rpm(fail=2,cd=2026-04-22T12:00:00Z)"
	wantB := "slotB=tpm(fail=0,cd=-)"
	if got != wantA+"; "+wantB {
		t.Errorf("unexpected format: %q", got)
	}
}

func TestSplitBySize_Empty(t *testing.T) {
	chunks := splitBySize(nil, 5)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestSplitBySize_SizeBelowOne(t *testing.T) {
	items := makeBatch(makeJob("a"), makeJob("b"))
	// size<1 is normalised to 1.
	chunks := splitBySize(items, 0)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks for size=0, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) != 1 || c[0].job.job.ID != items[i].job.job.ID {
			t.Errorf("chunk %d preserves order/size: got %+v", i, c)
		}
	}
}

func TestSplitBySize_SizeLargerThanInput(t *testing.T) {
	items := makeBatch(makeJob("a"), makeJob("b"))
	chunks := splitBySize(items, 10)
	if len(chunks) != 1 || len(chunks[0]) != 2 {
		t.Fatalf("expected 1 chunk of 2, got %+v", chunks)
	}
}

func TestSplitBySize_ExactMultiple(t *testing.T) {
	items := makeBatch(makeJob("a"), makeJob("b"), makeJob("c"), makeJob("d"))
	chunks := splitBySize(items, 2)
	if len(chunks) != 2 || len(chunks[0]) != 2 || len(chunks[1]) != 2 {
		t.Fatalf("expected 2 chunks of 2, got %+v", chunks)
	}
}

// ── realClock tests ─────────────────────────────────────────────────────────

func TestRealClock_Now(t *testing.T) {
	rc := realClock{}
	now := rc.Now()
	if now.IsZero() {
		t.Errorf("expected non-zero time")
	}
}

func TestRealClock_Sleep_NegativeDuration(t *testing.T) {
	rc := realClock{}
	start := time.Now()
	if err := rc.Sleep(context.Background(), -1*time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Errorf("Sleep with negative duration should return immediately")
	}
}

func TestRealClock_Sleep_ZeroDuration(t *testing.T) {
	rc := realClock{}
	if err := rc.Sleep(context.Background(), 0); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestRealClock_Sleep_PositiveDuration(t *testing.T) {
	rc := realClock{}
	start := time.Now()
	if err := rc.Sleep(context.Background(), 5*time.Millisecond); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 5*time.Millisecond {
		t.Errorf("Sleep returned before requested duration: %v", elapsed)
	}
}

func TestRealClock_Sleep_CtxCancel(t *testing.T) {
	rc := realClock{}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	err := rc.Sleep(ctx, 1*time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// ── NewWorker constructor ───────────────────────────────────────────────────

func TestNewWorker_FieldsWired(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	log := logger.NewNoop()

	w := NewWorker(nil, pub, scorer, ledger, 7, 30*time.Second, 3*time.Second, log)
	if w == nil {
		t.Fatal("expected non-nil worker")
	}
	if w.batchSize != 7 {
		t.Errorf("batchSize=%d, want 7", w.batchSize)
	}
	if w.llmCallInterval != 3*time.Second {
		t.Errorf("llmCallInterval=%v, want 3s", w.llmCallInterval)
	}
	if w.flushTimeout != 30*time.Second {
		t.Errorf("flushTimeout=%v, want 30s", w.flushTimeout)
	}
	if w.scorer != scorer || w.ledger != ledger || w.publisher != pub {
		t.Errorf("constructor wired wrong dependency")
	}
	if _, ok := w.clock.(realClock); !ok {
		t.Errorf("default clock should be realClock, got %T", w.clock)
	}
}

// ── pass-through (no-title) tests ───────────────────────────────────────────

func TestProcessChunk_PassThrough_NoTitle(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	item1, _ := makeChunkItem(ats.RawJob{ID: "no-title-1", Title: ""}, 1)
	item2, _ := makeChunkItem(ats.RawJob{ID: "no-title-2", Title: ""}, 1)
	chunk := []chunkItem{item1, item2}
	_ = w.processChunk(context.Background(), chunk, "fetch-1", 0, 1)

	if scorer.calls != 0 {
		t.Errorf("scorer should not be called for titleless chunk, got %d calls", scorer.calls)
	}
	filtered := pub.recordsOnSubject("jobs.filtered.")
	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered publishes, got %d", len(filtered))
	}
	if len(ledger.calls) != 1 {
		t.Fatalf("expected 1 ledger call (filtered) for pass-through, got %d", len(ledger.calls))
	}
	if ledger.calls[0].reason != jobcommon.SeenReasonFiltered {
		t.Errorf("expected reason %q, got %q", jobcommon.SeenReasonFiltered, ledger.calls[0].reason)
	}
	if len(ledger.calls[0].ids) != 2 {
		t.Errorf("expected 2 filtered IDs, got %v", ledger.calls[0].ids)
	}
}

func TestProcessChunk_PassThroughMixed(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"titled-1"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	item1, _ := makeChunkItem(ats.RawJob{ID: "no-title-x", Title: ""}, 1)
	item2, _ := makeChunkItem(ats.RawJob{ID: "titled-1", Title: "Engineer"}, 1)
	chunk := []chunkItem{item1, item2}
	_ = w.processChunk(context.Background(), chunk, "fetch-1", 0, 1)

	if scorer.calls != 1 {
		t.Errorf("expected scorer called once (for titled job), got %d", scorer.calls)
	}
	// Both pass-through and titled-relevant land on the filtered subject.
	filtered := pub.recordsOnSubject("jobs.filtered.")
	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered publishes, got %d", len(filtered))
	}
}

// ── LLMAllExhaustedError logging path ───────────────────────────────────────

func TestProcessChunk_LLMExhaustedLogsAndRetries(t *testing.T) {
	retryAfter := time.Date(2026, 4, 22, 13, 0, 0, 0, time.UTC)
	cd := time.Date(2026, 4, 22, 12, 30, 0, 0, time.UTC)
	scoreErr := &domainerr.LLMAllExhaustedError{
		RetryAfter: retryAfter,
		Slots: []domainerr.LLMSlotStatus{
			{SlotID: "slot1", LastDenial: "rpm", FailureCount: 3, CooldownUntil: cd},
		},
	}
	scorer := &fakeScorer{err: scoreErr}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	w := makeWorker(scorer, ledger, pub)
	w.clock = &fakeClock{now: now}

	item, state := makeChunkItem(makeJob("job-1"), 1)
	_ = w.processChunk(context.Background(), []chunkItem{item}, "fetch-1", 0, 1)

	// Exhausted is treated as a transient error → retry via Nak.
	if !state.Naked {
		t.Errorf("expected job Nak'd on LLM exhausted error")
	}
	if len(ledger.retryBatches) != 1 || ledger.retryBatches[0][0].RetryCount != 1 {
		t.Errorf("expected retry ledger entry retry_count=1, got %+v", ledger.retryBatches)
	}
}

func TestProcessChunk_LLMExhaustedZeroRetryAfter(t *testing.T) {
	// Zero RetryAfter time skips the retry_after log fields but path still works.
	scoreErr := &domainerr.LLMAllExhaustedError{}
	scorer := &fakeScorer{err: scoreErr}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.clock = &fakeClock{now: time.Now()}

	item, state := makeChunkItem(makeJob("job-1"), 1)
	_ = w.processChunk(context.Background(), []chunkItem{item}, "fetch-1", 0, 1)

	if !state.Naked {
		t.Errorf("expected job Nak'd")
	}
	if len(ledger.retryBatches) != 1 {
		t.Errorf("expected retry ledger entry")
	}
}

// ── publishToDLQ / publishDetail failure paths ──────────────────────────────

// failingPublisher returns errors for subjects matching failSubject.
type failingPublisher struct {
	failSubject string
	failAll     bool
	records     []publishedRecord
}

func (f *failingPublisher) Publish(_ context.Context, subject string, value []byte) error {
	if f.failAll || strings.Contains(subject, f.failSubject) {
		return errors.New("publish boom")
	}
	f.records = append(f.records, publishedRecord{subject: subject, value: value})
	return nil
}

func (f *failingPublisher) PublishWithMsgID(_ context.Context, subject string, value []byte, _ string) error {
	return f.Publish(context.Background(), subject, value)
}

func (f *failingPublisher) recordsOnSubject(needle string) []publishedRecord {
	var out []publishedRecord
	for _, r := range f.records {
		if strings.Contains(r.subject, needle) {
			out = append(out, r)
		}
	}
	return out
}

func TestProcessChunk_DLQPublishFailure(t *testing.T) {
	// Exhausted retry path → DLQ. Publisher fails for DLQ subject.
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &failingPublisher{failSubject: js.SubjectJobsDLQ}

	w := &Worker{
		scorer:    scorer,
		ledger:    ledger,
		publisher: pub,
		batchSize: 5,
		clock:     &fakeClock{now: time.Now()},
		log:       logger.NewNoop(),
	}
	_ = w.processChunk(context.Background(), makeBatchWithRetryCount(3, makeJob("job-1")), "fetch-1", 0, 1)

	// DLQ publish failed → job not in outcome.DLQed; UpdateSeenReasonBatch must not be invoked.
	for _, c := range ledger.calls {
		if c.reason == jobcommon.SeenReasonError && len(c.ids) > 0 {
			t.Errorf("expected no error ledger call when DLQ publish failed, got %+v", c)
		}
	}
	if len(pub.recordsOnSubject(js.SubjectJobsDLQ)) != 0 {
		t.Errorf("DLQ publish should have failed")
	}
}

func TestProcessChunk_PublishDetailFailureLogged(t *testing.T) {
	// All-relevant scorer with publisher failing on filtered subject.
	// Should log but not panic.
	scorer := &fakeScorer{relevantIDs: []string{"job-1"}}
	ledger := &fakeLedger{}
	pub := &failingPublisher{failSubject: "jobs.filtered."}

	w := &Worker{
		scorer:    scorer,
		ledger:    ledger,
		publisher: pub,
		batchSize: 5,
		clock:     &fakeClock{now: time.Now()},
		log:       logger.NewNoop(),
	}
	_ = w.processChunk(context.Background(), makeBatch(makeJob("job-1")), "fetch-1", 0, 1)

	// Despite the publish failure, no irrelevant ledger call should be made for a relevant job.
	for _, c := range ledger.calls {
		if c.reason == jobcommon.SeenReasonLLMIrrelevant {
			t.Errorf("relevant job should not be marked irrelevant on publish failure")
		}
	}
}

func TestProcessChunk_PassThroughPublishFailure(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &failingPublisher{failSubject: "jobs.filtered."}

	w := &Worker{
		scorer:    scorer,
		ledger:    ledger,
		publisher: pub,
		batchSize: 5,
		clock:     &fakeClock{now: time.Now()},
		log:       logger.NewNoop(),
	}
	item, _ := makeChunkItem(ats.RawJob{ID: "no-title", Title: ""}, 1)
	_ = w.processChunk(context.Background(), []chunkItem{item}, "fetch-1", 0, 1)
	// No assertion beyond "didn't panic".
}

func TestProcessChunk_AllIrrelevant(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: nil} // empty → all irrelevant
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	_ = w.processChunk(context.Background(), makeBatch(makeJob("a"), makeJob("b")), "f", 0, 1)

	if len(pub.recordsOnSubject("jobs.filtered.")) != 0 {
		t.Errorf("expected no filtered publishes, got some")
	}
	if len(ledger.calls) != 1 || ledger.calls[0].reason != jobcommon.SeenReasonLLMIrrelevant {
		t.Fatalf("expected 1 irrelevant ledger call, got %+v", ledger.calls)
	}
	if len(ledger.calls[0].ids) != 2 {
		t.Errorf("expected 2 irrelevant ids, got %d", len(ledger.calls[0].ids))
	}
}

// ── panic recovery ──────────────────────────────────────────────────────────

// panickyScorer panics on ScoreBatch.
type panickyScorer struct{}

func (panickyScorer) ScoreBatch(_ context.Context, _ []llmdto.JobsRelevanceFilterInput) ([]string, error) {
	panic("boom")
}

func TestProcessFetch_PanicRecoveredAndMarkedError(t *testing.T) {
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(&fakeScorer{}, ledger, pub, clk, 5)
	w.scorer = panickyScorer{}

	payload, _ := json.Marshal(makeJob("job-1"))
	msg, state := js.NewTestMsg(payload, 1)

	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch returned err: %v", err)
	}

	// Panic recovery should have marked the chunk's IDs as error.
	foundErr := false
	for _, c := range ledger.calls {
		if c.reason == jobcommon.SeenReasonError && len(c.ids) == 1 && c.ids[0] == "job-1" {
			foundErr = true
		}
	}
	if !foundErr {
		t.Errorf("expected error ledger entry for job-1 after panic, got %+v", ledger.calls)
	}
	// Panic recovery Nak's the message so NATS redelivers it.
	if !state.Naked {
		t.Errorf("expected msg Nak'd after panic recovery")
	}
}

// ── processFetch edge cases ─────────────────────────────────────────────────

func TestProcessFetch_EmptyRecordsReturnsNil(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)
	if err := w.processFetch(context.Background(), nil); err != nil {
		t.Errorf("expected nil for empty msgs, got %v", err)
	}
	if scorer.calls != 0 {
		t.Errorf("no work should have happened: scorer=%d", scorer.calls)
	}
}

func TestProcessFetch_MalformedMixedWithGood(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"job-good"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)
	malformed, malformedState := js.NewTestMsg([]byte("not json"), 1)
	goodPayload, _ := json.Marshal(makeJob("job-good"))
	good, goodState := js.NewTestMsg(goodPayload, 1)

	if err := w.processFetch(context.Background(), []*js.Msg{malformed, good}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	// Malformed record → Term'd + published to DLQ.
	if !malformedState.Termed {
		t.Errorf("expected malformed msg Term'd")
	}
	if len(pub.recordsOnSubject(js.SubjectJobsDLQ)) != 1 {
		t.Errorf("expected malformed payload on DLQ, got %d", len(pub.recordsOnSubject(js.SubjectJobsDLQ)))
	}
	// Good record → scored relevant → Ack'd + published to filtered.
	if !goodState.Acked {
		t.Errorf("expected good msg Ack'd")
	}
	if len(pub.recordsOnSubject("jobs.filtered.")) != 1 {
		t.Errorf("expected good record on filtered subject, got %d", len(pub.recordsOnSubject("jobs.filtered.")))
	}
}

// ── AC-6: ordering tests — ledger called before Ack ─────────────────────────

// orderingLedger captures, for each UpdateSeenReasonBatch call, whether any of
// the tracked messages had already been Acked at that moment. After all
// processChunk calls we can also inspect calls and retryBatches like fakeLedger.
type orderingLedger struct {
	mu            sync.Mutex
	calls         []ledgerCall
	retryBatches  [][]jobsvc.RetryLedgerEntry
	retryErr      error
	errOnCall     int    // if > 0, return error on the Nth UpdateSeenReasonBatch call (1-based)
	ackedAtLedger []bool // element i = was any tracked msg Acked at ledger call i?
	tracked       []*js.TestMsgState
}

func newOrderingLedger(states ...*js.TestMsgState) *orderingLedger {
	return &orderingLedger{tracked: states}
}

func (o *orderingLedger) UpdateSeenReasonBatch(_ context.Context, ids []string, reason string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	callIdx := len(o.calls) + 1

	// Snapshot acked state of all tracked messages at this instant.
	anyAcked := false
	for _, s := range o.tracked {
		if s.Acked {
			anyAcked = true
			break
		}
	}
	o.ackedAtLedger = append(o.ackedAtLedger, anyAcked)
	o.calls = append(o.calls, ledgerCall{ids: append([]string(nil), ids...), reason: reason})

	if o.errOnCall > 0 && callIdx == o.errOnCall {
		return errors.New("injected ledger error")
	}
	return nil
}

func (o *orderingLedger) UpdateRetryBatch(_ context.Context, entries []jobsvc.RetryLedgerEntry) error {
	if o.retryErr != nil {
		return o.retryErr
	}
	dup := append([]jobsvc.RetryLedgerEntry(nil), entries...)
	o.mu.Lock()
	o.retryBatches = append(o.retryBatches, dup)
	o.mu.Unlock()
	return nil
}

// makeWorkerWithInterface builds a Worker accepting any FilterLedger implementation.
// Use this when you need to pass an orderingLedger or other non-fakeLedger implementation.
func makeWorkerWithInterface(scorer *fakeScorer, ledger FilterLedger, pub *fakePublisher) *Worker {
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

// TestProcessChunk_PassThrough_AcksAfterLedger asserts that for the pass-through
// path (no title), UpdateSeenReasonBatch is invoked before any msg.Ack().
func TestProcessChunk_PassThrough_AcksAfterLedger(t *testing.T) {
	scorer := &fakeScorer{}
	pub := &fakePublisher{}

	item1, state1 := makeChunkItem(ats.RawJob{ID: "pt-order-1", Title: ""}, 1)
	item2, state2 := makeChunkItem(ats.RawJob{ID: "pt-order-2", Title: ""}, 1)

	ledger := newOrderingLedger(state1, state2)

	w := makeWorkerWithInterface(scorer, ledger, pub)
	_ = w.processChunk(context.Background(), []chunkItem{item1, item2}, "fetch-order", 0, 1)

	// Ledger must have been called exactly once.
	if len(ledger.calls) != 1 {
		t.Fatalf("expected 1 ledger call, got %d", len(ledger.calls))
	}
	if ledger.calls[0].reason != jobcommon.SeenReasonFiltered {
		t.Errorf("expected reason=%q, got %q", jobcommon.SeenReasonFiltered, ledger.calls[0].reason)
	}

	// At the time of the ledger call, no message should have been Acked yet.
	if ledger.ackedAtLedger[0] {
		t.Errorf("pass-through: msg was Acked BEFORE UpdateSeenReasonBatch — ordering bug not fixed")
	}

	// After processChunk returns, all messages must be Acked.
	if !state1.Acked {
		t.Errorf("expected item1 Acked after processChunk")
	}
	if !state2.Acked {
		t.Errorf("expected item2 Acked after processChunk")
	}
}

// TestProcessChunk_PassThrough_LedgerErrorNaks asserts that when
// UpdateSeenReasonBatch returns a (non-missing) error, the chunk's messages are
// Nak'd — not Acked. Acking on a failed ledger UPDATE would leave the ledger
// row stuck at 'fetch' ("Awaiting Filter") indefinitely.
func TestProcessChunk_PassThrough_LedgerErrorNaks(t *testing.T) {
	scorer := &fakeScorer{}
	pub := &fakePublisher{}

	item1, state1 := makeChunkItem(ats.RawJob{ID: "pt-err-1", Title: ""}, 1)
	item2, state2 := makeChunkItem(ats.RawJob{ID: "pt-err-2", Title: ""}, 1)

	ledger := newOrderingLedger(state1, state2)
	ledger.errOnCall = 1 // first call returns error

	w := makeWorkerWithInterface(scorer, ledger, pub)
	_ = w.processChunk(context.Background(), []chunkItem{item1, item2}, "fetch-err", 0, 1)

	if state1.Acked || state2.Acked {
		t.Errorf("expected no Acks when ledger returns error; got acked1=%v acked2=%v", state1.Acked, state2.Acked)
	}
	if !state1.Naked || !state2.Naked {
		t.Errorf("expected both items Nak'd when ledger returns error; got naked1=%v naked2=%v", state1.Naked, state2.Naked)
	}
}

// TestProcessChunk_Scorable_AcksAfterLedger asserts the per-bucket ordering:
// each bucket's UpdateSeenReasonBatch completes before that bucket's msg.Ack().
// With the published-OK partition, the relevant bucket (publish → mark filtered
// → ack) is processed before the irrelevant bucket (mark llm_irrelevant → ack).
func TestProcessChunk_Scorable_AcksAfterLedger(t *testing.T) {
	// relevant=job-s-1, irrelevant=job-s-2
	scorer := &fakeScorer{relevantIDs: []string{"job-s-1"}}
	pub := &fakePublisher{}

	item1, state1 := makeChunkItem(makeJob("job-s-1"), 1)
	item2, state2 := makeChunkItem(makeJob("job-s-2"), 1)

	ledger := newOrderingLedger(state1, state2)

	w := makeWorkerWithInterface(scorer, ledger, pub)
	_ = w.processChunk(context.Background(), []chunkItem{item1, item2}, "fetch-score-order", 0, 1)

	// Expect 2 ledger calls: filtered (relevant) first, then llm_irrelevant.
	if len(ledger.calls) != 2 {
		t.Fatalf("expected 2 ledger calls, got %d: %+v", len(ledger.calls), ledger.calls)
	}
	if ledger.calls[0].reason != jobcommon.SeenReasonFiltered {
		t.Errorf("expected first call reason=%q, got %q", jobcommon.SeenReasonFiltered, ledger.calls[0].reason)
	}
	if ledger.calls[1].reason != jobcommon.SeenReasonLLMIrrelevant {
		t.Errorf("expected second call reason=%q, got %q", jobcommon.SeenReasonLLMIrrelevant, ledger.calls[1].reason)
	}

	// Crash-safety invariant: at the first (relevant) ledger call, no message is
	// Acked yet — the ledger write always precedes the bucket's Ack.
	if ledger.ackedAtLedger[0] {
		t.Errorf("scorable: a msg was Acked BEFORE the first UpdateSeenReasonBatch call — ordering bug")
	}

	// After processChunk returns, all messages must be Acked.
	if !state1.Acked {
		t.Errorf("expected item1 (relevant) Acked after processChunk")
	}
	if !state2.Acked {
		t.Errorf("expected item2 (irrelevant) Acked after processChunk")
	}
}

// ── integration-style: crawler→filter race against the real sqlite ledger ─────

// newFilterLedgerDB returns an in-memory sqlite DB with the ledger + companies
// tables migrated, backing a real jobsvc.NewSeenLedgerStore. This lets the race
// test drive the actual RowsAffected → ErrLedgerRowMissing path instead of a
// hand-rolled fake.
func newFilterLedgerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable fks: %v", err)
	}
	if err := db.AutoMigrate(&companymodel.Company{}, &jobmodel.JobSeenLedger{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&companymodel.Company{ID: "co-race", Name: "Race Co", Slug: "race-co"}).Error; err != nil {
		t.Fatalf("seed company: %v", err)
	}
	return db
}

// TestFilter_CrawlerRaceScenario_NoPhantomRow (#23) — simulates the
// crawler→filter race: a message arrives whose ledger row has not yet been
// INSERTed. The filter must Nak (not Ack) and must NOT create a phantom ledger
// row. After the crawler INSERT lands, replaying the message transitions the
// row to 'filtered' normally.
func TestFilter_CrawlerRaceScenario_NoPhantomRow(t *testing.T) {
	db := newFilterLedgerDB(t)
	store := jobsvc.NewSeenLedgerStore(db)
	scorer := &fakeScorer{relevantIDs: []string{"race-1"}}
	pub := &fakePublisher{}

	w := &Worker{
		scorer:          scorer,
		ledger:          store,
		publisher:       pub,
		batchSize:       5,
		flushTimeout:    30 * time.Second,
		llmCallInterval: 5 * time.Second,
		clock:           realClock{},
		log:             logger.NewNoop(),
	}

	// Phase 1 — ledger row does NOT exist yet (publish raced ahead of INSERT).
	item, state := makeChunkItem(ats.RawJob{ID: "race-1", Adapter: "greenhouse", Title: "Engineer"}, 1)
	_ = w.processChunk(context.Background(), []chunkItem{item}, "fetch-race", 0, 1)

	if !state.Naked {
		t.Errorf("expected message Nak'd when ledger row missing")
	}
	if state.Acked {
		t.Errorf("message must NOT be Acked when ledger row missing")
	}
	var phantom int64
	if err := db.Model(&jobmodel.JobSeenLedger{}).Where("job_id = ?", "race-1").Count(&phantom).Error; err != nil {
		t.Fatalf("count phantom: %v", err)
	}
	if phantom != 0 {
		t.Fatalf("filter created a phantom ledger row for race-1; want 0, got %d", phantom)
	}

	// Phase 2 — crawler INSERT lands (reason='fetch'), then the message replays.
	if err := store.MarkSeenBatch(context.Background(), []string{"race-1"}, "co-race", jobcommon.SeenReasonFetch); err != nil {
		t.Fatalf("crawler MarkSeenBatch: %v", err)
	}
	item2, state2 := makeChunkItem(ats.RawJob{ID: "race-1", Adapter: "greenhouse", Title: "Engineer"}, 2)
	_ = w.processChunk(context.Background(), []chunkItem{item2}, "fetch-race", 0, 1)

	if !state2.Acked {
		t.Errorf("expected replayed message Acked once the ledger row exists")
	}
	var got jobmodel.JobSeenLedger
	if err := db.Where("job_id = ?", "race-1").First(&got).Error; err != nil {
		t.Fatalf("readback: %v", err)
	}
	if got.Reason != jobcommon.SeenReasonFiltered {
		t.Errorf("after replay, reason = %q, want filtered", got.Reason)
	}
}

// ── mergedLocations tests ────────────────────────────────────────────────────

func TestMergedLocations(t *testing.T) {
	tests := []struct {
		name     string
		job      ats.RawJob
		wantLen  int
		wantNil  bool
		contains string // non-empty: assert result[0] == this value
	}{
		{
			name: "AllLocations populated — returned directly",
			job: ats.RawJob{
				Location:     "New York, NY",
				AllLocations: []string{"New York, NY", "London, UK"},
			},
			wantLen:  2,
			contains: "New York, NY",
		},
		{
			name: "AllLocations nil, Location set — wrapped in single-element slice",
			job: ats.RawJob{
				Location:     "San Francisco, CA",
				AllLocations: nil,
			},
			wantLen:  1,
			contains: "San Francisco, CA",
		},
		{
			name: "AllLocations empty slice, Location set — wrapped in single-element slice",
			job: ats.RawJob{
				Location:     "Austin, TX",
				AllLocations: []string{},
			},
			wantLen:  1,
			contains: "Austin, TX",
		},
		{
			name:    "Both AllLocations and Location empty — returns nil",
			job:     ats.RawJob{},
			wantNil: true,
		},
		{
			name: "AllLocations populated, Location also set — AllLocations takes precedence",
			job: ats.RawJob{
				Location:     "Remote",
				AllLocations: []string{"Boston, MA"},
			},
			wantLen:  1,
			contains: "Boston, MA",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mergedLocations(tc.job)
			if tc.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if len(got) != tc.wantLen {
				t.Errorf("expected len=%d, got len=%d (%v)", tc.wantLen, len(got), got)
				return
			}
			if tc.contains != "" && got[0] != tc.contains {
				t.Errorf("expected got[0]=%q, got %q", tc.contains, got[0])
			}
		})
	}
}

// ── observability metric tests for ledger failure paths ──────────────────────

// TestProcessChunk_DLQLedgerFailure_IncrementsCommitFailures verifies that when
// UpdateSeenReasonBatch for DLQ'd jobs returns an error, FilterCommitFailuresTotal
// increments (P1 alertability requirement).
func TestProcessChunk_DLQLedgerFailure_IncrementsCommitFailures(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{
		seenErrForReason: map[string]error{
			jobcommon.SeenReasonError: errors.New("db write failed"),
		},
	}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	before := promtestutil.ToFloat64(metrics.FilterCommitFailuresTotal)

	// Exhaust the retry budget so the job goes straight to DLQ ledger write.
	_ = w.processChunk(context.Background(),
		makeBatchWithRetryCount(filterMaxRetries, makeJob("dlq-ledger-fail")),
		"fetch-dlq-ledger", 0, 1,
	)

	after := promtestutil.ToFloat64(metrics.FilterCommitFailuresTotal)
	if after-before != 1 {
		t.Errorf("expected FilterCommitFailuresTotal +1 on DLQ ledger failure, got +%v", after-before)
	}
}

// TestProcessChunk_RetryBatchLedgerFailure_IncrementsCommitFailures verifies that
// when UpdateRetryBatch returns an error, FilterCommitFailuresTotal increments.
func TestProcessChunk_RetryBatchLedgerFailure_IncrementsCommitFailures(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{retryErr: errors.New("db retry write failed")}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	before := promtestutil.ToFloat64(metrics.FilterCommitFailuresTotal)

	// retryCount=0 so the job goes to UpdateRetryBatch (not DLQ).
	item, state := makeChunkItem(makeJob("retry-ledger-fail"), 1)
	_ = w.processChunk(context.Background(), []chunkItem{item}, "fetch-retry-ledger", 0, 1)

	after := promtestutil.ToFloat64(metrics.FilterCommitFailuresTotal)
	if after-before != 1 {
		t.Errorf("expected FilterCommitFailuresTotal +1 on UpdateRetryBatch failure, got +%v", after-before)
	}
	// Message is still Nak'd (NATS redelivers) even when ledger write fails.
	if !state.Naked {
		t.Errorf("expected job Nak'd even when UpdateRetryBatch fails")
	}
}

// TestProcessChunk_NonUSA_LedgerError_NaksNotAcks verifies Task 1 fix:
// when UpdateSeenReasonBatch for non-USA items fails, messages are Nak'd
// (not Acked as the old inline code did).
func TestProcessChunk_NonUSA_LedgerError_NaksNotAcks(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{
		seenErrForReason: map[string]error{
			jobcommon.SeenReasonNonUSA: errors.New("db down"),
		},
	}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)

	item, state := makeChunkItem(ats.RawJob{
		ID:            "uk-fail",
		Title:         "SWE",
		Location:      "London, UK",
		LocationKnown: true,
		AllLocations:  []string{"London, UK"},
	}, 1)

	_ = w.processChunk(context.Background(), []chunkItem{item}, "fetch-nonusa-err", 0, 1)

	if state.Acked {
		t.Errorf("non-USA item must NOT be Acked when ledger write fails (phantom fetch bug)")
	}
	if !state.Naked {
		t.Errorf("non-USA item must be Nak'd when ledger write fails so NATS redelivers")
	}
}

// TestProcessChunk_NonUSA_LedgerRowMissing_NaksAndIncrementsMissing verifies that
// ErrLedgerRowMissing on the non-USA path increments filter_ledger_missing_total
// and Naks (not Acks) the affected messages.
func TestProcessChunk_NonUSA_LedgerRowMissing_NaksAndIncrementsMissing(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{
		seenErrForReason: map[string]error{
			jobcommon.SeenReasonNonUSA: domainerr.ErrLedgerRowMissing.WithMessage("race"),
		},
	}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	before := promtestutil.ToFloat64(metrics.FilterLedgerMissing.WithLabelValues(jobcommon.SeenReasonNonUSA))

	item, state := makeChunkItem(ats.RawJob{
		ID:            "uk-missing",
		Title:         "SWE",
		Location:      "London, UK",
		LocationKnown: true,
		AllLocations:  []string{"London, UK"},
	}, 1)

	_ = w.processChunk(context.Background(), []chunkItem{item}, "fetch-nonusa-missing", 0, 1)

	after := promtestutil.ToFloat64(metrics.FilterLedgerMissing.WithLabelValues(jobcommon.SeenReasonNonUSA))
	if after-before != 1 {
		t.Errorf("expected filter_ledger_missing_total{non_usa_location} +1, got +%v", after-before)
	}
	if state.Acked {
		t.Errorf("must NOT Ack when ledger row missing")
	}
	if !state.Naked {
		t.Errorf("must Nak when ledger row missing")
	}
}
