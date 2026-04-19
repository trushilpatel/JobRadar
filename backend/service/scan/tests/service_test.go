package scan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/service/company"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// fakeQueueStore is a test double for ScanQueueStore.
type fakeQueueStore struct {
	items            []dto.ScanQueueItem
	err              error
	enqueueCalled    bool
	removeCalled     bool
	enqueueDueResult int
	enqueueDueCalled bool
	resetStuckCalled bool
}

func (f *fakeQueueStore) Enqueue(_ context.Context, _, _ string) error {
	f.enqueueCalled = true
	return f.err
}
func (f *fakeQueueStore) EnqueueDue(_ context.Context, _ int, _ string) (int, error) {
	f.enqueueDueCalled = true
	return f.enqueueDueResult, f.err
}
func (f *fakeQueueStore) ClaimNext(_ context.Context) (*dto.ScanQueueItem, error) { return nil, f.err }
func (f *fakeQueueStore) Complete(_ context.Context, _ string) error              { return f.err }
func (f *fakeQueueStore) Remove(_ context.Context, _ string) error {
	f.removeCalled = true
	return f.err
}
func (f *fakeQueueStore) MoveUp(_ context.Context, _ string) error   { return f.err }
func (f *fakeQueueStore) MoveDown(_ context.Context, _ string) error { return f.err }
func (f *fakeQueueStore) ResetStuck(_ context.Context, _ time.Duration) error {
	f.resetStuckCalled = true
	return f.err
}
func (f *fakeQueueStore) UpdateHeartbeat(_ context.Context, _ string) error { return f.err }
func (f *fakeQueueStore) ListQueue(_ context.Context) ([]dto.ScanQueueItem, error) {
	return f.items, f.err
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

// fakeCompanyService is a test double for company.Service.
type fakeCompanyService struct {
	company *companymodel.Company
	views   []company.CompanyView
	err     error
}

func (f *fakeCompanyService) List(_ context.Context, _ company.CompanyFilter, _, _ int) (company.CompanyListResult, error) {
	return company.CompanyListResult{Items: f.views, Total: int64(len(f.views))}, f.err
}
func (f *fakeCompanyService) GetAll(_ context.Context) ([]company.CompanyView, error) {
	return f.views, f.err
}
func (f *fakeCompanyService) Get(_ context.Context, _ string) (*companymodel.Company, error) {
	return f.company, f.err
}
func (f *fakeCompanyService) Create(_ context.Context, _ company.CreateCompanyInput) (*companymodel.Company, error) {
	return f.company, f.err
}
func (f *fakeCompanyService) Update(_ context.Context, _ string, _ company.UpdateCompanyInput) error {
	return f.err
}
func (f *fakeCompanyService) Delete(_ context.Context, _ string) error { return f.err }
func (f *fakeCompanyService) SetAllActive(_ context.Context, _ bool) error {
	return f.err
}

// newImplService builds the full impl chain: repositoryService → implService.
func newImplService(q *fakeQueueStore, log *fakeScanLogReader, companySvc company.Service, cfg svc.Config) svc.Service {
	repo := svc.NewRepositoryService(q, log, cfg)
	return svc.NewService(repo, companySvc, cfg)
}

func TestTriggerScan_OK(t *testing.T) {
	q := &fakeQueueStore{}
	service := newImplService(q, &fakeScanLogReader{}, &fakeCompanyService{company: &companymodel.Company{ID: "stripe"}}, svc.Config{})

	err := service.TriggerScan(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("TriggerScan() error = %v", err)
	}
	if !q.enqueueCalled {
		t.Error("expected Enqueue to be called")
	}
}

func TestTriggerScan_CompanyNotFound(t *testing.T) {
	q := &fakeQueueStore{}
	service := newImplService(q, &fakeScanLogReader{}, &fakeCompanyService{
		err: domain.ErrNotFound.WithMessage("company not found"),
	}, svc.Config{})

	err := service.TriggerScan(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing company")
	}
	var sErr *domain.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND error, got %v", err)
	}
}

