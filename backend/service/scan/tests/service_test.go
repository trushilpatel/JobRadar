package scan

import (
	"context"
	"errors"
	"testing"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// fakeQueueStore is a test double for ScanQueueStore.
type fakeQueueStore struct {
	items                 []dto.ScanQueueItem
	err                   error
	enqueueCalled         bool
	removeCalled          bool
	enqueueDueResult      svc.EnqueueDueResult
	enqueueDueCalled      bool
	enqueueAllResult      int
	enqueueAllCalled      bool
	resetStuckCalled      bool
	enqueueDueArgInterval int
}

func (f *fakeQueueStore) Enqueue(_ context.Context, _, _ string) error {
	f.enqueueCalled = true
	return f.err
}
func (f *fakeQueueStore) EnqueueDue(_ context.Context, intervalMinutes int, _ string) (svc.EnqueueDueResult, error) {
	f.enqueueDueCalled = true
	f.enqueueDueArgInterval = intervalMinutes
	return f.enqueueDueResult, f.err
}
func (f *fakeQueueStore) EnqueueAll(_ context.Context, _ string) (int, error) {
	f.enqueueAllCalled = true
	return f.enqueueAllResult, f.err
}
func (f *fakeQueueStore) ClaimNext(_ context.Context, _ string) (*dto.ScanQueueItem, error) {
	return nil, f.err
}
func (f *fakeQueueStore) Complete(_ context.Context, _ string) error { return f.err }
func (f *fakeQueueStore) Remove(_ context.Context, _ string) error {
	f.removeCalled = true
	return f.err
}
func (f *fakeQueueStore) ResetStuck(_ context.Context, _ time.Duration) (int, error) {
	f.resetStuckCalled = true
	return 0, f.err
}
func (f *fakeQueueStore) PurgeInactive(_ context.Context) (int, error)      { return 0, f.err }
func (f *fakeQueueStore) UpdateHeartbeat(_ context.Context, _ string) error { return f.err }
func (f *fakeQueueStore) ListQueue(_ context.Context, filter dto.QueueFilter) ([]dto.ScanQueueItem, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []dto.ScanQueueItem
	for _, item := range f.items {
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		if filter.Source != "" && item.Source != filter.Source {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// fakeScanLogReader is a test double for ScanLogReader.
type fakeScanLogReader struct {
	stats dto.ScanStatsView
	err   error
}

func (f *fakeScanLogReader) GetScanStats(_ context.Context, _ string) (dto.ScanStatsView, error) {
	return f.stats, f.err
}
func (f *fakeScanLogReader) GetScanHistories(_ context.Context, _ []string) (map[string][]bool, error) {
	return nil, f.err
}
func (f *fakeScanLogReader) GetJobsScannedToday(_ context.Context) (int64, error) { return 0, f.err }
func (f *fakeScanLogReader) GetScansByDay(_ context.Context, _ int) ([]dto.DailyScanCount, error) {
	return nil, f.err
}
func (f *fakeScanLogReader) GetCompanyAnalytics(_ context.Context, _ string) (dto.CompanyAnalytics, error) {
	return dto.CompanyAnalytics{}, f.err
}

// fakeScanSettingsRepo is a test double for companysvc.ScanSettingsRepository.
// Only Get is exercised by the impl service today; other methods are stubs.
type fakeScanSettingsRepo struct {
	settings *companymodel.CompanyScanSettings
	getErr   error
}

func (f *fakeScanSettingsRepo) Get(_ context.Context, _ string) (*companymodel.CompanyScanSettings, error) {
	return f.settings, f.getErr
}
func (f *fakeScanSettingsRepo) GetMissing(_ context.Context) ([]string, error) { return nil, nil }
func (f *fakeScanSettingsRepo) Create(_ context.Context, _ companymodel.CompanyScanSettings) error {
	return nil
}
func (f *fakeScanSettingsRepo) Update(_ context.Context, _ string, _ companydto.CompanyScanSettingsInput) error {
	return nil
}
func (f *fakeScanSettingsRepo) UpdateScanState(_ context.Context, _ string, _ companydto.CompanyScanStateUpdate) error {
	return nil
}
func (f *fakeScanSettingsRepo) Delete(_ context.Context, _ string) error { return nil }

// fakeCompanyAdminService is a test double for companysvc.CompanyAdminService.
// Only GetByIDs is exercised by the impl service today; other methods are stubs.
type fakeCompanyAdminService struct {
	views []companydto.CompanyAdminView
	err   error
}

func (f *fakeCompanyAdminService) List(_ context.Context, _ companydto.CompanyFilter, _ int) (companydto.CompanyAdminListResult, error) {
	return companydto.CompanyAdminListResult{Total: int64(len(f.views))}, f.err
}
func (f *fakeCompanyAdminService) Get(_ context.Context, id string) (*companydto.CompanyAdminView, error) {
	for i := range f.views {
		if f.views[i].ID == id {
			return &f.views[i], f.err
		}
	}
	return nil, apperr.ErrNotFound.WithMessage("not found")
}
func (f *fakeCompanyAdminService) Create(_ context.Context, _ companydto.CompanyPublicInput, _ companydto.CompanyScanSettingsInput) (*companydto.CompanyAdminView, error) {
	return nil, f.err
}
func (f *fakeCompanyAdminService) UpdatePublic(_ context.Context, _ string, _ companydto.CompanyPublicInput) error {
	return f.err
}
func (f *fakeCompanyAdminService) UpdateScanSettings(_ context.Context, _ string, _ companydto.CompanyScanSettingsInput) error {
	return f.err
}
func (f *fakeCompanyAdminService) Delete(_ context.Context, _ string) error     { return f.err }
func (f *fakeCompanyAdminService) SetAllActive(_ context.Context, _ bool) error { return f.err }
func (f *fakeCompanyAdminService) GetAll(_ context.Context) ([]companydto.CompanyAdminView, error) {
	return f.views, f.err
}
func (f *fakeCompanyAdminService) GetByIDs(_ context.Context, ids []string) ([]companydto.CompanyAdminView, error) {
	if f.err != nil {
		return nil, f.err
	}
	idx := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idx[id] = struct{}{}
	}
	var out []companydto.CompanyAdminView
	for i := range f.views {
		if _, ok := idx[f.views[i].ID]; ok {
			out = append(out, f.views[i])
		}
	}
	return out, nil
}

// newImplService builds the full impl chain: repositoryService → implService.
// db and atsSvc are nil for tests that don't exercise GetEffectiveScanConfig
// or UpdateScanIntervalOverride — those are covered in scan_config_service_test.go.
func newImplService(q *fakeQueueStore, log *fakeScanLogReader, settings *fakeScanSettingsRepo, admin *fakeCompanyAdminService, cfg svc.Config) svc.Service {
	repo := svc.NewRepositoryService(q, log, cfg, nil, nil)
	return svc.NewService(repo, settings, admin, cfg)
}

func TestTriggerScan_OK(t *testing.T) {
	q := &fakeQueueStore{}
	settings := &fakeScanSettingsRepo{
		settings: &companymodel.CompanyScanSettings{CompanyID: "stripe", AdapterType: "greenhouse", Active: true},
	}
	service := newImplService(q, &fakeScanLogReader{}, settings, &fakeCompanyAdminService{}, svc.Config{})

	if err := service.TriggerScan(context.Background(), "stripe"); err != nil {
		t.Fatalf("TriggerScan() error = %v", err)
	}
	if !q.enqueueCalled {
		t.Error("expected Enqueue to be called")
	}
}

// TestTriggerScan_NoScanSettings asserts the impl service returns a clear
// BAD_REQUEST when the company has no scan_settings row — admins onboard via
// /admin/companies/{id}/scan-settings before triggering a manual scan.
func TestTriggerScan_NoScanSettings(t *testing.T) {
	q := &fakeQueueStore{}
	settings := &fakeScanSettingsRepo{
		getErr: apperr.ErrNotFound.WithMessage("scan settings not found"),
	}
	service := newImplService(q, &fakeScanLogReader{}, settings, &fakeCompanyAdminService{}, svc.Config{})

	err := service.TriggerScan(context.Background(), "stripe")
	if err == nil {
		t.Fatal("expected error when scan_settings missing")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "BAD_REQUEST" {
		t.Errorf("expected BAD_REQUEST, got %v", err)
	}
	if q.enqueueCalled {
		t.Error("expected Enqueue NOT to be called when settings missing")
	}
}

func TestTriggerScan_SettingsLookupInternalError(t *testing.T) {
	q := &fakeQueueStore{}
	settings := &fakeScanSettingsRepo{
		getErr: apperr.ErrInternal.WithMessage("db down"),
	}
	service := newImplService(q, &fakeScanLogReader{}, settings, &fakeCompanyAdminService{}, svc.Config{})

	err := service.TriggerScan(context.Background(), "stripe")
	if err == nil {
		t.Fatal("expected error")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %v", err)
	}
}

func TestListQueue_EnrichesWithCompanyData(t *testing.T) {
	lastScan := time.Now().Add(-1 * time.Hour)
	items := []dto.ScanQueueItem{
		{ID: "q1", CompanyID: "stripe", Status: "queued", Source: "manual", QueuedAt: time.Now()},
	}
	views := []companydto.CompanyAdminView{
		{
			CompanyAdminCommon: companydto.CompanyAdminCommon{Company: companymodel.Company{ID: "stripe", Name: "Stripe"}},
			ScanSettings: &companydto.CompanyScanSettingsView{
				CompanyScanSettingsListView: companydto.CompanyScanSettingsListView{
					CompanyID:   "stripe",
					AdapterType: "greenhouse",
					Active:      true,
					LastScanAt:  &lastScan,
				},
			},
		},
	}

	service := newImplService(
		&fakeQueueStore{items: items},
		&fakeScanLogReader{},
		&fakeScanSettingsRepo{},
		&fakeCompanyAdminService{views: views},
		svc.Config{ScanIntervalMinutes: 60},
	)

	result, err := service.ListQueue(context.Background(), dto.QueueFilter{})
	if err != nil {
		t.Fatalf("ListQueue() error = %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	if result[0].CompanyName != "Stripe" {
		t.Errorf("expected CompanyName 'Stripe', got %q", result[0].CompanyName)
	}
	if result[0].AdapterType != "greenhouse" {
		t.Errorf("expected AdapterType 'greenhouse', got %q", result[0].AdapterType)
	}
	if result[0].NextScanAt == nil {
		t.Error("expected NextScanAt to be computed")
	}
}

// TestListQueue_CompanyMissingScanSettings asserts ListQueue still returns the
// queue row even when its company has no scan_settings — the row is enriched
// with the company name only; AdapterType / NextScanAt stay zero.
func TestListQueue_CompanyMissingScanSettings(t *testing.T) {
	items := []dto.ScanQueueItem{
		{ID: "q1", CompanyID: "stripe", Status: "queued", Source: "manual"},
	}
	views := []companydto.CompanyAdminView{
		{CompanyAdminCommon: companydto.CompanyAdminCommon{Company: companymodel.Company{ID: "stripe", Name: "Stripe"}}, ScanSettings: nil},
	}

	service := newImplService(
		&fakeQueueStore{items: items},
		&fakeScanLogReader{},
		&fakeScanSettingsRepo{},
		&fakeCompanyAdminService{views: views},
		svc.Config{ScanIntervalMinutes: 60},
	)

	result, err := service.ListQueue(context.Background(), dto.QueueFilter{})
	if err != nil {
		t.Fatalf("ListQueue() error = %v", err)
	}
	if result[0].CompanyName != "Stripe" {
		t.Errorf("expected company name, got %q", result[0].CompanyName)
	}
	if result[0].AdapterType != "" {
		t.Errorf("expected empty adapter, got %q", result[0].AdapterType)
	}
	if result[0].NextScanAt != nil {
		t.Errorf("expected nil NextScanAt, got %v", result[0].NextScanAt)
	}
}

func TestListQueue_FiltersStatus(t *testing.T) {
	items := []dto.ScanQueueItem{
		{ID: "q1", CompanyID: "c1", Status: "queued", Source: "manual"},
		{ID: "q2", CompanyID: "c2", Status: "processing", Source: "manual"},
	}
	service := newImplService(
		&fakeQueueStore{items: items},
		&fakeScanLogReader{},
		&fakeScanSettingsRepo{},
		&fakeCompanyAdminService{},
		svc.Config{},
	)

	result, err := service.ListQueue(context.Background(), dto.QueueFilter{Status: "queued"})
	if err != nil {
		t.Fatalf("ListQueue() error = %v", err)
	}
	if len(result) != 1 || result[0].ID != "q1" {
		t.Errorf("expected only queued item q1, got %+v", result)
	}
}

func TestRemoveFromQueue(t *testing.T) {
	q := &fakeQueueStore{}
	service := newImplService(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	if err := service.RemoveFromQueue(context.Background(), "q1"); err != nil {
		t.Fatalf("RemoveFromQueue() error = %v", err)
	}
	if !q.removeCalled {
		t.Error("expected Remove to be called")
	}
}

func TestEnqueueDue_EnqueuesAll(t *testing.T) {
	q := &fakeQueueStore{enqueueDueResult: svc.EnqueueDueResult{Enqueued: 3, SkippedNoSettings: 2}}
	service := newImplService(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	res, err := service.EnqueueDue(context.Background(), 1440)
	if err != nil {
		t.Fatalf("EnqueueDue() error = %v", err)
	}
	if res.Enqueued != 3 {
		t.Errorf("expected Enqueued 3, got %d", res.Enqueued)
	}
	if res.SkippedNoSettings != 2 {
		t.Errorf("expected SkippedNoSettings 2, got %d", res.SkippedNoSettings)
	}
	if !q.enqueueDueCalled {
		t.Error("expected EnqueueDue to be called on store")
	}
}

func TestEnqueueDue_NoDueCompanies(t *testing.T) {
	q := &fakeQueueStore{enqueueDueResult: svc.EnqueueDueResult{}}
	service := newImplService(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	res, err := service.EnqueueDue(context.Background(), 1440)
	if err != nil {
		t.Fatalf("EnqueueDue() error = %v", err)
	}
	if res.Enqueued != 0 {
		t.Errorf("expected 0 enqueued when none due, got %d", res.Enqueued)
	}
}

func TestEnqueueDue_StoreError(t *testing.T) {
	q := &fakeQueueStore{err: errors.New("db down")}
	service := newImplService(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	_, err := service.EnqueueDue(context.Background(), 1440)
	if err == nil {
		t.Fatal("expected error on store failure")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL error, got %v", err)
	}
}

func TestResetStuck_OK(t *testing.T) {
	q := &fakeQueueStore{}
	service := newImplService(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	if _, err := service.ResetStuck(context.Background(), 30*time.Minute); err != nil {
		t.Fatalf("ResetStuck() error = %v", err)
	}
	if !q.resetStuckCalled {
		t.Error("expected ResetStuck to be called on store")
	}
}

func TestResetStuck_StoreError(t *testing.T) {
	q := &fakeQueueStore{err: errors.New("db down")}
	service := newImplService(q, &fakeScanLogReader{}, &fakeScanSettingsRepo{}, &fakeCompanyAdminService{}, svc.Config{})

	_, err := service.ResetStuck(context.Background(), 30*time.Minute)
	if err == nil {
		t.Fatal("expected error on store failure")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL error, got %v", err)
	}
}

// Ensure test doubles implement their interfaces.
var _ svc.ScanQueueStore = (*fakeQueueStore)(nil)
var _ svc.ScanLogReader = (*fakeScanLogReader)(nil)
var _ companysvc.ScanSettingsRepository = (*fakeScanSettingsRepo)(nil)
var _ companysvc.CompanyAdminService = (*fakeCompanyAdminService)(nil)
