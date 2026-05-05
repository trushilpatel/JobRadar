package company_test

import (
	"context"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/platform/cache"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// fakePublic counts calls and returns canned values — lets us assert when
// the cache layer hit vs. fell through.
type fakePublic struct {
	listResult companydto.CompanyPublicListResult
	getResult  *companydto.CompanyPublicView
	listCalls  int
	getCalls   int
	err        error
}

func (f *fakePublic) ListPublic(_ context.Context, _ companydto.CompanyFilter, _, _ int) (companydto.CompanyPublicListResult, error) {
	f.listCalls++
	return f.listResult, f.err
}
func (f *fakePublic) GetPublic(_ context.Context, _ string) (*companydto.CompanyPublicView, error) {
	f.getCalls++
	return f.getResult, f.err
}

func TestCache_GetPublic_HitMiss(t *testing.T) {
	view := &companydto.CompanyPublicView{Company: companymodel.Company{ID: "stripe", Name: "Stripe", Slug: "stripe"}}
	fake := &fakePublic{getResult: view}
	c := cache.NewMemoryCache()
	svc := companysvc.NewCacheService(fake, c, 1*time.Minute)

	// Miss — falls through to next, populates cache.
	if _, err := svc.GetPublic(context.Background(), "stripe"); err != nil {
		t.Fatalf("first GetPublic err = %v", err)
	}
	if fake.getCalls != 1 {
		t.Errorf("expected 1 next.GetPublic call, got %d", fake.getCalls)
	}

	// Hit — served from cache.
	if _, err := svc.GetPublic(context.Background(), "stripe"); err != nil {
		t.Fatalf("second GetPublic err = %v", err)
	}
	if fake.getCalls != 1 {
		t.Errorf("expected cache hit (still 1 call), got %d", fake.getCalls)
	}
}

func TestCache_ListPublic_HitMiss(t *testing.T) {
	fake := &fakePublic{
		listResult: companydto.CompanyPublicListResult{
			Items: []companydto.CompanyPublicView{{Company: companymodel.Company{ID: "stripe", Name: "Stripe", Slug: "stripe"}}},
			Total: 1,
		},
	}
	c := cache.NewMemoryCache()
	svc := companysvc.NewCacheService(fake, c, 1*time.Minute)

	if _, err := svc.ListPublic(context.Background(), companydto.CompanyFilter{}, 1, 20); err != nil {
		t.Fatalf("first ListPublic err = %v", err)
	}
	if fake.listCalls != 1 {
		t.Errorf("expected 1 next.ListPublic call, got %d", fake.listCalls)
	}
	if _, err := svc.ListPublic(context.Background(), companydto.CompanyFilter{}, 1, 20); err != nil {
		t.Fatalf("second ListPublic err = %v", err)
	}
	if fake.listCalls != 1 {
		t.Errorf("expected cache hit (still 1 call), got %d", fake.listCalls)
	}
}

func TestCache_ListPublic_DistinctFiltersDistinctKeys(t *testing.T) {
	fake := &fakePublic{}
	c := cache.NewMemoryCache()
	svc := companysvc.NewCacheService(fake, c, 1*time.Minute)

	_, _ = svc.ListPublic(context.Background(), companydto.CompanyFilter{Search: "a"}, 1, 20)
	_, _ = svc.ListPublic(context.Background(), companydto.CompanyFilter{Search: "b"}, 1, 20)
	if fake.listCalls != 2 {
		t.Errorf("distinct filters must miss the cache distinctly, calls = %d", fake.listCalls)
	}
}

func TestCache_NilCacheIsPassThrough(t *testing.T) {
	fake := &fakePublic{getResult: &companydto.CompanyPublicView{Company: companymodel.Company{ID: "x"}}}
	svc := companysvc.NewCacheService(fake, nil, 1*time.Minute)
	if _, err := svc.GetPublic(context.Background(), "x"); err != nil {
		t.Fatalf("nil cache: %v", err)
	}
	if _, err := svc.GetPublic(context.Background(), "x"); err != nil {
		t.Fatalf("nil cache: %v", err)
	}
	if fake.getCalls != 2 {
		t.Errorf("expected 2 calls with nil cache, got %d", fake.getCalls)
	}
}

var _ companysvc.CompanyService = (*fakePublic)(nil)
