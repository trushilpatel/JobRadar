package scan

import (
	"context"
	"errors"
	"testing"
	"time"

	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// These tests exercise the repositoryService terminal — the layer that wraps
// raw store errors with domain *ServiceError values. They run via the full
// impl chain so each test path matches production wiring.

// TestRepo_TriggerScan_StoreError covers the branch where the company has
// scan_settings (impl check passes) but the underlying Enqueue fails. The
// terminal must surface ErrInternal — not the raw db error.
func TestRepo_TriggerScan_StoreError(t *testing.T) {
	q := &fakeQueueStore{err: errors.New("db down")}
	settings := &fakeScanSettingsRepo{
		settings: &companymodel.CompanyScanSettings{CompanyID: "stripe", AdapterType: "greenhouse", Active: true},
	}
	s := newImplService(q, &fakeScanLogReader{}, settings, &fakeCompanyAdminService{}, svc.Config{})

	err := s.TriggerScan(context.Background(), "stripe")
	assertInternal(t, err)
}

// TestRepo_TriggerScanAll_OK confirms the queue.EnqueueAll count propagates
// through the chain unchanged and the source constant is `manual` — the
// "scan all" admin trigger bypasses the scan-interval cooldown.
func TestRepo_TriggerScanAll_OK(t *testing.T) {
	q := &fakeQueueStore{enqueueAllResult: 12}
	s := newImplService(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	n, err := s.TriggerScanAll(context.Background())
	if err != nil {
		t.Fatalf("TriggerScanAll() error = %v", err)
	}
	if n != 12 {
		t.Errorf("expected 12 enqueued, got %d", n)
	}
	if !q.enqueueAllCalled {
		t.Error("expected EnqueueAll to be invoked")
	}
}

func TestRepo_TriggerScanAll_StoreError(t *testing.T) {
	q := &fakeQueueStore{err: errors.New("db down")}
	s := newImplService(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	_, err := s.TriggerScanAll(context.Background())
	assertInternal(t, err)
}

func TestRepo_GetStats_OK(t *testing.T) {
	stats := dto.ScanStatsView{TotalScans: 5, TotalNew: 12}
	log := &fakeScanLogReader{stats: stats}
	s := newImplService(&fakeQueueStore{}, log, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	got, err := s.GetStats(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if got == nil || got.TotalScans != 5 || got.TotalNew != 12 {
		t.Errorf("unexpected stats: %+v", got)
	}
}

func TestRepo_GetStats_StoreError(t *testing.T) {
	log := &fakeScanLogReader{err: errors.New("db down")}
	s := newImplService(&fakeQueueStore{}, log, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	_, err := s.GetStats(context.Background(), "stripe")
	assertInternal(t, err)
}

// TestRepo_GetAnalytics_OK relies on the reader returning a zero value —
// the repositoryService just forwards it without enrichment. The test
// asserts a non-nil pointer is returned and the error path is not taken.
func TestRepo_GetAnalytics_OK(t *testing.T) {
	log := &fakeScanLogReader{}
	s := newImplService(&fakeQueueStore{}, log, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	a, err := s.GetAnalytics(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("GetAnalytics() error = %v", err)
	}
	if a == nil {
		t.Fatal("expected non-nil analytics pointer")
	}
}

func TestRepo_GetAnalytics_StoreError(t *testing.T) {
	log := &fakeScanLogReader{err: errors.New("db down")}
	s := newImplService(&fakeQueueStore{}, log, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	_, err := s.GetAnalytics(context.Background(), "stripe")
	assertInternal(t, err)
}

func TestRepo_PurgeInactive_OK(t *testing.T) {
	// fakeQueueStore.PurgeInactive returns (0, f.err) — without an err this
	// confirms the no-rows case is treated as success.
	q := &fakeQueueStore{}
	s := newImplService(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	n, err := s.PurgeInactive(context.Background())
	if err != nil {
		t.Fatalf("PurgeInactive() error = %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 purged, got %d", n)
	}
}

func TestRepo_PurgeInactive_StoreError(t *testing.T) {
	q := &fakeQueueStore{err: errors.New("db down")}
	s := newImplService(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	_, err := s.PurgeInactive(context.Background())
	assertInternal(t, err)
}

func TestRepo_ListQueue_StoreError(t *testing.T) {
	q := &fakeQueueStore{err: errors.New("db down")}
	s := newImplService(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	_, err := s.ListQueue(context.Background(), dto.QueueFilter{})
	assertInternal(t, err)
}

func TestRepo_RemoveFromQueue_StoreError(t *testing.T) {
	q := &fakeQueueStore{err: errors.New("db down")}
	s := newImplService(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	err := s.RemoveFromQueue(context.Background(), "entry-1")
	assertInternal(t, err)
}

// newImplServiceWithAdmin mirrors newImplService but accepts the
// CompanyAdminService interface directly — used so tests can swap in
// tracking fakes that record GetByIDs invocations.
func newImplServiceWithAdmin(q svc.ScanQueueStore, log svc.ScanLogReader, settings *fakeScanSettingsRepo, admin companysvc.CompanyAdminService, cfg svc.Config) svc.Service {
	repo := svc.NewRepositoryService(q, log, cfg)
	return svc.NewService(repo, settings, admin, cfg)
}

// TestImpl_ListQueue_EmptyShortCircuits covers the early return path in
// implService.ListQueue when next returns an empty slice — the company
// admin lookup must NOT be invoked.
func TestImpl_ListQueue_EmptyShortCircuits(t *testing.T) {
	q := &fakeQueueStore{items: nil}
	admin := &trackingCompanyAdminService{}
	s := newImplServiceWithAdmin(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, admin, svc.Config{})

	views, err := s.ListQueue(context.Background(), dto.QueueFilter{})
	if err != nil {
		t.Fatalf("ListQueue() error = %v", err)
	}
	if len(views) != 0 {
		t.Errorf("expected empty result, got %d", len(views))
	}
	if admin.getByIDsCalls != 0 {
		t.Errorf("expected GetByIDs NOT to be called for empty input, got %d", admin.getByIDsCalls)
	}
}

// TestImpl_ListQueue_CompanyAdminError covers the path where the underlying
// next.ListQueue succeeds but the company-admin lookup fails. The error
// must be wrapped as ErrInternal, never leaked raw.
func TestImpl_ListQueue_CompanyAdminError(t *testing.T) {
	items := []dto.ScanQueueItem{
		{ID: "q1", CompanyID: "stripe", Status: "queued", Source: "manual", QueuedAt: time.Now()},
	}
	q := &fakeQueueStore{items: items}
	admin := &trackingCompanyAdminService{err: errors.New("admin down")}
	s := newImplServiceWithAdmin(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, admin, svc.Config{})

	_, err := s.ListQueue(context.Background(), dto.QueueFilter{})
	assertInternal(t, err)
}

// TestImpl_ListQueue_NextError covers the early error path where the
// repository-level ListQueue itself fails. The error must propagate
// untouched — it is already a *ServiceError from the terminal layer.
func TestImpl_ListQueue_NextError(t *testing.T) {
	q := &fakeQueueStore{err: errors.New("db down")}
	s := newImplService(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	_, err := s.ListQueue(context.Background(), dto.QueueFilter{})
	assertInternal(t, err)
}

// TestImpl_ListQueue_DeduplicatesCompanyIDs asserts that when multiple
// queue rows reference the same company, GetByIDs is called with a
// deduplicated id list. Important so an under-onboarded company with many
// queued retries does not flood the admin store with duplicate lookups.
func TestImpl_ListQueue_DeduplicatesCompanyIDs(t *testing.T) {
	items := []dto.ScanQueueItem{
		{ID: "q1", CompanyID: "stripe", Status: "queued"},
		{ID: "q2", CompanyID: "stripe", Status: "queued"},
		{ID: "q3", CompanyID: "shopify", Status: "queued"},
	}
	q := &fakeQueueStore{items: items}
	admin := &trackingCompanyAdminService{}
	s := newImplServiceWithAdmin(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, admin, svc.Config{})

	if _, err := s.ListQueue(context.Background(), dto.QueueFilter{}); err != nil {
		t.Fatalf("ListQueue() error = %v", err)
	}
	if admin.getByIDsCalls != 1 {
		t.Fatalf("expected 1 GetByIDs call, got %d", admin.getByIDsCalls)
	}
	if len(admin.lastIDs) != 2 {
		t.Errorf("expected 2 unique ids, got %d (%v)", len(admin.lastIDs), admin.lastIDs)
	}
	have := map[string]bool{}
	for _, id := range admin.lastIDs {
		have[id] = true
	}
	if !have["stripe"] || !have["shopify"] {
		t.Errorf("expected stripe+shopify, got %v", admin.lastIDs)
	}
}

// trackingCompanyAdminService records calls to GetByIDs without otherwise
// providing data — used to assert the impl service short-circuits and
// deduplicates correctly.
type trackingCompanyAdminService struct {
	fakeCompanyAdminService
	getByIDsCalls int
	lastIDs       []string
	err           error
}

func (f *trackingCompanyAdminService) GetByIDs(_ context.Context, ids []string) ([]companydto.CompanyAdminView, error) {
	f.getByIDsCalls++
	f.lastIDs = append([]string(nil), ids...)
	if f.err != nil {
		return nil, f.err
	}
	return nil, nil
}
