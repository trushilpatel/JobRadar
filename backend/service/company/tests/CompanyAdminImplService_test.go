package company_test

import (
	"context"
	"testing"

	"github.com/trushilpatel/jobradar/platform/cache"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// fakeAdmin records calls and lets each test seed canned outputs.
type fakeAdmin struct {
	createView *companydto.CompanyAdminView
	createErr  error
	updateErr  error
	deleteErr  error

	createCalls    int
	updPublicCalls int
	updScanCalls   int
	deleteCalls    int
}

func (f *fakeAdmin) List(_ context.Context, _ companydto.CompanyFilter, _ int) (companydto.CompanyAdminListResult, error) {
	return companydto.CompanyAdminListResult{}, nil
}
func (f *fakeAdmin) Get(_ context.Context, _ string) (*companydto.CompanyAdminView, error) {
	return nil, nil
}
func (f *fakeAdmin) Create(_ context.Context, _ companydto.CompanyPublicInput, _ companydto.CompanyScanSettingsInput) (*companydto.CompanyAdminView, error) {
	f.createCalls++
	return f.createView, f.createErr
}
func (f *fakeAdmin) UpdatePublic(_ context.Context, _ string, _ companydto.CompanyPublicInput) error {
	f.updPublicCalls++
	return f.updateErr
}
func (f *fakeAdmin) UpdateScanSettings(_ context.Context, _ string, _ companydto.CompanyScanSettingsInput) error {
	f.updScanCalls++
	return f.updateErr
}
func (f *fakeAdmin) Delete(_ context.Context, _ string) error {
	f.deleteCalls++
	return f.deleteErr
}
func (f *fakeAdmin) SetAllActive(_ context.Context, _ bool) error { return nil }
func (f *fakeAdmin) GetByIDs(_ context.Context, _ []string) ([]companydto.CompanyAdminView, error) {
	return nil, nil
}
func (f *fakeAdmin) GetAll(_ context.Context) ([]companydto.CompanyAdminView, error) {
	return nil, nil
}

func TestAdminImpl_Create_PopulatesCacheKeyAfterWrite(t *testing.T) {
	fake := &fakeAdmin{
		createView: &companydto.CompanyAdminView{Company: companymodel.Company{ID: "stripe"}},
	}
	c := cache.NewMemoryCache()
	// Seed the public cache with a stale entry so we can verify it gets cleared.
	_ = c.Set(context.Background(), companysvc.PublicCompanyKey("stripe"), companydto.CompanyPublicView{}, 0)

	svc := companysvc.NewAdminImplService(fake, c, nil)
	if _, err := svc.Create(context.Background(), companydto.CompanyPublicInput{ID: "stripe"}, companydto.CompanyScanSettingsInput{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if fake.createCalls != 1 {
		t.Errorf("Create not delegated, calls=%d", fake.createCalls)
	}

	var v companydto.CompanyPublicView
	hit, _ := c.Get(context.Background(), companysvc.PublicCompanyKey("stripe"), &v)
	if hit {
		t.Errorf("expected public cache to be invalidated after Create")
	}
}

func TestAdminImpl_UpdatePublic_InvalidatesPublicCache(t *testing.T) {
	fake := &fakeAdmin{}
	c := cache.NewMemoryCache()
	_ = c.Set(context.Background(), companysvc.PublicCompanyKey("stripe"), companydto.CompanyPublicView{}, 0)

	svc := companysvc.NewAdminImplService(fake, c, nil)
	if err := svc.UpdatePublic(context.Background(), "stripe", companydto.CompanyPublicInput{Name: "Renamed"}); err != nil {
		t.Fatalf("UpdatePublic: %v", err)
	}
	var v companydto.CompanyPublicView
	hit, _ := c.Get(context.Background(), companysvc.PublicCompanyKey("stripe"), &v)
	if hit {
		t.Errorf("expected public cache to be invalidated after UpdatePublic")
	}
	if fake.updPublicCalls != 1 {
		t.Errorf("UpdatePublic not delegated, calls=%d", fake.updPublicCalls)
	}
}

func TestAdminImpl_UpdateScanSettings_DoesNotTouchPublicCache(t *testing.T) {
	fake := &fakeAdmin{}
	c := cache.NewMemoryCache()
	_ = c.Set(context.Background(), companysvc.PublicCompanyKey("stripe"), companydto.CompanyPublicView{}, 0)

	svc := companysvc.NewAdminImplService(fake, c, nil)
	if err := svc.UpdateScanSettings(context.Background(), "stripe", companydto.CompanyScanSettingsInput{Active: boolPtr(false)}); err != nil {
		t.Fatalf("UpdateScanSettings: %v", err)
	}
	var v companydto.CompanyPublicView
	hit, _ := c.Get(context.Background(), companysvc.PublicCompanyKey("stripe"), &v)
	if !hit {
		t.Errorf("scan_settings update must not invalidate the public cache")
	}
	if fake.updScanCalls != 1 {
		t.Errorf("UpdateScanSettings not delegated, calls=%d", fake.updScanCalls)
	}
}

func TestAdminImpl_Delete_InvalidatesPublicCache(t *testing.T) {
	fake := &fakeAdmin{}
	c := cache.NewMemoryCache()
	_ = c.Set(context.Background(), companysvc.PublicCompanyKey("stripe"), companydto.CompanyPublicView{}, 0)

	svc := companysvc.NewAdminImplService(fake, c, nil)
	if err := svc.Delete(context.Background(), "stripe"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var v companydto.CompanyPublicView
	hit, _ := c.Get(context.Background(), companysvc.PublicCompanyKey("stripe"), &v)
	if hit {
		t.Errorf("expected public cache to be invalidated after Delete")
	}
	if fake.deleteCalls != 1 {
		t.Errorf("Delete not delegated, calls=%d", fake.deleteCalls)
	}
}

func TestAdminImpl_NilCachePassesThrough(t *testing.T) {
	fake := &fakeAdmin{}
	svc := companysvc.NewAdminImplService(fake, nil, nil)
	if err := svc.UpdatePublic(context.Background(), "x", companydto.CompanyPublicInput{Name: "Y"}); err != nil {
		t.Fatalf("nil cache: %v", err)
	}
}

var _ companysvc.CompanyAdminService = (*fakeAdmin)(nil)
