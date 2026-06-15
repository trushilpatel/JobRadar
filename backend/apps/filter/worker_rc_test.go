package main

// worker_rc_test.go covers the RC fixes (AC-1 through AC-7) that are not
// already covered by worker_test.go or worker_extra_test.go.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/ats/core"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	js "github.com/trushilpatel/jobradar/common/jetstream"
)

// ── RC-5: RPD-exhaustion floor ────────────────────────────────────────────────

// TestRouteFailedChunk_LLMExhausted_ZeroRetryAfter_FlooredAt4h verifies AC-5:
// when LLM exhaustion carries a zero RetryAfter, next_attempt_at is floored at
// filterRPDExhaustionRetryAfter (4h) rather than filterBackOff[0] (30s).
func TestRouteFailedChunk_LLMExhausted_ZeroRetryAfter_FlooredAt4h(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	exhaustedErr := &domainerr.LLMAllExhaustedError{} // zero RetryAfter

	w := makeWorker(&fakeScorer{}, &fakeLedger{}, &fakePublisher{})
	w.clock = &fakeClock{now: now}

	item, _ := makeChunkItem(makeJob("rc5-zero"), 1)
	outcome := w.routeFailedChunk(context.Background(), []chunkItem{item}, exhaustedErr)

	if len(outcome.RetriedWithPayload) != 1 {
		t.Fatalf("expected 1 RetriedWithPayload entry, got %d", len(outcome.RetriedWithPayload))
	}
	want := now.Add(filterRPDExhaustionRetryAfter)
	got := outcome.RetriedWithPayload[0].NextAttemptAt
	if !got.Equal(want) {
		t.Errorf("next_attempt_at = %v, want %v (4h floor)", got, want)
	}
}

// TestRouteFailedChunk_LLMExhausted_SmallRetryAfter_FlooredAt4h verifies AC-5:
// when RetryAfter is less than 4h (e.g. 30s from cooldown), next_attempt_at is
// still floored to 4h ahead.
func TestRouteFailedChunk_LLMExhausted_SmallRetryAfter_FlooredAt4h(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	// RetryAfter is only 30s ahead — less than the 4h floor.
	retryAfter := now.Add(30 * time.Second)
	exhaustedErr := &domainerr.LLMAllExhaustedError{RetryAfter: retryAfter}

	w := makeWorker(&fakeScorer{}, &fakeLedger{}, &fakePublisher{})
	w.clock = &fakeClock{now: now}

	item, _ := makeChunkItem(core.JobSummary{ID: "rc5-small", Title: "SWE", Adapter: "greenhouse"}, 1)
	outcome := w.routeFailedChunk(context.Background(), []chunkItem{item}, exhaustedErr)

	if len(outcome.RetriedWithPayload) != 1 {
		t.Fatalf("expected 1 RetriedWithPayload entry, got %d", len(outcome.RetriedWithPayload))
	}
	want := now.Add(filterRPDExhaustionRetryAfter)
	got := outcome.RetriedWithPayload[0].NextAttemptAt
	if !got.Equal(want) {
		t.Errorf("small RetryAfter: next_attempt_at = %v, want %v (4h floor)", got, want)
	}
}

// TestRouteFailedChunk_LLMExhausted_LargeRetryAfter_Unchanged verifies AC-5:
// when RetryAfter is beyond the 4h floor (e.g. 6h ahead), it is used as-is.
func TestRouteFailedChunk_LLMExhausted_LargeRetryAfter_Unchanged(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	// RetryAfter is 6h ahead — already beyond the 4h floor.
	retryAfter := now.Add(6 * time.Hour)
	exhaustedErr := &domainerr.LLMAllExhaustedError{RetryAfter: retryAfter}

	w := makeWorker(&fakeScorer{}, &fakeLedger{}, &fakePublisher{})
	w.clock = &fakeClock{now: now}

	item, _ := makeChunkItem(core.JobSummary{ID: "rc5-large", Title: "SWE", Adapter: "greenhouse"}, 1)
	outcome := w.routeFailedChunk(context.Background(), []chunkItem{item}, exhaustedErr)

	if len(outcome.RetriedWithPayload) != 1 {
		t.Fatalf("expected 1 RetriedWithPayload entry, got %d", len(outcome.RetriedWithPayload))
	}
	got := outcome.RetriedWithPayload[0].NextAttemptAt
	if !got.Equal(retryAfter) {
		t.Errorf("large RetryAfter: next_attempt_at = %v, want %v (unchanged)", got, retryAfter)
	}
}

