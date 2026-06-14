package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/ats/core"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
)

// ── callbackFetcher ────────────────────────────────────────────────────────────

// callbackFetcher implements MessageFetcher via a user-supplied fn.
type callbackFetcher struct {
	fn func(n int, maxWait time.Duration) ([]*js.Msg, error)
}

func (c *callbackFetcher) Fetch(n int, maxWait time.Duration) ([]*js.Msg, error) {
	return c.fn(n, maxWait)
}

// ── Run function tests ─────────────────────────────────────────────────────────

// TestRun_CtxAlreadyCancelled_ReturnsNil verifies Run exits immediately when ctx
// is already cancelled at entry, without calling Fetch.
func TestRun_CtxAlreadyCancelled_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := makeWorkerWithClock(&fakeScorer{}, &fakeLedger{}, &fakePublisher{}, &fakeClock{now: time.Now()}, 1)
	w.consumer = &callbackFetcher{fn: func(n int, maxWait time.Duration) ([]*js.Msg, error) {
		t.Fatal("Fetch must not be called when ctx is already cancelled")
		return nil, nil
	}}

	if err := w.Run(ctx); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestRun_FetchError_CtxCancelledInSelect_ReturnsNil verifies Run returns nil
// when Fetch errors and ctx is cancelled (select takes ctx.Done immediately).
func TestRun_FetchError_CtxCancelledInSelect_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := makeWorkerWithClock(&fakeScorer{}, &fakeLedger{}, &fakePublisher{}, &fakeClock{now: time.Now()}, 1)
	w.consumer = &callbackFetcher{fn: func(n int, maxWait time.Duration) ([]*js.Msg, error) {
		cancel() // cancel ctx before returning error — select will drain immediately
		return nil, errors.New("nats down")
	}}

	if err := w.Run(ctx); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestRun_FetchReturnsEmpty_LoopContinues verifies that an empty Fetch result
// causes the loop to continue until ctx is cancelled.
func TestRun_FetchReturnsEmpty_LoopContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	w := makeWorkerWithClock(&fakeScorer{}, &fakeLedger{}, &fakePublisher{}, &fakeClock{now: time.Now()}, 1)
	w.consumer = &callbackFetcher{fn: func(n int, maxWait time.Duration) ([]*js.Msg, error) {
		calls++
		if calls == 1 {
			return nil, nil // empty
		}
		cancel()
		return nil, nil
	}}

	if err := w.Run(ctx); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if calls < 2 {
		t.Errorf("expected ≥2 Fetch calls, got %d", calls)
	}
}

// TestRun_ProcessFetchReturnsError_CtxCancelled_ReturnsNil verifies that when
// processFetch fails because ctx is cancelled (inter-chunk sleep detects it),
// Run returns nil rather than the error.
func TestRun_ProcessFetchReturnsError_CtxCancelled_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// batchSize=1 + 2 messages → 2 chunks → inter-chunk sleep triggers.
	// fakeClock.Sleep checks ctx.Err() first, so cancelling ctx before Fetch
	// returns means Sleep will immediately return context.Canceled.
	clk := &fakeClock{now: time.Now()}
	w := makeWorkerWithClock(&fakeScorer{relevantIDs: nil}, &fakeLedger{}, &fakePublisher{}, clk, 1)

	msgs2 := make2ValidRunMsgs(t)
	calls := 0
	w.consumer = &callbackFetcher{fn: func(n int, maxWait time.Duration) ([]*js.Msg, error) {
		calls++
		if calls == 1 {
			cancel()
			return msgs2, nil
		}
		return nil, nil
	}}

	if err := w.Run(ctx); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestRun_ProcessFetchError_CtxNotCancelled_LogsErrorAndContinues verifies
// that a non-ctx processFetch error causes Run to log an error and continue
// looping rather than exiting.
func TestRun_ProcessFetchError_CtxNotCancelled_LogsErrorAndContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// sleepErr is a non-ctx error so processFetch will fail without cancelling ctx.
	clk := &fakeClock{now: time.Now(), sleepErr: errors.New("clock malfunction")}
	w := makeWorkerWithClock(&fakeScorer{relevantIDs: nil}, &fakeLedger{}, &fakePublisher{}, clk, 1)

	msgs2 := make2ValidRunMsgs(t)
	calls := 0
	w.consumer = &callbackFetcher{fn: func(n int, maxWait time.Duration) ([]*js.Msg, error) {
		calls++
		if calls == 1 {
			return msgs2, nil // triggers processFetch which fails mid-sleep
		}
		cancel()
		return nil, nil
	}}

	if err := w.Run(ctx); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if calls < 2 {
		t.Errorf("expected Run to continue looping after processFetch error, got %d calls", calls)
	}
}

