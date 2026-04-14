package insights

import (
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/api/response"
)

// fakeInsightsStore is a test double for Store.
type fakeInsightsStore struct {
	techCounts    []TechCount
	activity      []CompanyActivityItem
	funnel        []FunnelStep
	locations     []LocationCount
	states        []StateCount
	sponsorship   []SponsorshipDay
	velocity      []CompanyVelocity
	roleCategories []RoleCategoryCount
	pipelineCounts PipelineCounts
	err           error
}

func (f *fakeInsightsStore) GetTechStackTrends(_ int) ([]TechCount, error) {
	return f.techCounts, f.err
}
func (f *fakeInsightsStore) GetCompanyActivity() ([]CompanyActivityItem, error) {
	return f.activity, f.err
}
func (f *fakeInsightsStore) GetPipelineFunnel() ([]FunnelStep, error) { return f.funnel, f.err }
func (f *fakeInsightsStore) GetLocationBreakdown() ([]LocationCount, error) {
	return f.locations, f.err
}
func (f *fakeInsightsStore) GetUSAStateBreakdown() ([]StateCount, error) { return f.states, f.err }
func (f *fakeInsightsStore) GetSponsorshipTrend(_ int) ([]SponsorshipDay, error) {
	return f.sponsorship, f.err
}
func (f *fakeInsightsStore) GetCompanyVelocity(_ int) ([]CompanyVelocity, error) {
	return f.velocity, f.err
}
func (f *fakeInsightsStore) GetRoleCategoryBreakdown() ([]RoleCategoryCount, error) {
	return f.roleCategories, f.err
}
func (f *fakeInsightsStore) GetPipelineCounts() (PipelineCounts, error) {
	return f.pipelineCounts, f.err
}

func TestTechStack_OK(t *testing.T) {
	svc := NewService(&fakeInsightsStore{techCounts: []TechCount{{Tech: "Go", Count: 5}}})
	data, err := svc.TechStack()
	if err != nil {
		t.Fatalf("TechStack() error = %v", err)
	}
	if data == nil {
		t.Error("expected non-nil data")
	}
}

func TestTechStack_Error(t *testing.T) {
	svc := NewService(&fakeInsightsStore{err: errors.New("db error")})
	_, err := svc.TechStack()
	if err == nil {
		t.Fatal("expected error")
	}
	var sErr *response.ServiceError
	if !errors.As(err, &sErr) || sErr.Status != 500 {
		t.Errorf("expected 500 ServiceError, got %v", err)
	}
}

func TestSponsorship_DefaultDays(t *testing.T) {
	svc := NewService(&fakeInsightsStore{sponsorship: []SponsorshipDay{}})
	// days=0 should default to 30 inside the service
	data, err := svc.Sponsorship(InsightsFilter{Days: 0})
	if err != nil {
		t.Fatalf("Sponsorship() error = %v", err)
	}
	if data == nil {
		t.Error("expected non-nil data")
	}
}

// Ensure test double implements interface.
var _ Store = (*fakeInsightsStore)(nil)