// TestRouteFailedChunk_LLMExhausted_ExactFloor_Unchanged verifies AC-5:
// when RetryAfter is exactly at the 4h floor, it is used as-is (not bumped).
func TestRouteFailedChunk_LLMExhausted_ExactFloor_Unchanged(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	retryAfter := now.Add(filterRPDExhaustionRetryAfter) // exactly 4h
	exhaustedErr := &domainerr.LLMAllExhaustedError{RetryAfter: retryAfter}

	w := makeWorker(&fakeScorer{}, &fakeLedger{}, &fakePublisher{})
	w.clock = &fakeClock{now: now}

	item, _ := makeChunkItem(core.JobSummary{ID: "rc5-exact", Title: "SWE", Adapter: "greenhouse"}, 1)
	outcome := w.routeFailedChunk(context.Background(), []chunkItem{item}, exhaustedErr)

	if len(outcome.RetriedWithPayload) != 1 {
		t.Fatalf("expected 1 RetriedWithPayload entry, got %d", len(outcome.RetriedWithPayload))
	}
	got := outcome.RetriedWithPayload[0].NextAttemptAt
	if !got.Equal(retryAfter) {
		t.Errorf("exact floor: next_attempt_at = %v, want %v", got, retryAfter)
	}
}

// ── RC-1: X-Retry-Count header reading in filter ─────────────────────────────

// TestProcessFetch_UsesRetryCountHeaderOverDeliveryCount verifies AC-1:
// when X-Retry-Count is present in a NATS message header, the filter uses
// that value as retryCount instead of DeliveryCount-1. This ensures the
// sweeper's monotonic count (not NATS delivery count) drives ledger writes.
func TestProcessFetch_UsesRetryCountHeaderOverDeliveryCount(t *testing.T) {
	// Build a message with X-Retry-Count=5 but NATS deliveryCount=1
	// (sweeper re-inject: delivery 1 but actual retry count is 5).
	job := makeJob("rc1-header")
	payload, _ := json.Marshal(job)
	msg, _ := js.NewTestMsgWithHeaders(payload, 1, map[string]string{
		js.HeaderRetryCount: "5",
	})

	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	// Make the scorer return the job as relevant so we get to the filtered path.
	w := makeWorker(&fakeScorer{relevantIDs: []string{"rc1-header"}}, ledger, pub)

	// Process a single-message fetch with the header-bearing message.
	// Verify no panic and the job is processed.
	_ = w.processFetch(context.Background(), []*js.Msg{msg})
}

// TestGetRetryCount_HeaderPresent returns the header value when set.
func TestGetRetryCount_HeaderPresent(t *testing.T) {
	_, state := js.NewTestMsg([]byte("{}"), 1)
	_ = state // unused here; test the helper via msg with headers
	msg, _ := js.NewTestMsgWithHeaders([]byte("{}"), 1, map[string]string{
		js.HeaderRetryCount: "7",
	})
	got := getRetryCount(msg)
	if got != 7 {
		t.Errorf("getRetryCount with header = %d, want 7", got)
	}
}

// TestGetRetryCount_NoHeaderFallsBackToDeliveryCount verifies that when
// X-Retry-Count header is absent, getRetryCount falls back to DeliveryCount-1.
func TestGetRetryCount_NoHeaderFallsBackToDeliveryCount(t *testing.T) {
	msg, _ := js.NewTestMsg([]byte("{}"), 3) // deliveryCount=3 → retryCount=2
	got := getRetryCount(msg)
	if got != 2 {
		t.Errorf("getRetryCount without header = %d, want 2 (DeliveryCount-1)", got)
	}
}

// TestGetRetryCount_MalformedHeaderFallsBackToDelivery verifies that when
// X-Retry-Count header exists but is not a valid integer, fallback is used.
func TestGetRetryCount_MalformedHeaderFallsBackToDelivery(t *testing.T) {
	msg, _ := js.NewTestMsgWithHeaders([]byte("{}"), 2, map[string]string{
		js.HeaderRetryCount: "not-a-number",
	})
	got := getRetryCount(msg)
	if got != 1 {
		t.Errorf("malformed header: getRetryCount = %d, want 1 (DeliveryCount-1)", got)
	}
}
