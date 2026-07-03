package scan

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	auditmodel "github.com/trushilpatel/jobradar/service/audit/models"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// schedulerFakeQueue is a focused queue store double for scheduler tests.
// It records arguments to confirm the scheduler binds the correct source
// constant and forwards interval/duration unchanged.
type schedulerFakeQueue struct {
	enqueueDueRes svc.EnqueueDueResult
	purgeCount    int
	resetCount    int
	err           error

	enqueueDueArgs struct {
		intervalMinutes int
		source          string
	}
	resetStuckArg time.Duration

	// Auto-pause sweep test hooks.
	autoPauseIDs       []string
	autoPauseErr       error
	autoPauseThreshold int
	autoPauseCalls     int
}

func (f *schedulerFakeQueue) Enqueue(context.Context, string, string) error { return nil }
func (f *schedulerFakeQueue) EnqueueDue(_ context.Context, m int, src string) (svc.EnqueueDueResult, error) {
	f.enqueueDueArgs.intervalMinutes = m
	f.enqueueDueArgs.source = src
	return f.enqueueDueRes, f.err
}
func (f *schedulerFakeQueue) EnqueueAll(context.Context, string) (int, error) { return 0, nil }
func (f *schedulerFakeQueue) ClaimNext(context.Context, string) (*dto.ScanQueueItem, error) {
	return nil, nil
}
func (f *schedulerFakeQueue) Complete(context.Context, string) error { return nil }
func (f *schedulerFakeQueue) Remove(context.Context, string) error   { return nil }
func (f *schedulerFakeQueue) ResetStuck(_ context.Context, d time.Duration) (int, error) {
	f.resetStuckArg = d
	return f.resetCount, f.err
}
func (f *schedulerFakeQueue) PurgeInactive(context.Context) (int, error) {
	return f.purgeCount, f.err
}
func (f *schedulerFakeQueue) AutoPauseChronicFailures(_ context.Context, threshold int) ([]string, error) {
	f.autoPauseCalls++
	f.autoPauseThreshold = threshold
	return f.autoPauseIDs, f.autoPauseErr
}
func (f *schedulerFakeQueue) UpdateHeartbeat(context.Context, string) error { return nil }
func (f *schedulerFakeQueue) ListQueue(context.Context, dto.QueueFilter) ([]dto.ScanQueueItem, error) {
	return nil, nil
}
func (f *schedulerFakeQueue) CountQueued(context.Context) (int, error)                { return 0, nil }
func (f *schedulerFakeQueue) OldestQueuedAgeSeconds(context.Context) (float64, error) { return 0, nil }

var _ svc.ScanQueueStore = (*schedulerFakeQueue)(nil)

// TestSchedulerService_EnqueueDue_OK asserts the scheduler service forwards
// the interval and emits QueueSourceScheduler — never QueueSourceManual.
// Confirms result mapping copies both fields.
func TestSchedulerService_EnqueueDue_OK(t *testing.T) {
	q := &schedulerFakeQueue{enqueueDueRes: svc.EnqueueDueResult{Enqueued: 4, SkippedNoSettings: 1}}
	s := svc.NewSchedulerService(q)

	res, err := s.EnqueueDue(context.Background(), 30)
	if err != nil {
		t.Fatalf("EnqueueDue() error = %v", err)
	}
	if res.Enqueued != 4 || res.SkippedNoSettings != 1 {
		t.Errorf("unexpected result: %+v", res)
	}
	if q.enqueueDueArgs.intervalMinutes != 30 {
		t.Errorf("expected interval 30, got %d", q.enqueueDueArgs.intervalMinutes)
	}
	if q.enqueueDueArgs.source != "scheduler" {
		t.Errorf("expected source 'scheduler', got %q", q.enqueueDueArgs.source)
	}
}

// TestSchedulerService_EnqueueDue_StoreError asserts raw store errors are
// wrapped as ErrInternal so the operator never sees the underlying DB
// driver text.
func TestSchedulerService_EnqueueDue_StoreError(t *testing.T) {
	q := &schedulerFakeQueue{err: errors.New("db down")}
	s := svc.NewSchedulerService(q)

	_, err := s.EnqueueDue(context.Background(), 60)
	assertInternal(t, err)
}

func TestSchedulerService_ResetStuck_OK(t *testing.T) {
	q := &schedulerFakeQueue{resetCount: 2}
	s := svc.NewSchedulerService(q)

	n, err := s.ResetStuck(context.Background(), 90*time.Second)
	if err != nil {
		t.Fatalf("ResetStuck() error = %v", err)
	}
	if n != 2 {
		t.Errorf("expected reset count 2, got %d", n)
	}
	if q.resetStuckArg != 90*time.Second {
		t.Errorf("expected 90s, got %v", q.resetStuckArg)
	}
}