func TestListQueue_EnrichesWithCompanyData(t *testing.T) {
	lastScan := time.Now().Add(-1 * time.Hour)
	items := []dto.ScanQueueItem{
		{ID: "q1", CompanyID: "stripe", Status: "queued", Source: "manual", QueuedAt: time.Now()},
	}
	views := []company.CompanyView{
		{Company: companymodel.Company{ID: "stripe", Name: "Stripe", AdapterType: "greenhouse", LastScanAt: &lastScan}},
	}

	service := newImplService(
		&fakeQueueStore{items: items},
		&fakeScanLogReader{},
		&fakeCompanyService{views: views},
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

func TestListQueue_FiltersStatus(t *testing.T) {
	items := []dto.ScanQueueItem{
		{ID: "q1", CompanyID: "c1", Status: "queued", Source: "manual"},
		{ID: "q2", CompanyID: "c2", Status: "processing", Source: "manual"},
	}
	service := newImplService(
		&fakeQueueStore{items: items},
		&fakeScanLogReader{},
		&fakeCompanyService{views: []company.CompanyView{}},
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
	service := newImplService(q, &fakeScanLogReader{}, &fakeCompanyService{}, svc.Config{})

	if err := service.RemoveFromQueue(context.Background(), "q1"); err != nil {
		t.Fatalf("RemoveFromQueue() error = %v", err)
	}
	if !q.removeCalled {
		t.Error("expected Remove to be called")
	}
}

func TestEnqueueDue_EnqueuesAll(t *testing.T) {
	q := &fakeQueueStore{enqueueDueResult: 3}
	service := newImplService(q, &fakeScanLogReader{}, &fakeCompanyService{}, svc.Config{})

	n, err := service.EnqueueDue(context.Background(), 1440)
	if err != nil {
		t.Fatalf("EnqueueDue() error = %v", err)
	}
	if n != 3 {
		t.Errorf("expected count 3, got %d", n)
	}
	if !q.enqueueDueCalled {
		t.Error("expected EnqueueDue to be called on store")
	}
}

func TestEnqueueDue_NoDueCompanies(t *testing.T) {
	q := &fakeQueueStore{enqueueDueResult: 0}
	service := newImplService(q, &fakeScanLogReader{}, &fakeCompanyService{}, svc.Config{})

	n, err := service.EnqueueDue(context.Background(), 1440)
	if err != nil {
		t.Fatalf("EnqueueDue() error = %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 when none due, got %d", n)
	}
}

func TestEnqueueDue_StoreError(t *testing.T) {
	q := &fakeQueueStore{err: errors.New("db down")}
	service := newImplService(q, &fakeScanLogReader{}, &fakeCompanyService{}, svc.Config{})

	_, err := service.EnqueueDue(context.Background(), 1440)
	if err == nil {
		t.Fatal("expected error on store failure")
	}
	var sErr *domain.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL error, got %v", err)
	}
}

func TestResetStuck_OK(t *testing.T) {
	q := &fakeQueueStore{}
	service := newImplService(q, &fakeScanLogReader{}, &fakeCompanyService{}, svc.Config{})

	if err := service.ResetStuck(context.Background(), 30*time.Minute); err != nil {
		t.Fatalf("ResetStuck() error = %v", err)
	}
	if !q.resetStuckCalled {
		t.Error("expected ResetStuck to be called on store")
	}
}

func TestResetStuck_StoreError(t *testing.T) {
	q := &fakeQueueStore{err: errors.New("db down")}
	service := newImplService(q, &fakeScanLogReader{}, &fakeCompanyService{}, svc.Config{})

	err := service.ResetStuck(context.Background(), 30*time.Minute)
	if err == nil {
		t.Fatal("expected error on store failure")
	}
	var sErr *domain.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL error, got %v", err)
	}
}

// Ensure test doubles implement their interfaces.
var _ svc.ScanQueueStore = (*fakeQueueStore)(nil)
var _ svc.ScanLogReader = (*fakeScanLogReader)(nil)
var _ company.Service = (*fakeCompanyService)(nil)
