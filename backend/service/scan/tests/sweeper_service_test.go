package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	scandto "github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// ── fake implementations ──────────────────────────────────────────────────────

// fakeSweeperLedger is a test double for jobsvc.SeenLedgerStore.
// Only the sweeper-relevant methods have meaningful implementations;
// all others are no-ops so the compile check passes.
type fakeSweeperLedger struct {
	dueJobs        []jobsvc.RetryJobRecord
	fetchErr       error
	resetErr       error
	escalateErr    error
	updateRetryErr error

	resetCalls    []string
	escalateCalls []string
	retryEntries  [][]jobsvc.RetryLedgerEntryWithPayload
}

func (f *fakeSweeperLedger) HasSeen(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *fakeSweeperLedger) HasSeenBatch(_ context.Context, _ []string) (map[string]bool, error) {
	return nil, nil
}
func (f *fakeSweeperLedger) MarkSeen(_ context.Context, _, _, _ string) error { return nil }
func (f *fakeSweeperLedger) MarkSeenBatch(_ context.Context, _ []string, _, _ string) error {
	return nil
}
func (f *fakeSweeperLedger) UpdateSeenReason(_ context.Context, _ string, _ jobcommon.SeenReason) error {
	return nil
}
func (f *fakeSweeperLedger) UpdateSeenReasonBatch(_ context.Context, _ []string, _ jobcommon.SeenReason) error {
	return nil
}
func (f *fakeSweeperLedger) UpdateRetryBatch(_ context.Context, _ []jobsvc.RetryLedgerEntry) error {
	return nil
}
func (f *fakeSweeperLedger) UpdateRetryBatchWithPayload(_ context.Context, entries []jobsvc.RetryLedgerEntryWithPayload) error {
	f.retryEntries = append(f.retryEntries, entries)
	return f.updateRetryErr
}
func (f *fakeSweeperLedger) FetchDueRetryJobs(_ context.Context, _ int) ([]jobsvc.RetryJobRecord, error) {
	return f.dueJobs, f.fetchErr
}
func (f *fakeSweeperLedger) FetchDueRetryJobsWithPayload(_ context.Context, _ int) ([]jobsvc.RetryJobRecord, error) {
	return f.dueJobs, f.fetchErr
}
func (f *fakeSweeperLedger) ResetRetryToFetch(_ context.Context, jobID string) error {
	f.resetCalls = append(f.resetCalls, jobID)
	return f.resetErr
}
func (f *fakeSweeperLedger) EscalateRetryToError(_ context.Context, jobID string) error {
	f.escalateCalls = append(f.escalateCalls, jobID)
	return f.escalateErr
}
func (f *fakeSweeperLedger) MarkRemovedBatch(_ context.Context, _ []string) error { return nil }
func (f *fakeSweeperLedger) GetActiveJobIDsByCompany(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (f *fakeSweeperLedger) EscalateStuckRetryJobs(_ context.Context, _ time.Duration, _ int) (int64, error) {
	return 0, nil
}
func (f *fakeSweeperLedger) ReclaimStuckFetchRows(_ context.Context, _ time.Duration, _ int) (int64, error) {
	return 0, nil
}

var _ jobsvc.SeenLedgerStore = (*fakeSweeperLedger)(nil)

// fakeSweeperPublisher is a test double for SweeperPublisher.
// It records every publish call and can inject a failure.
type fakeSweeperPublisher struct {
	publishErr error
	calls      []sweeperPublishCall
	callCount  atomic.Int64
}

type sweeperPublishCall struct {
	subject string
	payload []byte
}

func (p *fakeSweeperPublisher) Publish(_ context.Context, subject string, payload []byte) error {
	p.callCount.Add(1)
	p.calls = append(p.calls, sweeperPublishCall{subject: subject, payload: payload})
	return p.publishErr
}

func (p *fakeSweeperPublisher) PublishWithHeaders(_ context.Context, subject string, payload []byte, _ map[string]string) error {
	p.callCount.Add(1)
	p.calls = append(p.calls, sweeperPublishCall{subject: subject, payload: payload})
	return p.publishErr
}

// makeSweeperService constructs a scheduler service with sweeper enabled.
// maxRetries defaults to 10 if 0.
func makeSweeperService(q svc.ScanQueueStore, ledger jobsvc.SeenLedgerStore, pub svc.SweeperPublisher, maxRetries int) svc.SchedulerService {
	return svc.NewSchedulerServiceWithSweeper(q, svc.SchedulerServiceOptions{
		Ledger:     ledger,
		Publisher:  pub,
		MaxRetries: maxRetries,
		Log:        logger.NewNoop(),
	})
}

// sweeperRawPayload returns a minimal JSON payload for a fake RawJob.
func sweeperRawPayload(id string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"id":%q}`, id))
}

// sweeperRetryRecord builds a RetryJobRecord for test setup.
func sweeperRetryRecord(jobID, adapter string, retryCount int) jobsvc.RetryJobRecord {
	return jobsvc.RetryJobRecord{
		JobID:      jobID,
		Adapter:    adapter,
		Payload:    sweeperRawPayload(jobID),
		RetryCount: retryCount,
	}
}

// ── no-op queue store for sweeper tests ──────────────────────────────────────

// sweeperFakeQueue satisfies ScanQueueStore; sweeper tests don't call it.
type sweeperFakeQueue struct{}

func (q *sweeperFakeQueue) Enqueue(_ context.Context, _, _ string) error { return nil }
func (q *sweeperFakeQueue) EnqueueDue(_ context.Context, _ int, _ string) (svc.EnqueueDueResult, error) {
	return svc.EnqueueDueResult{}, nil
}
func (q *sweeperFakeQueue) EnqueueAll(_ context.Context, _ string) (int, error) { return 0, nil }
func (q *sweeperFakeQueue) ClaimNext(_ context.Context, _ string) (*scandto.ScanQueueItem, error) {
	return nil, nil
}
func (q *sweeperFakeQueue) Complete(_ context.Context, _ string) error { return nil }
func (q *sweeperFakeQueue) Remove(_ context.Context, _ string) error   { return nil }
func (q *sweeperFakeQueue) ResetStuck(_ context.Context, _ time.Duration) (int, error) {
	return 0, nil
}
func (q *sweeperFakeQueue) PurgeInactive(_ context.Context) (int, error) { return 0, nil }
func (q *sweeperFakeQueue) AutoPauseChronicFailures(_ context.Context, _ int) ([]string, error) {
	return nil, nil
}
func (q *sweeperFakeQueue) UpdateHeartbeat(_ context.Context, _ string) error { return nil }
func (q *sweeperFakeQueue) ListQueue(_ context.Context, _ scandto.QueueFilter) ([]scandto.ScanQueueItem, error) {
	return nil, nil
}

var _ svc.ScanQueueStore = (*sweeperFakeQueue)(nil)

// ── tests ─────────────────────────────────────────────────────────────────────

// TestRetryDueJobs_NoOp_WhenLedgerNil asserts that a scheduler service
// configured without a ledger returns nil immediately without calling the publisher.
func TestRetryDueJobs_NoOp_WhenLedgerNil(t *testing.T) {
	pub := &fakeSweeperPublisher{}
	s := svc.NewSchedulerServiceWithSweeper(&sweeperFakeQueue{}, svc.SchedulerServiceOptions{
		Ledger:    nil,
		Publisher: pub,
		Log:       logger.NewNoop(),
	})

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if pub.callCount.Load() != 0 {
		t.Error("Publish should not be called when ledger is nil")
	}
}

// TestRetryDueJobs_NoOp_WhenPublisherNil asserts that a scheduler service
// configured without a publisher returns nil immediately without calling the ledger.
func TestRetryDueJobs_NoOp_WhenPublisherNil(t *testing.T) {
	ledger := &fakeSweeperLedger{}
	s := svc.NewSchedulerServiceWithSweeper(&sweeperFakeQueue{}, svc.SchedulerServiceOptions{
		Ledger:    ledger,
		Publisher: nil,
		Log:       logger.NewNoop(),
	})

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(ledger.resetCalls) > 0 {
		t.Error("ledger should not be called when publisher is nil")
	}
}

// TestRetryDueJobs_EmptyBatch_NoPublishNoEscalate asserts that an empty batch
// from FetchDueRetryJobs results in no publishes and no escalations.
func TestRetryDueJobs_EmptyBatch_NoPublishNoEscalate(t *testing.T) {
	ledger := &fakeSweeperLedger{dueJobs: []jobsvc.RetryJobRecord{}}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub.callCount.Load() != 0 {
		t.Error("no publishes expected for empty batch")
	}
	if len(ledger.escalateCalls) != 0 {
		t.Error("no escalations expected for empty batch")
	}
}

// TestRetryDueJobs_FetchError_ReturnsWrappedError asserts that a fetch error
// is returned wrapped rather than swallowed.
func TestRetryDueJobs_FetchError_ReturnsWrappedError(t *testing.T) {
	fetchErr := errors.New("db down")
	ledger := &fakeSweeperLedger{fetchErr: fetchErr}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	err := s.RetryDueJobs(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() == "" {
		t.Errorf("error message is empty")
	}
}

// TestRetryDueJobs_UnderBudget_Publishes asserts that a job with
// retryCount < maxRetries is published via PublishWithHeaders (AC-1: no reset step).
func TestRetryDueJobs_UnderBudget_Publishes(t *testing.T) {
	job := sweeperRetryRecord("job-1", "greenhouse", 0)
	ledger := &fakeSweeperLedger{dueJobs: []jobsvc.RetryJobRecord{job}}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// AC-1: no ResetRetryToFetch call — ledger stays at reason='retry' in-flight.
	if len(ledger.resetCalls) != 0 {
		t.Errorf("expected no reset calls (AC-1), got %v", ledger.resetCalls)
	}
	if pub.callCount.Load() != 1 {
		t.Errorf("expected 1 publish, got %d", pub.callCount.Load())
	}
	if len(ledger.escalateCalls) != 0 {
		t.Error("no escalations expected for under-budget job")
	}
	// Verify correct subject format
	want := fmt.Sprintf("jobs.raw.%s", job.Adapter)
	if pub.calls[0].subject != want {
		t.Errorf("subject = %q, want %q", pub.calls[0].subject, want)
	}
}

// TestRetryDueJobs_AtMaxRetries_Escalates asserts that a job with
// retryCount == maxRetries goes directly to EscalateRetryToError (no publish, no reset).
func TestRetryDueJobs_AtMaxRetries_Escalates(t *testing.T) {
	job := sweeperRetryRecord("job-exhausted", "workday", 10) // retryCount == maxRetries
	ledger := &fakeSweeperLedger{dueJobs: []jobsvc.RetryJobRecord{job}}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ledger.escalateCalls) != 1 || ledger.escalateCalls[0] != "job-exhausted" {
		t.Errorf("expected escalate for job-exhausted, got %v", ledger.escalateCalls)
	}
	if pub.callCount.Load() != 0 {
		t.Error("no publish expected for budget-exhausted job")
	}
	// AC-1: no ResetRetryToFetch ever (applies to all paths).
	if len(ledger.resetCalls) != 0 {
		t.Errorf("expected no reset calls (AC-1), got %v", ledger.resetCalls)
	}
}

// TestRetryDueJobs_AboveMaxRetries_Escalates asserts that retryCount > maxRetries
// is treated the same as retryCount == maxRetries.
func TestRetryDueJobs_AboveMaxRetries_Escalates(t *testing.T) {
	job := sweeperRetryRecord("job-over", "lever", 15) // retryCount > maxRetries
	ledger := &fakeSweeperLedger{dueJobs: []jobsvc.RetryJobRecord{job}}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ledger.escalateCalls) != 1 {
		t.Errorf("expected 1 escalation, got %d", len(ledger.escalateCalls))
	}
}

// TestRetryDueJobs_NoReset_PublishesDirectly asserts that RetryDueJobs goes
// directly to PublishWithHeaders without calling ResetRetryToFetch first (AC-1).
// The ledger row stays at reason='retry' while the message is in-flight.
func TestRetryDueJobs_NoReset_PublishesDirectly(t *testing.T) {
	ledger := &fakeSweeperLedger{
		dueJobs: []jobsvc.RetryJobRecord{
			sweeperRetryRecord("job-direct", "greenhouse", 0),
		},
		// resetErr is set but should never be called.
		resetErr: errors.New("reset should not be called"),
	}

	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// AC-1: no reset call, but publish proceeds.
	if len(ledger.resetCalls) != 0 {
		t.Errorf("expected no reset calls (AC-1), got %v", ledger.resetCalls)
	}
	if pub.callCount.Load() != 1 {
		t.Errorf("expected 1 publish (direct, no reset gate), got %d", pub.callCount.Load())
	}
}

// TestRetryDueJobs_PublishFailure_ReQueuesWithDelay asserts that when Publish
// fails, UpdateRetryBatchWithPayload is called to re-queue the job with a delay,
// keeping the same retry count.
func TestRetryDueJobs_PublishFailure_ReQueuesWithDelay(t *testing.T) {
	job := sweeperRetryRecord("job-publish-fail", "greenhouse", 2)
	ledger := &fakeSweeperLedger{dueJobs: []jobsvc.RetryJobRecord{job}}
	pub := &fakeSweeperPublisher{publishErr: errors.New("nats down")}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	before := time.Now()
	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := time.Now()

	// AC-1: no reset call — ledger row stays at reason='retry'.
	if len(ledger.resetCalls) != 0 {
		t.Errorf("expected no reset calls (AC-1), got %v", ledger.resetCalls)
	}
	// Re-queue must have been called
	if len(ledger.retryEntries) != 1 {
		t.Fatalf("expected 1 UpdateRetryBatchWithPayload call, got %d", len(ledger.retryEntries))
	}
	entry := ledger.retryEntries[0]
	if len(entry) != 1 {
		t.Fatalf("expected 1 entry in batch, got %d", len(entry))
	}
	e := entry[0]
	if e.JobID != "job-publish-fail" {
		t.Errorf("job ID = %q, want job-publish-fail", e.JobID)
	}
	// RetryCount preserved (failed publish doesn't count as an attempt)
	if e.RetryCount != 2 {
		t.Errorf("retry count = %d, want 2", e.RetryCount)
	}
	// NextAttemptAt is in the future (~1 minute from now)
	minExpected := before.Add(30 * time.Second)
	if e.NextAttemptAt.Before(minExpected) {
		t.Errorf("next attempt at %v is too soon (before %v), expected ~1 minute delay", e.NextAttemptAt, minExpected)
	}
	if e.NextAttemptAt.After(after.Add(2 * time.Minute)) {
		t.Errorf("next attempt at %v is too far in the future", e.NextAttemptAt)
	}
}

// TestRetryDueJobs_PublishFailure_ReQueueFails_Continues asserts that if the
// re-queue call itself fails, the error is logged but the loop continues
// (per-job error isolation).
func TestRetryDueJobs_PublishFailure_ReQueueFails_Continues(t *testing.T) {
	ledger := &fakeSweeperLedger{
		dueJobs: []jobsvc.RetryJobRecord{
			sweeperRetryRecord("job-requeue-fail", "greenhouse", 1),
		},
		updateRetryErr: errors.New("db unavailable"),
	}
	pub := &fakeSweeperPublisher{publishErr: errors.New("nats down")}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	// Should not error even though re-queue fails
	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRetryDueJobs_EscalateFailure_ContinuesLoop asserts that an escalation
// failure for one job does not abort processing of subsequent jobs.
func TestRetryDueJobs_EscalateFailure_ContinuesLoop(t *testing.T) {
	job1 := sweeperRetryRecord("job-escalate-fail", "greenhouse", 10) // at max
	job2 := sweeperRetryRecord("job-ok", "greenhouse", 0)             // under budget
	ledger := &fakeSweeperLedger{
		dueJobs:     []jobsvc.RetryJobRecord{job1, job2},
		escalateErr: errors.New("escalate db error"),
	}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// job2 should still be processed (published, no reset — AC-1).
	if len(ledger.resetCalls) != 0 {
		t.Errorf("expected no reset calls (AC-1), got %v", ledger.resetCalls)
	}
	if pub.callCount.Load() != 1 {
		t.Errorf("expected 1 publish for job-ok, got %d", pub.callCount.Load())
	}
}

// TestRetryDueJobs_AllUnderBudget_AllPublished verifies that every under-budget
// job in a batch is published via PublishWithHeaders, and that no reset calls are
// made (AC-1: ledger row stays at reason='retry' while in-flight).
func TestRetryDueJobs_AllUnderBudget_AllPublished(t *testing.T) {
	const nJobs = 5
	jobs := make([]jobsvc.RetryJobRecord, nJobs)
	for i := range jobs {
		jobs[i] = sweeperRetryRecord(fmt.Sprintf("job-%d", i), "greenhouse", 0)
	}
	ledger := &fakeSweeperLedger{dueJobs: jobs}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// AC-1: no reset calls at all.
	if len(ledger.resetCalls) != 0 {
		t.Errorf("expected no reset calls (AC-1), got %v", ledger.resetCalls)
	}
	if pub.callCount.Load() != int64(nJobs) {
		t.Errorf("expected %d publishes, got %d", nJobs, pub.callCount.Load())
	}
	// All job IDs appear in the publish call list.
	publishedSubjects := make(map[string]bool, len(pub.calls))
	for _, c := range pub.calls {
		publishedSubjects[c.subject] = true
	}
	for _, j := range jobs {
		want := fmt.Sprintf("jobs.raw.%s", j.Adapter)
		if !publishedSubjects[want] {
			t.Errorf("missing publish call for %s", j.JobID)
		}
	}
}

// TestRetryDueJobs_PublishesCorrectSubject verifies that each job is published
// to "jobs.raw.{adapter}" using the adapter from the ledger row.
func TestRetryDueJobs_PublishesCorrectSubject(t *testing.T) {
	jobs := []jobsvc.RetryJobRecord{
		sweeperRetryRecord("job-a", "greenhouse", 0),
		sweeperRetryRecord("job-b", "workday", 0),
		sweeperRetryRecord("job-c", "lever", 0),
	}
	ledger := &fakeSweeperLedger{dueJobs: jobs}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	subjectSet := make(map[string]bool)
	for _, c := range pub.calls {
		subjectSet[c.subject] = true
	}
	for _, j := range jobs {
		want := fmt.Sprintf("jobs.raw.%s", j.Adapter)
		if !subjectSet[want] {
			t.Errorf("missing publish to %q", want)
		}
	}
}

// TestRetryDueJobs_PublishesOriginalPayload verifies that the payload published
// matches the payload stored in the ledger row.
func TestRetryDueJobs_PublishesOriginalPayload(t *testing.T) {
	job := sweeperRetryRecord("job-payload", "greenhouse", 1)
	ledger := &fakeSweeperLedger{dueJobs: []jobsvc.RetryJobRecord{job}}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.calls) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(pub.calls))
	}
	if string(pub.calls[0].payload) != string(job.Payload) {
		t.Errorf("payload mismatch: got %s, want %s", pub.calls[0].payload, job.Payload)
	}
}

// TestRetryDueJobs_MetricPublished asserts SweeperRetriesPublishedTotal is
// incremented for each successfully published job.
func TestRetryDueJobs_MetricPublished(t *testing.T) {
	before := testutil.ToFloat64(metrics.SweeperRetriesPublishedTotal)

	jobs := []jobsvc.RetryJobRecord{
		sweeperRetryRecord("metric-pub-1", "greenhouse", 0),
		sweeperRetryRecord("metric-pub-2", "greenhouse", 0),
	}
	ledger := &fakeSweeperLedger{dueJobs: jobs}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after := testutil.ToFloat64(metrics.SweeperRetriesPublishedTotal)
	if after-before < 2 {
		t.Errorf("SweeperRetriesPublishedTotal delta = %.0f, want ≥2", after-before)
	}
}

// TestRetryDueJobs_MetricEscalated asserts SweeperRetriesEscalatedTotal is
// incremented when a job is escalated to error.
func TestRetryDueJobs_MetricEscalated(t *testing.T) {
	before := testutil.ToFloat64(metrics.SweeperRetriesEscalatedTotal)

	job := sweeperRetryRecord("metric-esc", "greenhouse", 99) // way over budget
	ledger := &fakeSweeperLedger{dueJobs: []jobsvc.RetryJobRecord{job}}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after := testutil.ToFloat64(metrics.SweeperRetriesEscalatedTotal)
	if after-before < 1 {
		t.Errorf("SweeperRetriesEscalatedTotal delta = %.0f, want ≥1", after-before)
	}
}

// TestRetryDueJobs_MetricFailed asserts SweeperRetriesFailedTotal is incremented
// when a publish fails.
func TestRetryDueJobs_MetricFailed(t *testing.T) {
	before := testutil.ToFloat64(metrics.SweeperRetriesFailedTotal)

	job := sweeperRetryRecord("metric-fail", "greenhouse", 0)
	ledger := &fakeSweeperLedger{dueJobs: []jobsvc.RetryJobRecord{job}}
	pub := &fakeSweeperPublisher{publishErr: errors.New("nats down")}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after := testutil.ToFloat64(metrics.SweeperRetriesFailedTotal)
	if after-before < 1 {
		t.Errorf("SweeperRetriesFailedTotal delta = %.0f, want ≥1", after-before)
	}
}

// TestRetryDueJobs_MaxRetries_DefaultsTo10 asserts that when MaxRetries is
// unset (0), the service defaults to 10.
func TestRetryDueJobs_MaxRetries_DefaultsTo10(t *testing.T) {
	// A job at retryCount=10 should be escalated when maxRetries defaults to 10.
	job := sweeperRetryRecord("job-default-max", "greenhouse", 10)
	ledger := &fakeSweeperLedger{dueJobs: []jobsvc.RetryJobRecord{job}}
	pub := &fakeSweeperPublisher{}
	s := svc.NewSchedulerServiceWithSweeper(&sweeperFakeQueue{}, svc.SchedulerServiceOptions{
		Ledger:     ledger,
		Publisher:  pub,
		MaxRetries: 0, // unset → should default to 10
		Log:        logger.NewNoop(),
	})

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ledger.escalateCalls) != 1 {
		t.Errorf("expected escalation at default maxRetries=10, got escalations=%v", ledger.escalateCalls)
	}
}

// TestRetryDueJobs_PerJobIsolation_PublishFailure asserts that a publish failure
// for one job does not prevent subsequent jobs from being processed (per-job
// error isolation). The failing job is re-queued; the succeeding job is published.
func TestRetryDueJobs_PerJobIsolation_PublishFailure(t *testing.T) {
	job1 := sweeperRetryRecord("job-isolation-fail", "greenhouse", 0)
	job2 := sweeperRetryRecord("job-isolation-ok", "lever", 0)

	// Publish fails for the first call only.
	callIdx := 0
	pub := &conditionalPublisher{
		failOn: func() bool {
			idx := callIdx
			callIdx++
			return idx == 0 // first call fails
		},
	}
	ledger := &fakeSweeperLedger{dueJobs: []jobsvc.RetryJobRecord{job1, job2}}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// job2 must have been published despite job1's publish failure.
	if pub.callCount < 2 {
		t.Errorf("expected at least 2 publish calls (one fail + one ok), got %d", pub.callCount)
	}
	// Re-queue entry for job1 (the failed one).
	if len(ledger.retryEntries) != 1 {
		t.Errorf("expected 1 re-queue entry for failed job, got %d", len(ledger.retryEntries))
	}
}

// conditionalPublisher calls failOn() for each publish to decide whether to fail.
type conditionalPublisher struct {
	failOn    func() bool
	callCount int
}

func (p *conditionalPublisher) Publish(_ context.Context, _ string, _ []byte) error {
	p.callCount++
	if p.failOn() {
		return errors.New("publish failed")
	}
	return nil
}

func (p *conditionalPublisher) PublishWithHeaders(_ context.Context, _ string, _ []byte, _ map[string]string) error {
	p.callCount++
	if p.failOn() {
		return errors.New("publish failed")
	}
	return nil
}

// conditionalSweeperLedger fails ResetRetryToFetch for a specific job ID only.
type conditionalSweeperLedger struct {
	failJobID   string
	successJobs []jobsvc.RetryJobRecord
	resetCalls  []string

	escalateCalls []string
	retryEntries  [][]jobsvc.RetryLedgerEntryWithPayload
}

func (l *conditionalSweeperLedger) HasSeen(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (l *conditionalSweeperLedger) HasSeenBatch(_ context.Context, _ []string) (map[string]bool, error) {
	return nil, nil
}
func (l *conditionalSweeperLedger) MarkSeen(_ context.Context, _, _, _ string) error { return nil }
func (l *conditionalSweeperLedger) MarkSeenBatch(_ context.Context, _ []string, _, _ string) error {
	return nil
}
func (l *conditionalSweeperLedger) UpdateSeenReason(_ context.Context, _ string, _ jobcommon.SeenReason) error {
	return nil
}
func (l *conditionalSweeperLedger) UpdateSeenReasonBatch(_ context.Context, _ []string, _ jobcommon.SeenReason) error {
	return nil
}
func (l *conditionalSweeperLedger) UpdateRetryBatch(_ context.Context, _ []jobsvc.RetryLedgerEntry) error {
	return nil
}
func (l *conditionalSweeperLedger) UpdateRetryBatchWithPayload(_ context.Context, entries []jobsvc.RetryLedgerEntryWithPayload) error {
	l.retryEntries = append(l.retryEntries, entries)
	return nil
}
func (l *conditionalSweeperLedger) FetchDueRetryJobs(_ context.Context, _ int) ([]jobsvc.RetryJobRecord, error) {
	return l.successJobs, nil
}
func (l *conditionalSweeperLedger) FetchDueRetryJobsWithPayload(_ context.Context, _ int) ([]jobsvc.RetryJobRecord, error) {
	return l.successJobs, nil
}
func (l *conditionalSweeperLedger) ResetRetryToFetch(_ context.Context, jobID string) error {
	l.resetCalls = append(l.resetCalls, jobID)
	if jobID == l.failJobID {
		return errors.New("targeted reset failure")
	}
	return nil
}
func (l *conditionalSweeperLedger) EscalateRetryToError(_ context.Context, jobID string) error {
	l.escalateCalls = append(l.escalateCalls, jobID)
	return nil
}
func (l *conditionalSweeperLedger) MarkRemovedBatch(_ context.Context, _ []string) error { return nil }
func (l *conditionalSweeperLedger) GetActiveJobIDsByCompany(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (l *conditionalSweeperLedger) EscalateStuckRetryJobs(_ context.Context, _ time.Duration, _ int) (int64, error) {
	return 0, nil
}
func (l *conditionalSweeperLedger) ReclaimStuckFetchRows(_ context.Context, _ time.Duration, _ int) (int64, error) {
	return 0, nil
}

var _ jobsvc.SeenLedgerStore = (*conditionalSweeperLedger)(nil)

// ── RC-7: EscalateStuckRetryJobs tests ───────────────────────────────────────

// TestEscalateStuckRetryJobs_CallsLedger asserts that the scheduler service's
// EscalateStuckRetryJobs method delegates to the ledger store with the expected
// stuckThreshold and limit, and returns the count.
func TestEscalateStuckRetryJobs_CallsLedger(t *testing.T) {
	ledger := &trackingSweeperLedger{}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	n, err := s.EscalateStuckRetryJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 escalations, got %d", n)
	}
	if !ledger.escalateStuckCalled {
		t.Error("expected EscalateStuckRetryJobs to be called on ledger")
	}
}

// TestEscalateStuckRetryJobs_ReturnsCount asserts the count from the ledger is
// returned to the caller so scheduler.go can log and metric it.
func TestEscalateStuckRetryJobs_ReturnsCount(t *testing.T) {
	ledger := &trackingSweeperLedger{escalateStuckCount: 3}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	n, err := s.EscalateStuckRetryJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 escalations, got %d", n)
	}
}

// TestEscalateStuckRetryJobs_LedgerError_Propagates asserts that a ledger error
// is returned wrapped so the caller can log and metric it.
func TestEscalateStuckRetryJobs_LedgerError_Propagates(t *testing.T) {
	ledger := &trackingSweeperLedger{escalateStuckErr: errors.New("db down")}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	_, err := s.EscalateStuckRetryJobs(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestEscalateStuckRetryJobs_MetricIncremented asserts SweeperStuckEscalatedTotal
// is incremented when rows are escalated.
func TestEscalateStuckRetryJobs_MetricIncremented(t *testing.T) {
	before := testutil.ToFloat64(metrics.SweeperStuckEscalatedTotal)

	ledger := &trackingSweeperLedger{escalateStuckCount: 2}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	n, err := s.EscalateStuckRetryJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2, got %d", n)
	}

	after := testutil.ToFloat64(metrics.SweeperStuckEscalatedTotal)
	if after-before < 2 {
		t.Errorf("SweeperStuckEscalatedTotal delta = %.0f, want ≥2", after-before)
	}
}

// TestEscalateStuckRetryJobs_NoOp_WhenLedgerNil asserts that when no ledger is
// configured, EscalateStuckRetryJobs is a no-op returning 0, nil.
func TestEscalateStuckRetryJobs_NoOp_WhenLedgerNil(t *testing.T) {
	s := svc.NewSchedulerServiceWithSweeper(&sweeperFakeQueue{}, svc.SchedulerServiceOptions{
		Ledger:    nil,
		Publisher: &fakeSweeperPublisher{},
		Log:       logger.NewNoop(),
	})

	n, err := s.EscalateStuckRetryJobs(context.Background())
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

// trackingSweeperLedger extends fakeSweeperLedger with EscalateStuckRetryJobs
// and ReclaimStuckFetchRows tracking.
type trackingSweeperLedger struct {
	fakeSweeperLedger
	escalateStuckCalled bool
	escalateStuckCount  int64
	escalateStuckErr    error

	reclaimFetchCalled    bool
	reclaimFetchThreshold time.Duration
	reclaimFetchLimit     int
	reclaimFetchCount     int64
	reclaimFetchErr       error
}

func (l *trackingSweeperLedger) EscalateStuckRetryJobs(_ context.Context, _ time.Duration, _ int) (int64, error) {
	l.escalateStuckCalled = true
	return l.escalateStuckCount, l.escalateStuckErr
}

func (l *trackingSweeperLedger) ReclaimStuckFetchRows(_ context.Context, threshold time.Duration, limit int) (int64, error) {
	l.reclaimFetchCalled = true
	l.reclaimFetchThreshold = threshold
	l.reclaimFetchLimit = limit
	return l.reclaimFetchCount, l.reclaimFetchErr
}

// ── ReclaimStuckFetchRows service tests ──────────────────────────────────────

// TestReclaimStuckFetchRows_CallsLedger asserts the scheduler service delegates
// to the ledger store with the module-const threshold (1h) and limit (100), and
// returns the count.
func TestReclaimStuckFetchRows_CallsLedger(t *testing.T) {
	ledger := &trackingSweeperLedger{}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	n, err := s.ReclaimStuckFetchRows(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 reclaimed, got %d", n)
	}
	if !ledger.reclaimFetchCalled {
		t.Error("expected ReclaimStuckFetchRows to be called on ledger")
	}
	if ledger.reclaimFetchThreshold != time.Hour {
		t.Errorf("threshold = %v, want 1h", ledger.reclaimFetchThreshold)
	}
	if ledger.reclaimFetchLimit != 100 {
		t.Errorf("limit = %d, want 100", ledger.reclaimFetchLimit)
	}
}

// TestReclaimStuckFetchRows_ReturnsCount asserts the count from the ledger is
// propagated so scheduler.go can log and metric it.
func TestReclaimStuckFetchRows_ReturnsCount(t *testing.T) {
	ledger := &trackingSweeperLedger{reclaimFetchCount: 5}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	n, err := s.ReclaimStuckFetchRows(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 reclaimed, got %d", n)
	}
}

// TestReclaimStuckFetchRows_LedgerError_Propagates asserts a ledger error is
// wrapped and returned with a 0 count.
func TestReclaimStuckFetchRows_LedgerError_Propagates(t *testing.T) {
	ledger := &trackingSweeperLedger{reclaimFetchErr: errors.New("db down")}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	n, err := s.ReclaimStuckFetchRows(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if n != 0 {
		t.Errorf("expected 0 on error, got %d", n)
	}
}

// TestReclaimStuckFetchRows_MetricIncremented asserts
// SchedulerStuckFetchReclaimedTotal is incremented by n when rows are reclaimed.
func TestReclaimStuckFetchRows_MetricIncremented(t *testing.T) {
	before := testutil.ToFloat64(metrics.SchedulerStuckFetchReclaimedTotal)

	ledger := &trackingSweeperLedger{reclaimFetchCount: 4}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	n, err := s.ReclaimStuckFetchRows(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 4 {
		t.Errorf("expected 4, got %d", n)
	}

	after := testutil.ToFloat64(metrics.SchedulerStuckFetchReclaimedTotal)
	if after-before < 4 {
		t.Errorf("SchedulerStuckFetchReclaimedTotal delta = %.0f, want ≥4", after-before)
	}
}

// TestReclaimStuckFetchRows_NoOp_WhenLedgerNil asserts that with no ledger
// configured, ReclaimStuckFetchRows is a no-op returning 0, nil.
func TestReclaimStuckFetchRows_NoOp_WhenLedgerNil(t *testing.T) {
	s := svc.NewSchedulerServiceWithSweeper(&sweeperFakeQueue{}, svc.SchedulerServiceOptions{
		Ledger:    nil,
		Publisher: &fakeSweeperPublisher{},
		Log:       logger.NewNoop(),
	})

	n, err := s.ReclaimStuckFetchRows(context.Background())
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

// ── RC-3: FetchDueRetryJobsWithPayload ────────────────────────────────────────

// TestRetryDueJobs_UsesWithPayloadFetch asserts that RetryDueJobs calls
// FetchDueRetryJobsWithPayload (not FetchDueRetryJobs), verified by the
// tracking ledger which records which method was called (AC-3).
func TestRetryDueJobs_UsesWithPayloadFetch(t *testing.T) {
	ledger := &payloadFetchTrackingLedger{}
	pub := &fakeSweeperPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ledger.withPayloadCalled {
		t.Error("RetryDueJobs must call FetchDueRetryJobsWithPayload (AC-3), but it did not")
	}
	if ledger.withoutPayloadCalled {
		t.Error("RetryDueJobs must NOT call FetchDueRetryJobs (AC-3), but it did")
	}
}

// payloadFetchTrackingLedger records which FetchDueRetryJobs variant was called.
type payloadFetchTrackingLedger struct {
	fakeSweeperLedger
	withPayloadCalled    bool
	withoutPayloadCalled bool
}

func (l *payloadFetchTrackingLedger) FetchDueRetryJobs(_ context.Context, _ int) ([]jobsvc.RetryJobRecord, error) {
	l.withoutPayloadCalled = true
	return nil, nil
}

func (l *payloadFetchTrackingLedger) FetchDueRetryJobsWithPayload(_ context.Context, _ int) ([]jobsvc.RetryJobRecord, error) {
	l.withPayloadCalled = true
	return nil, nil
}

// ── RC-1: X-Retry-Count header stamping ──────────────────────────────────────

// TestRetryDueJobs_StampsRetryCountHeader verifies AC-1: the sweeper calls
// PublishWithHeaders with X-Retry-Count = job.RetryCount + 1 so the filter
// can use the correct monotonic count rather than NATS DeliveryCount.
func TestRetryDueJobs_StampsRetryCountHeader(t *testing.T) {
	job := sweeperRetryRecord("job-header", "greenhouse", 2) // RetryCount=2
	ledger := &fakeSweeperLedger{dueJobs: []jobsvc.RetryJobRecord{job}}
	pub := &headerCapturingPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.calls) != 1 {
		t.Fatalf("expected 1 publish call, got %d", len(pub.calls))
	}
	call := pub.calls[0]
	got := call.headers[js.HeaderRetryCount]
	want := "3" // RetryCount + 1
	if got != want {
		t.Errorf("X-Retry-Count header = %q, want %q (AC-1)", got, want)
	}
}

// TestRetryDueJobs_NoResetBeforePublish verifies AC-1: the sweeper must NOT
// call ResetRetryToFetch before publishing. The ledger row stays at reason='retry'
// while in-flight so EscalateStuckRetryJobs can detect it.
func TestRetryDueJobs_NoResetBeforePublish(t *testing.T) {
	job := sweeperRetryRecord("job-no-reset", "greenhouse", 1)
	ledger := &fakeSweeperLedger{dueJobs: []jobsvc.RetryJobRecord{job}}
	pub := &headerCapturingPublisher{}
	s := makeSweeperService(&sweeperFakeQueue{}, ledger, pub, 10)

	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ledger.resetCalls) != 0 {
		t.Errorf("sweeper must NOT call ResetRetryToFetch (AC-1), got calls: %v", ledger.resetCalls)
	}
}

// headerCapturingPublisher captures PublishWithHeaders calls for header assertions.
type headerCapturingPublisher struct {
	publishErr error
	calls      []headerPublishCall
}

type headerPublishCall struct {
	subject string
	payload []byte
	headers map[string]string
}

func (p *headerCapturingPublisher) Publish(_ context.Context, _ string, _ []byte) error {
	// RC-1: sweeper must use PublishWithHeaders, not Publish.
	return p.publishErr
}

func (p *headerCapturingPublisher) PublishWithHeaders(_ context.Context, subject string, payload []byte, headers map[string]string) error {
	p.calls = append(p.calls, headerPublishCall{subject: subject, payload: payload, headers: headers})
	return p.publishErr
}
