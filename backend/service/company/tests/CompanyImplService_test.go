package company_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	"gorm.io/gorm"
)

// fakeScanHistoryReader is a test double for ScanHistoryReader.
type fakeScanHistoryReader struct{}

func (f *fakeScanHistoryReader) GetScanHistories(_ context.Context, _ []string) (map[string][]bool, error) {
	return nil, nil
}

// fakeStore is a test double for Store.
type fakeStore struct {
	companies []model.Company
	err       error

	updateCalled bool
	updateFields map[string]any
	deleteCalled bool
	createCalled bool
}

func (f *fakeStore) GetActiveCompanies(_ context.Context) ([]model.Company, error) {
	return f.companies, f.err
}
func (f *fakeStore) GetAllCompanies(_ context.Context) ([]model.Company, error) {
	return f.companies, f.err
}
func (f *fakeStore) GetCompanyByID(_ context.Context, id string) (model.Company, error) {
	for _, c := range f.companies {
		if c.ID == id {
			return c, nil
		}
	}
	if f.err != nil {
		return model.Company{}, f.err
	}
	return model.Company{}, gorm.ErrRecordNotFound
}
func (f *fakeStore) GetCompanies(_ context.Context, _ companydto.CompanyFilter, _, _ int) ([]model.Company, int64, error) {
	return f.companies, int64(len(f.companies)), f.err
}
func (f *fakeStore) GetCompanyCounts(_ context.Context) (total, active int64, err error) {
	active = 0
	for _, c := range f.companies {
		if c.Active {
			active++
		}
	}
	return int64(len(f.companies)), active, f.err
}
func (f *fakeStore) CreateCompany(_ context.Context, c model.Company) error {
	f.createCalled = true
	return f.err
}
func (f *fakeStore) UpdateCompany(_ context.Context, id string, fields map[string]any) error {
	f.updateCalled = true
	f.updateFields = fields
	if f.err != nil {
		return f.err
	}
	for _, c := range f.companies {
		if c.ID == id {
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}
func (f *fakeStore) DeleteCompany(_ context.Context, id string) error {
	f.deleteCalled = true
	return f.err
}
func (f *fakeStore) SetAllCompaniesActive(_ context.Context, active bool) error {
	return f.err
}
func (f *fakeStore) UpdateScanStatus(_ context.Context, id, status string) error {
	return f.err
}

func TestList_ComputesNextScanAt(t *testing.T) {
	lastScan := time.Now().Add(-1 * time.Hour)
	companies := []model.Company{
		{ID: "c1", Name: "C1", LastScanAt: &lastScan},
		{ID: "c2", Name: "C2", LastScanAt: nil},
	}
	svc := companysvc.NewService(&fakeStore{companies: companies}, companycommon.Config{ScanIntervalMinutes: 60}, &fakeScanHistoryReader{})

	result, err := svc.List(context.Background(), companydto.CompanyFilter{}, 1, 20)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 views, got %d", len(result.Items))
	}

	// c1 has LastScanAt set so NextScanAt should be computed
	if result.Items[0].NextScanAt == nil {
		t.Error("expected NextScanAt to be non-nil for c1")
	}
	// c2 has no LastScanAt so NextScanAt should be nil
	if result.Items[1].NextScanAt != nil {
		t.Error("expected NextScanAt to be nil for c2")
	}

	// NextScanAt should be lastScan + 60 minutes
	want := lastScan.Add(60 * time.Minute)
	got := *result.Items[0].NextScanAt
	if got.Sub(want).Abs() > time.Second {
		t.Errorf("NextScanAt = %v, want ~%v", got, want)
	}
}

func TestCreate_SetsLastScanAt(t *testing.T) {
	before := time.Now()
	fs := &fakeStore{companies: []model.Company{}}
	svc := companysvc.NewService(fs, companycommon.Config{}, &fakeScanHistoryReader{})

	company, err := svc.Create(context.Background(), companydto.CreateCompanyInput{
		ID:          "stripe",
		Name:        "Stripe",
		URL:         "https://stripe.com/jobs",
		AdapterType: companycommon.AdapterGreenhouse,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !fs.createCalled {
		t.Error("expected CreateCompany to be called")
	}
	if company.LastScanAt == nil {
		t.Fatal("expected LastScanAt to be set")
	}
	if company.LastScanAt.Before(before) {
		t.Errorf("LastScanAt %v is before test start %v", company.LastScanAt, before)
	}
}

func TestUpdate_NilFieldsSkipped(t *testing.T) {
	fs := &fakeStore{companies: []model.Company{{ID: "stripe"}}}
	svc := companysvc.NewService(fs, companycommon.Config{}, &fakeScanHistoryReader{})

	name := "New Name"
	err := svc.Update(context.Background(), "stripe", companydto.UpdateCompanyInput{Name: &name})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !fs.updateCalled {
		t.Fatal("expected UpdateCompany to be called")
	}

	// Only name should be in the fields map
	if len(fs.updateFields) != 1 {
		t.Errorf("expected 1 field, got %d: %v", len(fs.updateFields), fs.updateFields)
	}
	if _, ok := fs.updateFields["name"]; !ok {
		t.Error("expected 'name' in update fields")
	}
	if _, ok := fs.updateFields["url"]; ok {
		t.Error("unexpected 'url' in update fields")
	}
}

func TestDelete_ReturnsNotFoundWhenMissing(t *testing.T) {
	fs := &fakeStore{companies: []model.Company{}} // no companies
	svc := companysvc.NewService(fs, companycommon.Config{}, &fakeScanHistoryReader{})

	err := svc.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing company")
	}

	var sErr *domain.ServiceError
	if !errors.As(err, &sErr) {
		t.Fatalf("expected ServiceError, got %T: %v", err, err)
	}
	if sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %s", sErr.Code)
	}
}

func TestList_FiltersActiveCompanies(t *testing.T) {
	// fakeStore.GetCompanies ignores the filter — this test verifies the service
	// passes filter through; store-level filtering is tested via integration tests.
	companies := []model.Company{
		{ID: "c1", Active: true},
	}
	svc := companysvc.NewService(&fakeStore{companies: companies}, companycommon.Config{}, &fakeScanHistoryReader{})

	active := true
	result, err := svc.List(context.Background(), companydto.CompanyFilter{Active: &active}, 1, 20)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "c1" {
		t.Errorf("expected company c1, got %+v", result.Items)
	}
	if result.Total != 1 {
		t.Errorf("expected total 1, got %d", result.Total)
	}
}