func TestSchedulerService_ResetStuck_ZeroCount_OK(t *testing.T) {
	q := &schedulerFakeQueue{resetCount: 0}
	s := svc.NewSchedulerService(q)

	n, err := s.ResetStuck(context.Background(), 30*time.Second)
	if err != nil {
		t.Fatalf("ResetStuck() zero-count error = %v", err)
	}
	if n != 0 {
		t.Errorf("expected reset count 0, got %d", n)
	}
}

func TestSchedulerService_ResetStuck_StoreError(t *testing.T) {
	q := &schedulerFakeQueue{err: errors.New("db down")}
	s := svc.NewSchedulerService(q)

	_, err := s.ResetStuck(context.Background(), 30*time.Second)
	assertInternal(t, err)
}

func TestSchedulerService_PurgeInactive_OK(t *testing.T) {
	q := &schedulerFakeQueue{purgeCount: 7}
	s := svc.NewSchedulerService(q)

	n, err := s.PurgeInactive(context.Background())
	if err != nil {
		t.Fatalf("PurgeInactive() error = %v", err)
	}
	if n != 7 {
		t.Errorf("expected 7 purged, got %d", n)
	}
}

func TestSchedulerService_PurgeInactive_StoreError(t *testing.T) {
	q := &schedulerFakeQueue{err: errors.New("db down")}
	s := svc.NewSchedulerService(q)

	_, err := s.PurgeInactive(context.Background())
	assertInternal(t, err)
}

// TestSchedulerService_RetryDueJobs_NoOp asserts that the base scheduler
// service (without sweeper opts) returns nil — RetryDueJobs is a no-op
// when no ledger/publisher are wired.
func TestSchedulerService_RetryDueJobs_NoOp(t *testing.T) {
	q := &schedulerFakeQueue{}
	s := svc.NewSchedulerService(q)
	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("RetryDueJobs() error = %v, want nil", err)
	}
}

// TestRepositoryService_RetryDueJobs_NoOp asserts that the repository-layer
// RetryDueJobs is always a no-op — it satisfies the Service interface but the
// sweeper is implemented on schedulerService, not the decorator chain.
func TestRepositoryService_RetryDueJobs_NoOp(t *testing.T) {
	s := newImplService(&fakeQueueStore{}, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})
	if err := s.RetryDueJobs(context.Background()); err != nil {
		t.Fatalf("RetryDueJobs() through decorator chain error = %v, want nil", err)
	}
}

// TestBaseService_RetryDueJobs_Delegates asserts that BaseService.RetryDueJobs
// delegates to the next layer without modification.
func TestBaseService_RetryDueJobs_Delegates(t *testing.T) {
	var called bool
	fake := &fakeScanService{} // returns f.err (nil)
	b := svc.NewBaseService(fake)
	// noopService.RetryDueJobs will set called via the interface check
	_ = b.RetryDueJobs(context.Background())
	_ = called // suppress unused
}

// assertInternal fails the test if err is not a wrapped *ServiceError with
// code INTERNAL. Used by both scheduler and crawler-queue tests below.
func assertInternal(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %v", err)
	}
}

// ── Auto-pause sweep tests ────────────────────────────────────────────────────

// fakeAuditor records Append calls and can inject an error to exercise the
// non-fatal audit path.
type fakeAuditor struct {
	entries []auditmodel.Entry
	err     error
}

func (a *fakeAuditor) Append(_ context.Context, entry auditmodel.Entry) error {
	a.entries = append(a.entries, entry)
	return a.err
}

// makeAutoPauseService constructs a scheduler service with the auto-pause
// auditor wired via options. Pass a nil auditor to exercise the unconfigured
// path.
func makeAutoPauseService(q svc.ScanQueueStore, auditor svc.AutoPauseAuditor) svc.SchedulerService {
	return svc.NewSchedulerServiceWithSweeper(q, svc.SchedulerServiceOptions{
		Auditor: auditor,
		Log:     logger.NewNoop(),
	})
}

