package insights

import (
	"context"
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/service/insights/dto"
	svc "github.com/trushilpatel/jobradar/service/insights/services"
)

// fakeInsightsStore is a test double for Store.
type fakeInsightsStore struct {
	techCounts     []dto.TechCount
	activity       []dto.CompanyActivityItem
	funnel         []dto.FunnelStep
	locations      []dto.LocationCount
	states         []dto.StateCount
	sponsorship    []dto.SponsorshipDay
	velocity       []dto.CompanyVelocity
	roleCategories []dto.RoleCategoryCount
	pipelineCounts dto.PipelineCounts
	err            error
}

func (f *fakeInsightsStore) GetTechStackTrends(_ context.Context, _ int) ([]dto.TechCount, error) {
	return f.techCounts, f.err
}
func (f *fakeInsightsStore) GetCompanyActivity(_ context.Context) ([]dto.CompanyActivityItem, error) {
	return f.activity, f.err
}
func (f *fakeInsightsStore) GetPipelineFunnel(_ context.Context) ([]dto.FunnelStep, error) {
	return f.funnel, f.err
}
func (f *fakeInsightsStore) GetLocationBreakdown(_ context.Context) ([]dto.LocationCount, error) {
	return f.locations, f.err
}
func (f *fakeInsightsStore) GetUSAStateBreakdown(_ context.Context) ([]dto.StateCount, error) {
	return f.states, f.err
}
func (f *fakeInsightsStore) GetSponsorshipTrend(_ context.Context, _ int) ([]dto.SponsorshipDay, error) {
	return f.sponsorship, f.err
}
func (f *fakeInsightsStore) GetCompanyVelocity(_ context.Context, _ int) ([]dto.CompanyVelocity, error) {
	return f.velocity, f.err
}
func (f *fakeInsightsStore) GetRoleCategoryBreakdown(_ context.Context) ([]dto.RoleCategoryCount, error) {
	return f.roleCategories, f.err
}
func (f *fakeInsightsStore) GetPipelineCounts(_ context.Context) (dto.PipelineCounts, error) {
	return f.pipelineCounts, f.err
}

func TestTechStack_OK(t *testing.T) {
	service := svc.NewImplService(&fakeInsightsStore{techCounts: []dto.TechCount{{Tech: "Go", Count: 5}}})
	data, err := service.TechStack(context.Background())
	if err != nil {
		t.Fatalf("TechStack() error = %v", err)
	}
	if data == nil {
		t.Error("expected non-nil data")
	}
}

func TestTechStack_Error(t *testing.T) {
	service := svc.NewImplService(&fakeInsightsStore{err: errors.New("db error")})
	_, err := service.TechStack(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var sErr *domain.ServiceError
	if !errors.As(err, &sErr) || sErr.Status != 500 {
		t.Errorf("expected 500 ServiceError, got %v", err)
	}
}

func TestSponsorship_DefaultDays(t *testing.T) {
	service := svc.NewImplService(&fakeInsightsStore{sponsorship: []dto.SponsorshipDay{}})
	// days=0 should default to 30 inside the service
	data, err := service.Sponsorship(context.Background(), dto.InsightsFilter{Days: 0})
	if err != nil {
		t.Fatalf("Sponsorship() error = %v", err)
	}
	if data == nil {
		t.Error("expected non-nil data")
	}
}

// Ensure test double implements interface.
var _ svc.Store = (*fakeInsightsStore)(nil)