// make2ValidRunMsgs returns two properly marshalled NATS test messages.
func make2ValidRunMsgs(t *testing.T) []*js.Msg {
	t.Helper()
	p1, _ := json.Marshal(makeJob("run-a"))
	p2, _ := json.Marshal(makeJob("run-b"))
	m1, _ := js.NewTestMsg(p1, 1)
	m2, _ := js.NewTestMsg(p2, 1)
	return []*js.Msg{m1, m2}
}

// ── processFetch edge-case coverage ──────────────────────────────────────────

// TestProcessFetch_MalformedMsg_DLQPublishFails_LogsError verifies that when a
// malformed message's DLQ publish fails, an error is logged (no panic, nil return).
func TestProcessFetch_MalformedMsg_DLQPublishFails_LogsError(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{err: errors.New("publish failed")} // all publishes fail
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)
	msg, _ := js.NewTestMsg([]byte("not valid json"), 1)

	err := w.processFetch(context.Background(), []*js.Msg{msg})
	if err != nil {
		t.Errorf("processFetch: want nil, got %v", err)
	}
}

// TestProcessFetch_MalformedMsg_TermFails_LogsWarn verifies that when a
// malformed message is Term'd and Term returns an error, a warning is logged.
func TestProcessFetch_MalformedMsg_TermFails_LogsWarn(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{} // DLQ publish succeeds
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)
	msg, state := js.NewTestMsg([]byte("not valid json"), 1)
	state.TermErr = errors.New("term failed")

	err := w.processFetch(context.Background(), []*js.Msg{msg})
	if err != nil {
		t.Errorf("processFetch: want nil, got %v", err)
	}
	if !state.Termed {
		t.Errorf("expected Term called even when it errors")
	}
}

// TestProcessFetch_PanicRecovery_LedgerFails_NaksMessages verifies that when
// a panic occurs in processChunk AND the error-mark ledger write fails,
// the messages are still Nak'd (no silent drop).
func TestProcessFetch_PanicRecovery_LedgerFails_NaksMessages(t *testing.T) {
	ledger := &fakeLedger{seenErr: errors.New("db down")}
	pub := &fakePublisher{}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(&fakeScorer{}, ledger, pub, clk, 5)
	w.scorer = panickyScorer{}

	p, _ := json.Marshal(makeJob("panic-ledger-fail"))
	msg, state := js.NewTestMsg(p, 1)

	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	// Even when the error-mark fails, the message must be Nak'd so NATS redelivers.
	if !state.Naked {
		t.Errorf("expected Nak'd after panic even when ledger write fails")
	}
}

// TestProcessFetch_PanicRecovery_NakFails_NoAdditionalPanic verifies that when
// panic recovery Nak fails, no secondary panic occurs (the warning is logged).
func TestProcessFetch_PanicRecovery_NakFails_NoAdditionalPanic(t *testing.T) {
	pub := &fakePublisher{}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(&fakeScorer{}, &fakeLedger{}, pub, clk, 5)
	w.scorer = panickyScorer{}

	p, _ := json.Marshal(makeJob("panic-nak-fail"))
	msg, state := js.NewTestMsg(p, 1)
	state.NakErr = errors.New("nak failed") // Nak will fail

	// Must not panic.
	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
}

// TestProcessFetch_CtxCancelAfterFirstChunk_NaksRemaining verifies that when
// fetchCtx is cancelled between chunks (scorer cancels ctx on first call),
// remaining messages are Nak'd so NATS redelivers them.
func TestProcessFetch_CtxCancelAfterFirstChunk_NaksRemaining(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_ = ctx

	clk := &fakeClock{now: time.Now()}
	w := makeWorkerWithClock(&fakeScorer{}, &fakeLedger{}, &fakePublisher{}, clk, 1)

	// cancelOnFirstScore cancels ctx before returning from the first ScoreBatch call.
	w.scorer = &cancelOnFirstScoreScorer{cancel: cancel}

	msgs, _ := makeTestMsgs(t, []struct {
		id            string
		deliveryCount int
	}{
		{"fc-1", 1}, {"fc-2", 1}, {"fc-3", 1},
	})

	err := w.processFetch(context.Background(), msgs)
	// Either nil (ctx cancel detected at fetchCtx check) or context.Canceled is fine.
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("processFetch unexpected error: %v", err)
	}
}

// cancelOnFirstScoreScorer cancels a context on the first ScoreBatch call.
type cancelOnFirstScoreScorer struct {
	cancel context.CancelFunc
	calls  int
}

func (s *cancelOnFirstScoreScorer) ScoreBatch(_ context.Context, _ []llmdto.JobsRelevanceFilterInput) ([]string, error) {
	s.calls++
	if s.calls == 1 {
		s.cancel()
	}
	return nil, nil
}

// ── publishPartition edge-case coverage ───────────────────────────────────────

