package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
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
	w.processChunk(context.Background(), chunk, "fetch-1", 0, 1)

	if scorer.calls != 0 {
		t.Errorf("scorer should not be called for titleless chunk, got %d calls", scorer.calls)
	}
	filtered := pub.recordsOnSubject("jobs.filtered.")
	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered publishes, got %d", len(filtered))
	}
	if len(ledger.calls) != 0 {
		t.Errorf("no ledger calls for pass-through, got %d", len(ledger.calls))
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
	w.processChunk(context.Background(), chunk, "fetch-1", 0, 1)

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
	w.processChunk(context.Background(), []chunkItem{item}, "fetch-1", 0, 1)

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
	w.processChunk(context.Background(), []chunkItem{item}, "fetch-1", 0, 1)

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
	w.processChunk(context.Background(), makeBatchWithRetryCount(3, makeJob("job-1")), "fetch-1", 0, 1)

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
	w.processChunk(context.Background(), makeBatch(makeJob("job-1")), "fetch-1", 0, 1)

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
	w.processChunk(context.Background(), []chunkItem{item}, "fetch-1", 0, 1)
	// No assertion beyond "didn't panic".
}

func TestProcessChunk_AllIrrelevant(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: nil} // empty → all irrelevant
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.processChunk(context.Background(), makeBatch(makeJob("a"), makeJob("b")), "f", 0, 1)

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
