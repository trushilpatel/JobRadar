package scan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/service/company"
)

// fakeQueueStore is a test double for ScanQueueStore.
type fakeQueueStore struct {
	items         []ScanQueueItem
	err           error
	enqueueCalled bool
	removeCalled  bool
}

func (f *fakeQueueStore) Enqueue(_ context.Context, _, _ string) error {
	f.enqueueCalled = true
	return f.err
}
func (f *fakeQueueStore) EnqueueDue(_ context.Context, _ int, _ string) (string, error) {
	return "", f.err
}
func (f *fakeQueueStore) ClaimNext(ctx context.Context) (*ScanQueueItem, error) { return nil, f.err }
func (f *fakeQueueStore) Complete(_ context.Context, id string) error           { return f.err }
func (f *fakeQueueStore) Remove(_ context.Context, id string) error {
	f.removeCalled = true
	return f.err
}
func (f *fakeQueueStore) MoveUp(_ context.Context, id string) error           { return f.err }
func (f *fakeQueueStore) MoveDown(_ context.Context, id string) error         { return f.err }
func (f *fakeQueueStore) ResetStuck(_ context.Context, d time.Duration) error { return f.err }
func (f *fakeQueueStore) UpdateHeartbeat(_ context.Context, id string) error  { return f.err }
func (f *fakeQueueStore) ListQueue(_ context.Context) ([]ScanQueueItem, error) {
	return f.items, f.err
}

// fakeScanLogReader is a test double for ScanLogReader.
type fakeScanLogReader struct {
	stats ScanStatsView
	err   error
}

func (f *fakeScanLogReader) GetScanStats(_ context.Context, companyID string) (ScanStatsView, error) {
	return f.stats, f.err
}
func (f *fakeScanLogReader) GetScanHistories(_ context.Context, _ []string) (map[string][]bool, error) {
	return nil, f.err
}
func (f *fakeScanLogReader) GetJobsScannedToday(_ context.Context) (int64, error) { return 0, f.err }
func (f *fakeScanLogReader) GetScansByDay(_ context.Context, days int) ([]DailyScanCount, error) {
	return nil, f.err
}

// fakeCompanyService is a test double for company.Service.
type fakeCompanyService struct {
	company *model.Company
	views   []company.CompanyView
	err     error
}

func (f *fakeCompanyService) List(_ context.Context, _ company.CompanyFilter, _, _ int) (company.CompanyListResult, error) {
	return company.CompanyListResult{Items: f.views, Total: int64(len(f.views))}, f.err
}
func (f *fakeCompanyService) GetAll(_ context.Context) ([]company.CompanyView, error) {
	return f.views, f.err
}
func (f *fakeCompanyService) Get(_ context.Context, id string) (*model.Company, error) {
	return f.company, f.err
}
func (f *fakeCompanyService) Create(_ context.Context, _ company.CreateCompanyInput) (*model.Company, error) {
	return f.company, f.err
}
func (f *fakeCompanyService) Update(_ context.Context, id string, _ company.UpdateCompanyInput) error {
	return f.err
}
func (f *fakeCompanyService) Delete(_ context.Context, id string) error { return f.err }
func (f *fakeCompanyService) SetAllActive(_ context.Context, active bool) error {
	return f.err
}

func TestTriggerScan_OK(t *testing.T) {
	q := &fakeQueueStore{}
	svc := NewService(q, &fakeScanLogReader{}, &fakeCompanyService{company: &model.Company{ID: "stripe"}}, Config{})

	err := svc.TriggerScan(context.Background(), "stripe")
	if err != nil {
		t.Fatalf("TriggerScan() error = %v", err)
	}
	if !q.enqueueCalled {
		t.Error("expected Enqueue to be called")
	}
}

func TestTriggerScan_CompanyNotFound(t *testing.T) {
	q := &fakeQueueStore{}
	svc := NewService(q, &fakeScanLogReader{}, &fakeCompanyService{
		err: domain.ErrNotFound.WithMessage("company not found"),
	}, Config{})

	err := svc.TriggerScan(context.Background(), "missing")
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
	items := []ScanQueueItem{
		{ID: "q1", CompanyID: "stripe", Status: "queued", Source: "manual", QueuedAt: time.Now()},
	}
	views := []company.CompanyView{
		{Company: model.Company{ID: "stripe", Name: "Stripe", AdapterType: "greenhouse", LastScanAt: &lastScan}},
	}

	svc := NewService(
		&fakeQueueStore{items: items},
		&fakeScanLogReader{},
		&fakeCompanyService{views: views},
		Config{ScanIntervalMinutes: 60},
	)

	result, err := svc.ListQueue(context.Background(), QueueFilter{})
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
	items := []ScanQueueItem{
		{ID: "q1", CompanyID: "c1", Status: "queued", Source: "manual"},
		{ID: "q2", CompanyID: "c2", Status: "processing", Source: "manual"},
	}
	svc := NewService(
		&fakeQueueStore{items: items},
		&fakeScanLogReader{},
		&fakeCompanyService{views: []company.CompanyView{}},
		Config{},
	)

	result, err := svc.ListQueue(context.Background(), QueueFilter{Status: "queued"})
	if err != nil {
		t.Fatalf("ListQueue() error = %v", err)
	}
	if len(result) != 1 || result[0].ID != "q1" {
		t.Errorf("expected only queued item q1, got %+v", result)
	}
}

func TestRemoveFromQueue(t *testing.T) {
	q := &fakeQueueStore{}
	svc := NewService(q, &fakeScanLogReader{}, &fakeCompanyService{}, Config{})

	if err := svc.RemoveFromQueue(context.Background(), "q1"); err != nil {
		t.Fatalf("RemoveFromQueue() error = %v", err)
	}
	if !q.removeCalled {
		t.Error("expected Remove to be called")
	}
}

// Ensure test doubles implement their interfaces.
var _ ScanQueueStore = (*fakeQueueStore)(nil)
var _ ScanLogReader = (*fakeScanLogReader)(nil)
var _ company.Service = (*fakeCompanyService)(nil)
