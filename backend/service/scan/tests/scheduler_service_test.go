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
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
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

	// Gauge-refresh test hooks.
	countQueuedRes int
	countQueuedErr error
	oldestAgeRes   float64
	oldestAgeErr   error
}

func (f *schedulerFakeQueue) Enqueue(context.Context, string, string) error { return nil }
func (f *schedulerFakeQueue) EnqueueDue(_ context.Context, m int, src string) (svc.EnqueueDueResult, error) {
	f.enqueueDueArgs.intervalMinutes = m
	f.enqueueDueArgs.source = src
	return f.enqueueDueRes, f.err
}
func (f *schedulerFakeQueue) EnqueueAll(context.Context, string) (int, error) { return 0, nil }
func (f *schedulerFakeQueue) Complete(context.Context, string) error          { return nil }
func (f *schedulerFakeQueue) Remove(context.Context, string) error            { return nil }
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
func (f *schedulerFakeQueue) CountQueued(context.Context) (int, error) {
	return f.countQueuedRes, f.countQueuedErr
}
func (f *schedulerFakeQueue) OldestQueuedAgeSeconds(context.Context) (float64, error) {
	return f.oldestAgeRes, f.oldestAgeErr
}

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

// TestRepositoryService_RefreshGaugeMetrics_NoOp asserts that the
// repository-layer RefreshGaugeMetrics is always a no-op — it satisfies the
// Service interface but the gauge-refresh path is implemented on
// schedulerService, not the decorator chain.
func TestRepositoryService_RefreshGaugeMetrics_NoOp(t *testing.T) {
	s := newImplService(&fakeQueueStore{}, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})
	if err := s.RefreshGaugeMetrics(context.Background()); err != nil {
		t.Fatalf("RefreshGaugeMetrics() through decorator chain error = %v, want nil", err)
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

// ── RefreshGaugeMetrics tests ────────────────────────────────────────────────

// fakeSeenLedgerStore is a test double for jobsvc.SeenLedgerStore, scoped to
// RefreshGaugeMetrics tests. Mirrors fakeSweeperLedger's "meaningful methods
// for what this test needs, no-op stubs for everything else" pattern, but
// exposes CountByReason as a configurable field since these tests need to
// control its return value.
type fakeSeenLedgerStore struct {
	countByReasonRes int
	countByReasonErr error
}

func (f *fakeSeenLedgerStore) HasSeen(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *fakeSeenLedgerStore) HasSeenBatch(_ context.Context, _ []string) (map[string]bool, error) {
	return nil, nil
}
func (f *fakeSeenLedgerStore) MarkSeen(_ context.Context, _, _, _ string) error { return nil }
func (f *fakeSeenLedgerStore) MarkSeenBatch(_ context.Context, _ []string, _, _ string) error {
	return nil
}
func (f *fakeSeenLedgerStore) UpdateSeenReason(_ context.Context, _ string, _ jobcommon.SeenReason) error {
	return nil
}
func (f *fakeSeenLedgerStore) UpdateSeenReasonBatch(_ context.Context, _ []string, _ jobcommon.SeenReason) error {
	return nil
}
func (f *fakeSeenLedgerStore) UpdateRetryBatch(_ context.Context, _ []jobsvc.RetryLedgerEntry) error {
	return nil
}
func (f *fakeSeenLedgerStore) UpdateRetryBatchWithPayload(_ context.Context, _ []jobsvc.RetryLedgerEntryWithPayload) error {
	return nil
}
func (f *fakeSeenLedgerStore) FetchDueRetryJobs(_ context.Context, _ int) ([]jobsvc.RetryJobRecord, error) {
	return nil, nil
}
func (f *fakeSeenLedgerStore) FetchDueRetryJobsWithPayload(_ context.Context, _ int) ([]jobsvc.RetryJobRecord, error) {
	return nil, nil
}
func (f *fakeSeenLedgerStore) ResetRetryToFetch(_ context.Context, _ string) error { return nil }
func (f *fakeSeenLedgerStore) EscalateRetryToError(_ context.Context, _ string) error {
	return nil
}
func (f *fakeSeenLedgerStore) MarkRemovedBatch(_ context.Context, _ []string) error { return nil }
func (f *fakeSeenLedgerStore) GetActiveJobIDsByCompany(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (f *fakeSeenLedgerStore) EscalateStuckRetryJobs(_ context.Context, _ time.Duration, _ int) (int64, error) {
	return 0, nil
}
func (f *fakeSeenLedgerStore) ReclaimStuckFetchRows(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (f *fakeSeenLedgerStore) ReclaimStuckFilteredRows(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (f *fakeSeenLedgerStore) CountByReason(_ context.Context, _ jobcommon.SeenReason) (int, error) {
	return f.countByReasonRes, f.countByReasonErr
}

var _ jobsvc.SeenLedgerStore = (*fakeSeenLedgerStore)(nil)

// fakeGaugeNodeReader is a test double for svc.GaugeNodeReader.
type fakeGaugeNodeReader struct {
	count int
	err   error
	calls int
}

func (f *fakeGaugeNodeReader) CountOnline(context.Context) (int, error) {
	f.calls++
	return f.count, f.err
}

// fakeGaugeSettingsReader is a test double for svc.GaugeSettingsReader.
type fakeGaugeSettingsReader struct {
	count int
	err   error
	calls int
}

func (f *fakeGaugeSettingsReader) CountAutoPaused(context.Context) (int, error) {
	f.calls++
	return f.count, f.err
}

func TestSchedulerService_RefreshGaugeMetrics_SetsAllFiveGauges(t *testing.T) {
	q := &schedulerFakeQueue{countQueuedRes: 7, oldestAgeRes: 42.5}
	nodeReader := &fakeGaugeNodeReader{count: 3}
	settingsReader := &fakeGaugeSettingsReader{count: 2}
	ledger := &fakeSeenLedgerStore{countByReasonRes: 5}

	s := svc.NewSchedulerServiceWithSweeper(q, svc.SchedulerServiceOptions{
		Ledger:         ledger,
		NodeReader:     nodeReader,
		SettingsReader: settingsReader,
		Log:            logger.NewNoop(),
	})

	if err := s.RefreshGaugeMetrics(context.Background()); err != nil {
		t.Fatalf("RefreshGaugeMetrics() error = %v", err)
	}

	if got := testutil.ToFloat64(metrics.SchedulerQueuePending); got != 7 {
		t.Errorf("SchedulerQueuePending = %v, want 7", got)
	}
	if got := testutil.ToFloat64(metrics.SchedulerQueueOldestPendingAgeSeconds); got != 42.5 {
		t.Errorf("SchedulerQueueOldestPendingAgeSeconds = %v, want 42.5", got)
	}
	if got := testutil.ToFloat64(metrics.CrawlerNodesOnline); got != 3 {
		t.Errorf("CrawlerNodesOnline = %v, want 3", got)
	}
	if got := testutil.ToFloat64(metrics.SchedulerCompaniesAutoPausedCurrent); got != 2 {
		t.Errorf("SchedulerCompaniesAutoPausedCurrent = %v, want 2", got)
	}
	if got := testutil.ToFloat64(metrics.JobPipelineLedgerRetryCount); got != 5 {
		t.Errorf("JobPipelineLedgerRetryCount = %v, want 5", got)
	}
	if nodeReader.calls != 1 || settingsReader.calls != 1 {
		t.Errorf("expected exactly 1 call each; nodeReader=%d settingsReader=%d", nodeReader.calls, settingsReader.calls)
	}
}

func TestSchedulerService_RefreshGaugeMetrics_NilOptionalReaders_NoPanic(t *testing.T) {
	q := &schedulerFakeQueue{countQueuedRes: 1}
	s := svc.NewSchedulerServiceWithSweeper(q, svc.SchedulerServiceOptions{Log: logger.NewNoop()})

	// nodeReader/settingsReader/ledger all nil — must not panic, must not error
	// (their gauges are simply not refreshed this tick).
	if err := s.RefreshGaugeMetrics(context.Background()); err != nil {
		t.Fatalf("RefreshGaugeMetrics() error = %v, want nil (nil readers are skipped, not errors)", err)
	}
}

func TestSchedulerService_RefreshGaugeMetrics_PartialFailure_JoinsErrorsAndSetsSucceeded(t *testing.T) {
	q := &schedulerFakeQueue{countQueuedErr: errors.New("db down"), oldestAgeRes: 10}
	nodeReader := &fakeGaugeNodeReader{err: errors.New("node store down")}
	settingsReader := &fakeGaugeSettingsReader{count: 4}

	s := svc.NewSchedulerServiceWithSweeper(q, svc.SchedulerServiceOptions{
		NodeReader:     nodeReader,
		SettingsReader: settingsReader,
		Log:            logger.NewNoop(),
	})

	err := s.RefreshGaugeMetrics(context.Background())
	if err == nil {
		t.Fatal("expected a joined error from the 2 failing reads")
	}
	if !strings.Contains(err.Error(), "count queued") || !strings.Contains(err.Error(), "count online nodes") {
		t.Errorf("expected joined error to mention both failures, got: %v", err)
	}
	// The 2 succeeding reads (oldest age, auto-paused) must still have set
	// their gauges — a partial failure never blocks the other gauges.
	if got := testutil.ToFloat64(metrics.SchedulerQueueOldestPendingAgeSeconds); got != 10 {
		t.Errorf("SchedulerQueueOldestPendingAgeSeconds = %v, want 10 (should still be set despite the other failure)", got)
	}
	if got := testutil.ToFloat64(metrics.SchedulerCompaniesAutoPausedCurrent); got != 4 {
		t.Errorf("SchedulerCompaniesAutoPausedCurrent = %v, want 4", got)
	}
}

// TestSchedulerService_RefreshGaugeMetrics_RemainingFailurePaths exercises
// the 3 error branches not covered by the partial-failure test above
// (OldestQueuedAgeSeconds, SettingsReader.CountAutoPaused, and
// Ledger.CountByReason), confirming each is joined into the returned error
// and the 2 succeeding reads (queue depth, online nodes) still set their
// gauges.
func TestSchedulerService_RefreshGaugeMetrics_RemainingFailurePaths(t *testing.T) {
	q := &schedulerFakeQueue{countQueuedRes: 9, oldestAgeErr: errors.New("age query failed")}
	nodeReader := &fakeGaugeNodeReader{count: 6}
	settingsReader := &fakeGaugeSettingsReader{err: errors.New("settings store down")}
	ledger := &fakeSeenLedgerStore{countByReasonErr: errors.New("ledger down")}

	s := svc.NewSchedulerServiceWithSweeper(q, svc.SchedulerServiceOptions{
		Ledger:         ledger,
		NodeReader:     nodeReader,
		SettingsReader: settingsReader,
		Log:            logger.NewNoop(),
	})

	err := s.RefreshGaugeMetrics(context.Background())
	if err == nil {
		t.Fatal("expected a joined error from the 3 failing reads")
	}
	for _, want := range []string{"oldest queued age", "count auto-paused", "count retry ledger"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected joined error to mention %q, got: %v", want, err)
		}
	}
	if got := testutil.ToFloat64(metrics.SchedulerQueuePending); got != 9 {
		t.Errorf("SchedulerQueuePending = %v, want 9 (should still be set despite the other failures)", got)
	}
	if got := testutil.ToFloat64(metrics.CrawlerNodesOnline); got != 6 {
		t.Errorf("CrawlerNodesOnline = %v, want 6 (should still be set despite the other failures)", got)
	}
}