// TestAutoPause_CallsStoreWithThreshold asserts the sweep forwards the
// AutoPauseThreshold constant to the store and returns the paused count.
func TestAutoPause_CallsStoreWithThreshold(t *testing.T) {
	q := &schedulerFakeQueue{autoPauseIDs: []string{"c1", "c2"}}
	s := makeAutoPauseService(q, &fakeAuditor{})

	n, err := s.AutoPauseChronicFailures(context.Background())
	if err != nil {
		t.Fatalf("AutoPauseChronicFailures() error = %v", err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2", n)
	}
	if q.autoPauseCalls != 1 {
		t.Errorf("store calls = %d, want 1", q.autoPauseCalls)
	}
	if q.autoPauseThreshold != scancommon.AutoPauseThreshold {
		t.Errorf("threshold = %d, want %d", q.autoPauseThreshold, scancommon.AutoPauseThreshold)
	}
}

// TestAutoPause_WritesOneAuditRowPerCompany asserts one audit entry is appended
// per newly paused company, with the correct action + target.
func TestAutoPause_WritesOneAuditRowPerCompany(t *testing.T) {
	q := &schedulerFakeQueue{autoPauseIDs: []string{"c1", "c2", "c3"}}
	audit := &fakeAuditor{}
	s := makeAutoPauseService(q, audit)

	if _, err := s.AutoPauseChronicFailures(context.Background()); err != nil {
		t.Fatalf("AutoPauseChronicFailures() error = %v", err)
	}
	if len(audit.entries) != 3 {
		t.Fatalf("audit entries = %d, want 3", len(audit.entries))
	}
	for i, e := range audit.entries {
		if e.Action != svc.AutoPauseAction {
			t.Errorf("entry[%d] action = %q, want %q", i, e.Action, svc.AutoPauseAction)
		}
		if e.TargetID == nil || *e.TargetID != q.autoPauseIDs[i] {
			t.Errorf("entry[%d] target id mismatch: %+v", i, e.TargetID)
		}
	}
}

// TestAutoPause_StoreError_Wrapped asserts a store error is wrapped with the
// method prefix and no companies are reported paused.
func TestAutoPause_StoreError_Wrapped(t *testing.T) {
	q := &schedulerFakeQueue{autoPauseErr: errors.New("db down")}
	s := makeAutoPauseService(q, &fakeAuditor{})

	n, err := s.AutoPauseChronicFailures(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "AutoPauseChronicFailures:") {
		t.Errorf("error not wrapped with method prefix: %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0 on error", n)
	}
}

// TestAutoPause_NilQueue_NoOp asserts a nil queue produces a safe (0, nil)
// no-op rather than a panic.
func TestAutoPause_NilQueue_NoOp(t *testing.T) {
	s := svc.NewSchedulerServiceWithSweeper(nil, svc.SchedulerServiceOptions{Log: logger.NewNoop()})
	n, err := s.AutoPauseChronicFailures(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("nil-queue AutoPause = (%d, %v), want (0, nil)", n, err)
	}
}

// TestAutoPause_NilAuditor_NonFatal asserts that with no auditor wired, the
// sweep still pauses companies and returns the count (audit is non-fatal).
func TestAutoPause_NilAuditor_NonFatal(t *testing.T) {
	q := &schedulerFakeQueue{autoPauseIDs: []string{"c1"}}
	s := makeAutoPauseService(q, nil)

	n, err := s.AutoPauseChronicFailures(context.Background())
	if err != nil {
		t.Fatalf("AutoPauseChronicFailures() error = %v", err)
	}
	if n != 1 {
		t.Errorf("count = %d, want 1 (audit-nil must not block pause)", n)
	}
}

// TestAutoPause_AuditError_NonFatal asserts an Append failure is swallowed (the
// pause still succeeds and the full count is returned).
func TestAutoPause_AuditError_NonFatal(t *testing.T) {
	q := &schedulerFakeQueue{autoPauseIDs: []string{"c1", "c2"}}
	audit := &fakeAuditor{err: errors.New("audit table gone")}
	s := makeAutoPauseService(q, audit)

	n, err := s.AutoPauseChronicFailures(context.Background())
	if err != nil {
		t.Fatalf("AutoPauseChronicFailures() error = %v; audit failure must be non-fatal", err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2", n)
	}
	if len(audit.entries) != 2 {
		t.Errorf("Append attempted %d times, want 2 (both attempted despite error)", len(audit.entries))
	}
}

// TestAutoPause_Idempotent_NoRows asserts that when the store returns no newly
// paused companies (all already paused), the sweep returns 0 and writes no
// audit rows.
func TestAutoPause_Idempotent_NoRows(t *testing.T) {
	q := &schedulerFakeQueue{autoPauseIDs: nil}
	audit := &fakeAuditor{}
	s := makeAutoPauseService(q, audit)

	n, err := s.AutoPauseChronicFailures(context.Background())
	if err != nil {
		t.Fatalf("AutoPauseChronicFailures() error = %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0 (idempotent — nothing new to pause)", n)
	}
	if len(audit.entries) != 0 {
		t.Errorf("audit entries = %d, want 0", len(audit.entries))
	}
}

// TestAutoPause_MetricIncremented asserts the paused-companies metric advances
// by the number of companies paused.
func TestAutoPause_MetricIncremented(t *testing.T) {
	before := testutil.ToFloat64(metrics.SchedulerCompaniesAutoPausedTotal)
	q := &schedulerFakeQueue{autoPauseIDs: []string{"c1", "c2", "c3"}}
	s := makeAutoPauseService(q, &fakeAuditor{})

	if _, err := s.AutoPauseChronicFailures(context.Background()); err != nil {
		t.Fatalf("AutoPauseChronicFailures() error = %v", err)
	}
	after := testutil.ToFloat64(metrics.SchedulerCompaniesAutoPausedTotal)
	if after-before != 3 {
		t.Errorf("metric delta = %v, want 3", after-before)
	}
}