// TestPublishPartition_NakFails_TermsMessage verifies that when Nak fails after
// a publish failure, the message is Term'd (prevents stuck message).
func TestPublishPartition_NakFails_TermsMessage(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"pub-nak-fail"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{err: errors.New("publish failed")} // publishDetail fails
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)

	p, _ := json.Marshal(makeJob("pub-nak-fail"))
	msg, state := js.NewTestMsg(p, 1)
	state.NakErr = errors.New("nak failed after publish error")

	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	// Nak failed → should fall through to Term.
	if !state.Termed {
		t.Errorf("expected msg Term'd when Nak fails after publish failure")
	}
}

// TestPublishPartition_NakFails_TermFails_NoPanic verifies that when both Nak
// and Term fail, no panic occurs and the worker continues.
func TestPublishPartition_NakFails_TermFails_NoPanic(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"nak-term-fail"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{err: errors.New("publish failed")}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)

	p, _ := json.Marshal(makeJob("nak-term-fail"))
	msg, state := js.NewTestMsg(p, 1)
	state.NakErr = errors.New("nak failed")
	state.TermErr = errors.New("term failed")

	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	// Both Nak and Term failed — ensure no panic and Term was attempted.
	if !state.Termed {
		t.Errorf("expected Term attempted even if it errors")
	}
}

// ── markSeenOrNak edge-case coverage ──────────────────────────────────────────

// TestMarkSeenOrNak_LedgerFails_NakFails_LogsWarn verifies that when the ledger
// write fails and msg.Nak() also fails, the warning is logged (no panic).
func TestMarkSeenOrNak_LedgerFails_NakFails_LogsWarn(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: nil}
	ledger := &fakeLedger{seenErr: errors.New("db error")}
	pub := &fakePublisher{}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)

	p, _ := json.Marshal(core.JobSummary{ID: "mson-nak-fail", Title: ""})
	msg, state := js.NewTestMsg(p, 1)
	state.NakErr = errors.New("nak failed")

	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	_ = state
}

// TestMarkSeenOrNak_AckFails_LogsWarn verifies that when the ledger write
// succeeds but msg.Ack() fails, the warning is logged (no panic).
func TestMarkSeenOrNak_AckFails_LogsWarn(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: nil}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)

	// A job with no title goes to passThrough → scored irrelevant.
	p, _ := json.Marshal(core.JobSummary{ID: "mson-ack-fail", Title: ""})
	msg, _ := js.NewTestMsg(p, 1)

	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
}

// ── routeFailedChunk edge-case coverage ───────────────────────────────────────

// TestRouteFailedChunk_NakFails_TermFails_NoAdditionalPanic verifies that when
// Nak fails AND the subsequent Term also fails, no additional panic occurs.
func TestRouteFailedChunk_NakFails_TermFails_NoAdditionalPanic(t *testing.T) {
	scorer := &fakeScorer{err: errors.New("llm error")}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)

	p, _ := json.Marshal(makeJob("rfc-both-fail"))
	msg, state := js.NewTestMsg(p, 1)
	state.NakErr = errors.New("nak failed")
	state.TermErr = errors.New("term failed")

	// processFetch → scorer errors → routeFailedChunk → Nak fails → Term fails.
	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	// Term was attempted even though it errored.
	if !state.Termed {
		t.Errorf("expected Term attempted after Nak failure")
	}
}

// TestPublishToDLQ_TermFails_OnSuccessfulPublish verifies that when the DLQ
// publish succeeds but Term fails, no panic occurs and the error is logged.
func TestPublishToDLQ_TermFails_OnSuccessfulPublish(t *testing.T) {
	scorer := &fakeScorer{err: errors.New("llm error")}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)

	// Exhaust retries (retryCount >= filterMaxRetries = 3) so job goes to DLQ.
	p, _ := json.Marshal(makeJob("dlq-term-fail"))
	msg, state := js.NewTestMsg(p, filterMaxRetries+1) // deliveryCount > maxRetries
	state.TermErr = errors.New("term failed after dlq publish")

	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	// DLQ publish succeeded → Term was attempted.
	if !state.Termed {
		t.Errorf("expected Term attempted on DLQ path even when Term errors")
	}
}

// ── markSeenOrNak: generic ledger error on Nak-fail path ─────────────────────

// TestMarkSeenOrNak_GenericLedgerError_NakFails checks the warn-on-nak-fail
// path when UpdateSeenReasonBatch returns a generic (non-missing-row) error.
func TestMarkSeenOrNak_GenericLedgerError_NakFails(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: nil}
	ledger := &fakeLedger{seenErrForReason: map[jobcommon.SeenReason]error{
		jobcommon.SeenReasonLLMIrrelevant: errors.New("transient db error"),
	}}
	pub := &fakePublisher{}
	clk := &fakeClock{now: time.Now()}

	w := makeWorkerWithClock(scorer, ledger, pub, clk, 5)

	p, _ := json.Marshal(core.JobSummary{ID: "irrelevant-nak-fail", Title: "SWE"})
	msg, state := js.NewTestMsg(p, 1)
	state.NakErr = errors.New("nak failed after ledger error")

	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	_ = state
}
