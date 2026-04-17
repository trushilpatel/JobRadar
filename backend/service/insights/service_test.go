package insights

import (
	"context"
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/domain"
)

// fakeInsightsStore is a test double for Store.
type fakeInsightsStore struct {
	techCounts     []TechCount
	activity       []CompanyActivityItem
	funnel         []FunnelStep
	locations      []LocationCount
	states         []StateCount
	sponsorship    []SponsorshipDay
	velocity       []CompanyVelocity
	roleCategories []RoleCategoryCount
	pipelineCounts PipelineCounts
	err            error
}

func (f *fakeInsightsStore) GetTechStackTrends(_ context.Context, _ int) ([]TechCount, error) {
	return f.techCounts, f.err
}
func (f *fakeInsightsStore) GetCompanyActivity(_ context.Context) ([]CompanyActivityItem, error) {
	return f.activity, f.err
}
func (f *fakeInsightsStore) GetPipelineFunnel(_ context.Context) ([]FunnelStep, error) {
	return f.funnel, f.err
}
func (f *fakeInsightsStore) GetLocationBreakdown(_ context.Context) ([]LocationCount, error) {
	return f.locations, f.err
}
func (f *fakeInsightsStore) GetUSAStateBreakdown(_ context.Context) ([]StateCount, error) {
	return f.states, f.err
}
func (f *fakeInsightsStore) GetSponsorshipTrend(_ context.Context, _ int) ([]SponsorshipDay, error) {
	return f.sponsorship, f.err
}
func (f *fakeInsightsStore) GetCompanyVelocity(_ context.Context, _ int) ([]CompanyVelocity, error) {
	return f.velocity, f.err
}
func (f *fakeInsightsStore) GetRoleCategoryBreakdown(_ context.Context) ([]RoleCategoryCount, error) {
	return f.roleCategories, f.err
}
func (f *fakeInsightsStore) GetPipelineCounts(_ context.Context) (PipelineCounts, error) {
	return f.pipelineCounts, f.err
}

func TestTechStack_OK(t *testing.T) {
	svc := NewService(&fakeInsightsStore{techCounts: []TechCount{{Tech: "Go", Count: 5}}})
	data, err := svc.TechStack(context.Background())
	if err != nil {
		t.Fatalf("TechStack() error = %v", err)
	}
	if data == nil {
		t.Error("expected non-nil data")
	}
}

func TestTechStack_Error(t *testing.T) {
	svc := NewService(&fakeInsightsStore{err: errors.New("db error")})
	_, err := svc.TechStack(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var sErr *domain.ServiceError
	if !errors.As(err, &sErr) || sErr.Status != 500 {
		t.Errorf("expected 500 ServiceError, got %v", err)
	}
}

func TestSponsorship_DefaultDays(t *testing.T) {
	svc := NewService(&fakeInsightsStore{sponsorship: []SponsorshipDay{}})
	// days=0 should default to 30 inside the service
	data, err := svc.Sponsorship(context.Background(), InsightsFilter{Days: 0})
	if err != nil {
		t.Fatalf("Sponsorship() error = %v", err)
	}
	if data == nil {
		t.Error("expected non-nil data")
	}
}

// Ensure test double implements interface.
var _ Store = (*fakeInsightsStore)(nil)
